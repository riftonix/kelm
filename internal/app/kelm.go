package kelm

import (
	"context"
	"os"
	"time"

	"kelm/internal/pkg/k8s"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

// Set for tracking namespaces being deleted by operator
var deletingNamespaces = make(map[string]struct{})

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
	logrus.Info("Operator launched")
	logrus.Infof("Ignoring namespaces: %s", ignoredNamespaces)
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
		envCopy := env // to avoid closure issues
		go CreateCountdown(
			ctx,
			envCopy,
			int(env.RemainingTtl.Seconds()),
			"removal",
			func(namespaces []string) {
				// Mark namespaces as being deleted
				for _, ns := range namespaces {
					deletingNamespaces[ns] = struct{}{}
				}
				// Delete namespaces
				k8s.ForceDeleteNamespaces(
					client,
					namespaces,
					time.Minute,
					5*time.Second,
				)
				// Remove from set after deletion
				for _, ns := range namespaces {
					delete(deletingNamespaces, ns)
				}
			},
		)
		// for _, remainingNotificationTtl := range env.RemainingNotificationsTtl {
		// 	go CreateCountdown(
		// 		ctx,
		// 		envCopy,
		// 		int(remainingNotificationTtl.Seconds()),
		// 		"notification",
		// 		nil,
		// 	)
		// }
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
		// Ignore events for namespaces being deleted by operator
		if _, exists := deletingNamespaces[ns.Name]; exists {
			logrus.Debugf("Ignoring event %s for namespace %s (deletion in progress)", event.Type, ns.Name)
			continue
		}
		namespace, err := handleNamespace(*ns)
		if err != nil && !errors.IsNotFound(err) {
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
		if errors.IsNotFound(err) {
			logrus.Infof("Env '%s' was empty and removed", envName)
			continue
		}
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
			envCopy := env // to avoid closure issues
			go CreateCountdown(
				ctx,
				envCopy,
				int(env.RemainingTtl.Seconds()),
				"removal",
				func(namespaces []string) {
					// Mark namespaces as being deleted
					for _, ns := range namespaces {
						deletingNamespaces[ns] = struct{}{}
					}
					k8s.ForceDeleteNamespaces(
						client,
						namespaces,
						time.Minute,
						5*time.Second,
					)
					// Remove from set after deletion
					for _, ns := range namespaces {
						delete(deletingNamespaces, ns)
					}
				},
			)
			// for _, remainingNotificationTtl := range env.RemainingNotificationsTtl {
			// 	go CreateCountdown(
			// 		ctx,
			// 		envCopy,
			// 		int(remainingNotificationTtl.Seconds()),
			// 		"notification",
			// 		nil,
			// 	)
			// }
		}
	}
}
