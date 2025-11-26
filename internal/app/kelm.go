package kelm

import (
	"context"
	"os"

	"kelm/internal/pkg/timer"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type CountdownCancel struct {
	envName string
	ttl     int
	cancel  context.CancelFunc
}

func Init() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		logrus.Errorf("Failed to build kubeconfig: %v\n", err)
		os.Exit(1)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Errorf("Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	envs, err := getEnvs(client, labels.Set{"kelm.riftonix.io/managed": "true"})
	if err != nil {
		logrus.Errorf("Failed to get namespaces: %v", err)
		os.Exit(1)
	}
	countdowns := make([]CountdownCancel, 0)
	for envName, env := range envs {
		ctx, cancel := context.WithCancel(context.Background())
		countdowns = append(countdowns, CountdownCancel{
			envName: envName,
			cancel:  cancel,
			ttl:     int(env.RemainingTtl.Seconds()),
		})
		go timer.CreateCountdown(ctx, envName, int(env.RemainingTtl.Seconds()), "removal")
		for _, remainingNotificationTtl := range env.RemainingNotificationsTtl {
			go timer.CreateCountdown(ctx, envName, int(remainingNotificationTtl.Seconds()), "notification")
		}
	}
	go Watch(client, &countdowns)
	select {}
}

func Watch(client *kubernetes.Clientset, countdowns *[]CountdownCancel) {
	watchInterface, err := client.CoreV1().Namespaces().Watch(context.Background(), meta.ListOptions{
		LabelSelector: "kelm.riftonix.io/managed=true",
	})
	if err != nil {
		logrus.Errorf("Failed to start watch: %v", err)
		return
	}
	defer watchInterface.Stop()
	for event := range watchInterface.ResultChan() {
		ns, ok := event.Object.(*core.Namespace)
		if !ok {
			logrus.Warn("Unexpected object type in watch event")
			continue
		}
		namespace, err := handleNamespace(*ns)
		if err != nil {
			logrus.Warningf("%v", err)
			continue
		}
		envName := namespace.EnvName
		logrus.Infof("Event %s for namespace %s with env.name=%s", event.Type, ns.Name, envName)

		//recalculate timers
		filtered := (*countdowns)[:0]
		for _, cd := range *countdowns {
			if cd.envName == envName {
				cd.cancel()
			} else {
				filtered = append(filtered, cd)
			}
		}
		*countdowns = filtered

		envs, err := getEnvs(client, labels.Set{
			"kelm.riftonix.io/managed":  "true",
			"kelm.riftonix.io/env.name": envName,
		})
		if err != nil {
			logrus.Errorf("Failed to get namespaces for env.name=%s: %v", envName, err)
			continue
		}

		for envName, env := range envs {
			ctx, cancel := context.WithCancel(context.Background())
			*countdowns = append(*countdowns, CountdownCancel{
				envName: envName,
				cancel:  cancel,
				ttl:     int(env.RemainingTtl.Seconds()),
			})
			go timer.CreateCountdown(ctx, envName, int(env.RemainingTtl.Seconds()), "removal")
			for _, remainingNotificationTtl := range env.RemainingNotificationsTtl {
				go timer.CreateCountdown(ctx, envName, int(remainingNotificationTtl.Seconds()), "notification")
			}
		}
	}
}
