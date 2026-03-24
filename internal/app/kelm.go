package kelm

import (
	"context"
	"os"
	"time"

	"kelm/internal/pkg/k8s"
	"kelm/internal/pkg/zarf"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type CountdownCancel struct {
	envName string
	ttl     int
	cancel  context.CancelFunc
}

// Set for tracking namespaces being deleted by operator
var deletingNamespaces = make(map[string]struct{})

func getRetryDelay() time.Duration {
	s := os.Getenv("RETRY_DELAY")
	if s == "" {
		return 1 * time.Hour
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		logrus.Warnf("Invalid RETRY_DELAY %q, using 1h: %v", s, err)
		return 1 * time.Hour
	}
	return d
}

func Init() {
	var config *rest.Config
	var err error

	// Try in-cluster config first
	config, err = rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig file (for local dev)
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			logrus.Errorf("Failed to build kubeconfig: %v\n", err)
			os.Exit(1)
		}
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Errorf("Failed to create clientset: %v\n", err)
		os.Exit(1)
	}
	logrus.Info("Operator launched")
	logrus.Infof("Ignoring namespaces: %s", ignoredNamespaces)
	logrus.Infof("Zarf integration enabled: %v", isZarfEnabled())
	logrus.Infof("Retry delay: %v", getRetryDelay())
	envs, err := getEnvs(client, labels.Set{"kelm.riftonix.io/managed": "true"})
	if err != nil {
		logrus.Errorf("Failed to get namespaces: %v", err)
		os.Exit(1)
	}
	countdowns := make([]CountdownCancel, 0)
	for _, env := range envs {
		startCountdown(client, &countdowns, env, int(env.RemainingTtl.Seconds()))
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
		if err != nil && !kerrors.IsNotFound(err) {
			logrus.Warningf("%v", err)
			continue
		}
		envName := namespace.EnvName
		logrus.Infof("Event %s for namespace %s with env.name=%s", event.Type, ns.Name, envName)

		// Cancel existing countdowns for this env and recalculate
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
		if kerrors.IsNotFound(err) {
			logrus.Infof("Env '%s' was empty and removed", envName)
			continue
		}
		if err != nil {
			logrus.Errorf("Failed to get namespaces for env.name=%s: %v", envName, err)
			continue
		}

		for _, env := range envs {
			startCountdown(client, countdowns, env, int(env.RemainingTtl.Seconds()))
		}
	}
}

// startCountdown registers and launches a deletion countdown for the given env.
func startCountdown(client *kubernetes.Clientset, countdowns *[]CountdownCancel, env Env, ttlSeconds int) {
	ctx, cancel := context.WithCancel(context.Background())
	*countdowns = append(*countdowns, CountdownCancel{
		envName: env.Name,
		cancel:  cancel,
		ttl:     ttlSeconds,
	})
	go CreateCountdown(ctx, env, ttlSeconds, "removal", makeDeleteCallback(client, countdowns, env))
}

// makeDeleteCallback builds the deletion callback for an env.
// On failure, a retry is scheduled after RETRY_DELAY.
func makeDeleteCallback(client *kubernetes.Clientset, countdowns *[]CountdownCancel, env Env) DeleteNamespacesCallback {
	return func(namespaces []string) {
		for _, ns := range namespaces {
			deletingNamespaces[ns] = struct{}{}
		}
		defer func() {
			for _, ns := range namespaces {
				delete(deletingNamespaces, ns)
			}
		}()

		if env.IsZarf {
			if err := zarf.RemovePackage(context.Background(), env.ZarfPackageName); err != nil {
				if kerrors.IsNotFound(err) {
					logrus.Warnf("Zarf package %q is not found in cluster, assuming it already removed", env.ZarfPackageName)
				} else {
					logrus.Errorf("Failed to remove zarf package %q: %v", env.ZarfPackageName, err)
					scheduleRetry(client, countdowns, env)
					return
				}
			}
			if err := zarf.PruneImages(context.Background()); err != nil {
				logrus.Errorf("Failed to prune zarf registry images: %v", err)
				scheduleRetry(client, countdowns, env)
				return
			}
		}

		results := k8s.ForceDeleteNamespaces(client, namespaces, time.Minute, 5*time.Second)
		if hasFailedDeletions(results) {
			scheduleRetry(client, countdowns, env)
			return
		}
	}
}

func scheduleRetry(client *kubernetes.Clientset, countdowns *[]CountdownCancel, env Env) {
	delay := getRetryDelay()
	logrus.Infof("Scheduling retry deletion for env '%s' in %v", env.Name, delay)
	startCountdown(client, countdowns, env, int(delay.Seconds()))
}

func hasFailedDeletions(results []k8s.NamespaceDeleteResult) bool {
	for _, r := range results {
		if r.State == "timeout" || r.State == "error" {
			return true
		}
	}
	return false
}
