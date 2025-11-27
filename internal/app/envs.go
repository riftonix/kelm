package kelm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"kelm/internal/pkg/timer"

	"github.com/sirupsen/logrus"
	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// 1 RawEnvPart = 1 namespace
type RawEnvPart struct {
	Name                string
	IsManaged           bool
	EnvName             string
	Ttl                 string
	ReplenishRatio      float64
	NotificationFactors []float64
	NsData              core.Namespace
	CreationTimestamp   time.Time
	UpdateTimestamp     time.Time
}

// 1 RawEnv = n namespaces
type RawEnv struct {
	Name                string
	Namespaces          []core.Namespace
	Ttl                 string `default:"0s"`
	ReplenishRatio      float64
	NotificationFactors []float64
	CreationTimestamp   time.Time
	UpdateTimestamp     time.Time
}

// 1 RawEnv = 1 Env; Env - resulted entity, needs for kelm.go
type Env struct {
	Name                      string
	Namespaces                []string
	RemainingTtl              time.Duration
	ReplenishRatio            float64
	RemainingNotificationsTtl []time.Duration
	CreationTimestamp         time.Time
	UpdateTimestamp           time.Time
}

func getIgnoredNamespaces() []string {
	env := os.Getenv("IGNORED_NAMESPACES")
	if env == "" {
		return []string{"default", "kube-system", "cert-manager"}
	}
	parts := strings.Split(env, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{"default", "kube-system", "cert-manager"}
	}
	return result
}

var ignoredNamespaces = getIgnoredNamespaces()

func handleNamespace(ns core.Namespace) (RawEnvPart, error) {
	for _, ignored := range ignoredNamespaces {
		if ns.Name == ignored {
			return RawEnvPart{}, fmt.Errorf("namespace %s is in ignored list", ns.Name)
		}
	}
	isManaged := ns.Labels["kelm.riftonix.io/managed"]
	envName := ns.Labels["kelm.riftonix.io/env.name"]
	ttl := ns.Annotations["kelm.riftonix.io/ttl.removal"]
	replenishRatio := ns.Annotations["kelm.riftonix.io/ttl.replenishRatio"]
	notificationFactors := ns.Annotations["kelm.riftonix.io/ttl.notificationFactors"]
	updateTimestamp := ns.Annotations["kelm.riftonix.io/updateTimestamp"]
	var rawEnvPart RawEnvPart
	if isManaged != "true" {
		return rawEnvPart, fmt.Errorf("namespace %s label kelm.riftonix.io/managed is not true", ns.Name)
	}
	if envName == "" {
		return rawEnvPart, fmt.Errorf("namespace %s has empty label kelm.riftonix.io/env.name", ns.Name)
	}
	if ttl == "" {
		return rawEnvPart, fmt.Errorf("namespace %s has empty annotation kelm.riftonix.io/ttl.removal", ns.Name)
	}
	if replenishRatio == "" {
		return rawEnvPart, fmt.Errorf("namespace %s has empty annotation kelm.riftonix.io/ttl.replenishRatio", ns.Name)
	}
	parsedReplenishRatio, err := strconv.ParseFloat(replenishRatio, 64)
	if err != nil {
		return rawEnvPart, fmt.Errorf("failed to parse namespace %s annotation kelm.riftonix.io/ttl.replenishRatio '%s': %w", ns.Name, replenishRatio, err)
	}
	if notificationFactors == "" {
		return rawEnvPart, fmt.Errorf("namespace %s has empty annotation kelm.riftonix.io/ttl.notificationFactors", ns.Name)
	}
	if updateTimestamp == "" {
		return rawEnvPart, fmt.Errorf("namespace %s has empty annotation kelm.riftonix.io/updateTimestamp", ns.Name)
	}
	parsedUpdateTimestamp, err := timer.ParseTime(updateTimestamp)
	if err != nil {
		return rawEnvPart, fmt.Errorf("failed to parse namespace %s annotation kelm.riftonix.io/updateTimestamp '%s': %w", ns.Name, updateTimestamp, err)
	}
	var unmarshaledNotificationFactors []float64
	err = json.Unmarshal([]byte(notificationFactors), &unmarshaledNotificationFactors)
	if err != nil {
		return rawEnvPart, fmt.Errorf("failed to parse namespace %s annotation kelm.riftonix.io/ttl.notificationFactors '%s': %w", ns.Name, notificationFactors, err)
	}
	rawEnvPart.Name = ns.Name
	rawEnvPart.IsManaged = true
	rawEnvPart.EnvName = envName
	rawEnvPart.Ttl = ttl
	rawEnvPart.ReplenishRatio = parsedReplenishRatio
	rawEnvPart.NotificationFactors = unmarshaledNotificationFactors
	rawEnvPart.NsData = ns
	rawEnvPart.CreationTimestamp = ns.CreationTimestamp.Time.UTC()
	rawEnvPart.UpdateTimestamp = parsedUpdateTimestamp
	return rawEnvPart, nil
}

func updateRawEnv(rawEnv RawEnv, rawEnvPart RawEnvPart) RawEnv {
	var err error
	rawEnv.Name = rawEnvPart.EnvName
	rawEnv.Namespaces = append(rawEnv.Namespaces, rawEnvPart.NsData)
	if rawEnv.Ttl == "" {
		rawEnv.Ttl = "0s" // Default value
	}
	rawEnv.Ttl, err = timer.GetMaxDuration(rawEnv.Ttl, rawEnvPart.Ttl)
	if err != nil {
		// You should not see this log, rawEnvPart already validated
		logrus.Warningf("Ttl in %s has bad format '%s': %v", rawEnvPart.Name, rawEnvPart.Ttl, err)
	}
	rawEnv.ReplenishRatio = max(rawEnv.ReplenishRatio, rawEnvPart.ReplenishRatio)
	rawEnv.NotificationFactors = append(rawEnv.NotificationFactors, rawEnvPart.NotificationFactors...)
	slices.Sort(rawEnv.NotificationFactors)
	rawEnv.NotificationFactors = slices.Compact(rawEnv.NotificationFactors)
	rawEnv.CreationTimestamp = timer.GetMaxTime(rawEnv.CreationTimestamp, rawEnvPart.CreationTimestamp)
	rawEnv.UpdateTimestamp = timer.GetMaxTime(rawEnv.UpdateTimestamp, rawEnvPart.UpdateTimestamp)
	return rawEnv
}

func getEnvs(client kubernetes.Interface, labelsSet labels.Set) (map[string]Env, error) {
	filter := meta.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelsSet).String(),
	}
	logrus.Debug("Gathering namespaces...")
	namespaces, err := client.CoreV1().Namespaces().List(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	envs := make(map[string]Env)
	rawEnvs := make(map[string]RawEnv)
	for _, ns := range namespaces.Items {
		rawEnvPart, err := handleNamespace(ns)
		if err != nil {
			logrus.Warningf("%v", err)
			continue
		}
		rawEnvs[rawEnvPart.EnvName] = updateRawEnv(rawEnvs[rawEnvPart.EnvName], rawEnvPart)
	}
	for _, rawEnv := range rawEnvs {
		var env Env
		env.Name = rawEnv.Name
		for _, ns := range rawEnv.Namespaces {
			env.Namespaces = append(env.Namespaces, ns.Name)
		}
		env.RemainingTtl, err = timer.GetDuration(rawEnv.CreationTimestamp, rawEnv.Ttl, 1)
		if err != nil {
			// You should not see this log, rawEnvPart already validated
			logrus.Warningf("Failed to parse annotations in %s: %v\n", rawEnv.Name, err)
			continue
		}
		env.ReplenishRatio = rawEnv.ReplenishRatio
		for _, factor := range rawEnv.NotificationFactors {
			remainingNotificationTtl, err := timer.GetDuration(rawEnv.CreationTimestamp, rawEnv.Ttl, factor)
			if err != nil {
				logrus.Warningf("Failed to parse annotations in %s: %v", rawEnv.Name, err)
				continue
			}
			env.RemainingNotificationsTtl = append(env.RemainingNotificationsTtl, remainingNotificationTtl)
		}
		env.CreationTimestamp = rawEnv.CreationTimestamp
		env.UpdateTimestamp = rawEnv.UpdateTimestamp
		logrus.WithFields(logrus.Fields{
			"Namespaces":                env.Namespaces,
			"RemainingTtl":              env.RemainingTtl,
			"ReplenishRatio":            env.ReplenishRatio,
			"RemainingNotificationsTtl": env.RemainingNotificationsTtl,
			"CreationTimestamp":         env.CreationTimestamp,
			"UpdateTimestamp":           env.UpdateTimestamp,
		}).Infof("Env '%s' updated", env.Name)
		envs[rawEnv.Name] = env
	}
	return envs, nil
}
