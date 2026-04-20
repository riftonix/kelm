package kelm

import (
	"context"
	"os"
	"sync"
	"time"

	"kelm/internal/pkg/k8s"
	"kelm/internal/pkg/zarf"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
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
var deletingNamespacesMu sync.RWMutex
var countdownsMu sync.Mutex

func getRetryDelay() time.Duration {
	return getDurationEnv("RETRY_DELAY", time.Hour)
}

func getWatchRetryDelay() time.Duration {
	return getDurationEnv("WATCH_RETRY_DELAY", 10*time.Second)
}

func getResyncInterval() time.Duration {
	return getDurationEnv("RESYNC_INTERVAL", 5*time.Minute)
}

func getDurationEnv(name string, fallback time.Duration) time.Duration {
	s := os.Getenv(name)
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		logrus.Warnf("Invalid %s %q, using %v: %v", name, s, fallback, err)
		return fallback
	}
	if d <= 0 {
		logrus.Warnf("Invalid %s %q, using %v: duration must be positive", name, s, fallback)
		return fallback
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
	logrus.Infof("Watch retry delay: %v", getWatchRetryDelay())
	logrus.Infof("Resync interval: %v", getResyncInterval())
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
	WatchWithContext(context.Background(), client, countdowns)
}

func WatchWithContext(ctx context.Context, client *kubernetes.Clientset, countdowns *[]CountdownCancel) {
	resyncTicker := time.NewTicker(getResyncInterval())
	defer resyncTicker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			logrus.Infof("Stopping namespace watch: %v", err)
			return
		}

		watchInterface, err := client.CoreV1().Namespaces().Watch(ctx, meta.ListOptions{
			LabelSelector: "kelm.riftonix.io/managed=true",
		})
		if err != nil {
			logrus.Errorf("Failed to start watch: %v", err)
			waitForWatchRetry(ctx)
			continue
		}

		logrus.Debug("Namespace watch started")
		watchClosed := false
		for !watchClosed {
			select {
			case <-ctx.Done():
				watchInterface.Stop()
				logrus.Infof("Stopping namespace watch: %v", ctx.Err())
				return
			case <-resyncTicker.C:
				resyncCountdowns(client, countdowns)
			case event, ok := <-watchInterface.ResultChan():
				if !ok {
					watchClosed = true
					logrus.Warn("Namespace watch channel closed, reconnecting")
					continue
				}
				handleNamespaceEvent(client, countdowns, event)
			}
		}
		watchInterface.Stop()
		waitForWatchRetry(ctx)
	}
}

func handleNamespaceEvent(client *kubernetes.Clientset, countdowns *[]CountdownCancel, event watch.Event) {
	ns, ok := event.Object.(*core.Namespace)
	if !ok {
		logrus.Warnf("Unexpected object type in watch event %s", event.Type)
		return
	}
	// Ignore events for namespaces being deleted by operator
	if isNamespaceDeleting(ns.Name) {
		logrus.Debugf("Ignoring event %s for namespace %s (deletion in progress)", event.Type, ns.Name)
		return
	}
	namespace, err := handleNamespace(*ns)
	if err != nil && !kerrors.IsNotFound(err) {
		logrus.Warningf("%v", err)
		return
	}
	envName := namespace.EnvName
	logrus.Infof("Event %s for namespace %s with env.name=%s", event.Type, ns.Name, envName)

	// Cancel existing countdowns for this env and recalculate
	cancelCountdownsForEnv(countdowns, envName)

	envs, err := getEnvs(client, labels.Set{
		"kelm.riftonix.io/managed":  "true",
		"kelm.riftonix.io/env.name": envName,
	})
	if kerrors.IsNotFound(err) {
		logrus.Infof("Env '%s' was empty and removed", envName)
		return
	}
	if err != nil {
		logrus.Errorf("Failed to get namespaces for env.name=%s: %v", envName, err)
		return
	}

	for _, env := range envs {
		startCountdown(client, countdowns, env, int(env.RemainingTtl.Seconds()))
	}
}

func waitForWatchRetry(ctx context.Context) {
	delay := getWatchRetryDelay()
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}

func resyncCountdowns(client *kubernetes.Clientset, countdowns *[]CountdownCancel) {
	logrus.Debug("Resyncing namespace countdowns")
	envs, err := getEnvs(client, labels.Set{"kelm.riftonix.io/managed": "true"})
	if err != nil {
		logrus.Errorf("Failed to resync namespaces: %v", err)
		return
	}
	cancelAllCountdowns(countdowns)
	for _, env := range envs {
		startCountdown(client, countdowns, env, int(env.RemainingTtl.Seconds()))
	}
}

// startCountdown registers and launches a deletion countdown for the given env.
func startCountdown(client *kubernetes.Clientset, countdowns *[]CountdownCancel, env Env, ttlSeconds int) {
	ctx, cancel := context.WithCancel(context.Background())
	countdownsMu.Lock()
	*countdowns = append(*countdowns, CountdownCancel{
		envName: env.Name,
		cancel:  cancel,
		ttl:     ttlSeconds,
	})
	countdownsMu.Unlock()
	go CreateCountdown(ctx, env, ttlSeconds, "removal", makeDeleteCallback(client, countdowns, env))
}

func cancelCountdownsForEnv(countdowns *[]CountdownCancel, envName string) {
	countdownsMu.Lock()
	defer countdownsMu.Unlock()

	filtered := (*countdowns)[:0]
	for _, cd := range *countdowns {
		if cd.envName == envName {
			cd.cancel()
			continue
		}
		filtered = append(filtered, cd)
	}
	*countdowns = filtered
}

func cancelAllCountdowns(countdowns *[]CountdownCancel) {
	countdownsMu.Lock()
	defer countdownsMu.Unlock()

	for _, cd := range *countdowns {
		cd.cancel()
	}
	*countdowns = (*countdowns)[:0]
}

// makeDeleteCallback builds the deletion callback for an env.
// On failure, a retry is scheduled after RETRY_DELAY.
func makeDeleteCallback(client *kubernetes.Clientset, countdowns *[]CountdownCancel, env Env) DeleteNamespacesCallback {
	return func(namespaces []string) {
		for _, ns := range namespaces {
			markNamespaceDeleting(ns)
		}
		defer func() {
			for _, ns := range namespaces {
				unmarkNamespaceDeleting(ns)
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

func markNamespaceDeleting(namespace string) {
	deletingNamespacesMu.Lock()
	defer deletingNamespacesMu.Unlock()
	deletingNamespaces[namespace] = struct{}{}
}

func unmarkNamespaceDeleting(namespace string) {
	deletingNamespacesMu.Lock()
	defer deletingNamespacesMu.Unlock()
	delete(deletingNamespaces, namespace)
}

func isNamespaceDeleting(namespace string) bool {
	deletingNamespacesMu.RLock()
	defer deletingNamespacesMu.RUnlock()
	_, exists := deletingNamespaces[namespace]
	return exists
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
