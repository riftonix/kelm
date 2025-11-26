package kelm

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	core "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestHandleNamespace(t *testing.T) {
	validTime := time.Now().UTC().Format(time.RFC3339)
	notificationFactors, _ := json.Marshal([]float64{0.5, 0.8})

	baseNamespace := core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name: "test-ns",
			Labels: map[string]string{
				"kelm.riftonix.io/managed":  "true",
				"kelm.riftonix.io/env.name": "env1",
			},
			Annotations: map[string]string{
				"kelm.riftonix.io/ttl.removal":             "1h",
				"kelm.riftonix.io/ttl.replenishRatio":      "1.5",
				"kelm.riftonix.io/ttl.notificationFactors": string(notificationFactors),
				"kelm.riftonix.io/updateTimestamp":         validTime,
			},
			CreationTimestamp: meta.Time{Time: time.Now().Add(-2 * time.Hour)},
		},
	}

	t.Run("valid namespace", func(t *testing.T) {
		namespace, err := handleNamespace(baseNamespace)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if namespace.Name != "test-ns" || namespace.EnvName != "env1" || namespace.Ttl != "1h" {
			t.Errorf("Unexpected RawEnvPart: %+v", namespace)
		}
		if len(namespace.NotificationFactors) != 2 || namespace.NotificationFactors[0] != 0.5 {
			t.Errorf("Unexpected NotificationFactors: %+v", namespace.NotificationFactors)
		}
	})

	t.Run("not managed", func(t *testing.T) {
		ns := baseNamespace
		ns.Labels["kelm.riftonix.io/managed"] = "false"
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for not managed namespace")
		}
	})

	t.Run("missing env.name", func(t *testing.T) {
		ns := baseNamespace
		delete(ns.Labels, "kelm.riftonix.io/env.name")
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for missing env.name")
		}
	})

	t.Run("missing ttl", func(t *testing.T) {
		ns := baseNamespace
		delete(ns.Annotations, "kelm.riftonix.io/ttl.removal")
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for missing ttl.removal")
		}
	})

	t.Run("bad replenishRatio", func(t *testing.T) {
		ns := baseNamespace
		ns.Annotations["kelm.riftonix.io/ttl.replenishRatio"] = "bad"
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for bad replenishRatio")
		}
	})

	t.Run("bad notificationFactors", func(t *testing.T) {
		ns := baseNamespace
		ns.Annotations["kelm.riftonix.io/ttl.notificationFactors"] = "notjson"
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for bad notificationFactors")
		}
	})

	t.Run("bad updateTimestamp", func(t *testing.T) {
		ns := baseNamespace
		ns.Annotations["kelm.riftonix.io/updateTimestamp"] = "badtime"
		_, err := handleNamespace(ns)
		if err == nil || err.Error() == "" {
			t.Error("Expected error for bad updateTimestamp")
		}
	})
}

func TestUpdateRawEnv(t *testing.T) {
	// Prepare base RawEnv and RawEnvPart
	baseTime := time.Now().Add(-3 * time.Hour).UTC()
	updateTime := time.Now().Add(-1 * time.Hour).UTC()
	baseNamespace := core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name:              "ns1",
			CreationTimestamp: meta.Time{Time: baseTime},
		},
	}
	part := RawEnvPart{
		Name:                "ns1",
		IsManaged:           true,
		EnvName:             "env1",
		Ttl:                 "2h",
		ReplenishRatio:      1.5,
		NotificationFactors: []float64{0.5, 0.8},
		NsData:              baseNamespace,
		CreationTimestamp:   baseTime,
		UpdateTimestamp:     updateTime,
	}

	t.Run("first update, empty RawEnv", func(t *testing.T) {
		var newRawEnv RawEnv
		updatedRawEnv := updateRawEnv(newRawEnv, part)
		if updatedRawEnv.Name != "env1" {
			t.Errorf("Expected Name 'env1', got %q", updatedRawEnv.Name)
		}
		if len(updatedRawEnv.Namespaces) != 1 || updatedRawEnv.Namespaces[0].Name != "ns1" {
			t.Errorf("Expected Namespaces with ns1, got %+v", updatedRawEnv.Namespaces)
		}
		if updatedRawEnv.Ttl != "2h" {
			t.Errorf("Expected Ttl '2h', got %q", updatedRawEnv.Ttl)
		}
		if updatedRawEnv.ReplenishRatio != 1.5 {
			t.Errorf("Expected ReplenishRatio 1.5, got %v", updatedRawEnv.ReplenishRatio)
		}
		if len(updatedRawEnv.NotificationFactors) != 2 {
			t.Errorf("Expected 2 NotificationFactors, got %d", len(updatedRawEnv.NotificationFactors))
		}
		if !updatedRawEnv.CreationTimestamp.Equal(baseTime) {
			t.Errorf("Expected CreationTimestamp %v, got %v", baseTime, updatedRawEnv.CreationTimestamp)
		}
		if !updatedRawEnv.UpdateTimestamp.Equal(updateTime) {
			t.Errorf("Expected UpdateTimestamp %v, got %v", updateTime, updatedRawEnv.UpdateTimestamp)
		}
	})

	t.Run("merge with existing RawEnv", func(t *testing.T) {
		// Existing env with different values
		existingRawEnv := RawEnv{
			Name:                "env1",
			Namespaces:          []core.Namespace{{ObjectMeta: meta.ObjectMeta{Name: "ns0"}}},
			Ttl:                 "1h",
			ReplenishRatio:      1.0,
			NotificationFactors: []float64{0.5, 0.9},
			CreationTimestamp:   baseTime.Add(-1 * time.Hour),
			UpdateTimestamp:     updateTime.Add(-30 * time.Minute),
		}
		updatedRawEnv := updateRawEnv(existingRawEnv, part)
		if len(updatedRawEnv.Namespaces) != 2 {
			t.Errorf("Expected 2 Namespaces, got %d", len(updatedRawEnv.Namespaces))
		}
		if updatedRawEnv.Ttl != "2h" {
			t.Errorf("Expected Ttl '2h' (max), got %q", updatedRawEnv.Ttl)
		}
		if updatedRawEnv.ReplenishRatio != 1.5 {
			t.Errorf("Expected ReplenishRatio 1.5 (max), got %v", updatedRawEnv.ReplenishRatio)
		}
		// NotificationFactors should be sorted and unique
		expectedFactors := []float64{0.5, 0.8, 0.9}
		if len(updatedRawEnv.NotificationFactors) != 3 {
			t.Errorf("Expected 3 NotificationFactors, got %d", len(updatedRawEnv.NotificationFactors))
		}
		for i, v := range expectedFactors {
			if updatedRawEnv.NotificationFactors[i] != v {
				t.Errorf("Expected NotificationFactors[%d]=%v, got %v", i, v, updatedRawEnv.NotificationFactors[i])
			}
		}
		// CreationTimestamp and UpdateTimestamp should be max of both
		if !updatedRawEnv.CreationTimestamp.Equal(baseTime) {
			t.Errorf("Expected CreationTimestamp %v, got %v", baseTime, updatedRawEnv.CreationTimestamp)
		}
		if !updatedRawEnv.UpdateTimestamp.Equal(updateTime) {
			t.Errorf("Expected UpdateTimestamp %v, got %v", updateTime, updatedRawEnv.UpdateTimestamp)
		}
	})
}

func makeNamespace(name, envName, ttl, replenishRatio, notificationFactors, updateTimestamp string, creation time.Time, managed string) *core.Namespace {
	return &core.Namespace{
		ObjectMeta: meta.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kelm.riftonix.io/managed":  managed,
				"kelm.riftonix.io/env.name": envName,
			},
			Annotations: map[string]string{
				"kelm.riftonix.io/ttl.removal":             ttl,
				"kelm.riftonix.io/ttl.replenishRatio":      replenishRatio,
				"kelm.riftonix.io/ttl.notificationFactors": notificationFactors,
				"kelm.riftonix.io/updateTimestamp":         updateTimestamp,
			},
			CreationTimestamp: meta.Time{Time: creation},
		},
	}
}

func TestGetEnvs(t *testing.T) {
	validTime := time.Now().UTC().Format(time.RFC3339)
	notificationFactors, _ := json.Marshal([]float64{0.5, 0.8})

	t.Run("single valid namespace", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			makeNamespace("ns1", "env1", "1h", "1.5", string(notificationFactors), validTime, time.Now().Add(-2*time.Hour), "true"),
		)
		labelsSet := labels.Set{"kelm.riftonix.io/managed": "true"}
		envs, err := getEnvs(client, labelsSet)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envs) != 1 {
			t.Fatalf("Expected 1 env, got %d", len(envs))
		}
		if envs["env1"].Name != "env1" {
			t.Errorf("Expected env name 'env1', got %s", envs["env1"].Name)
		}
		if len(envs["env1"].Namespaces) != 1 || envs["env1"].Namespaces[0] != "ns1" {
			t.Errorf("Expected namespace 'ns1', got %+v", envs["env1"].Namespaces)
		}
	})

	t.Run("multiple namespaces, different envs", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			makeNamespace("ns1", "env1", "1h", "1.5", string(notificationFactors), validTime, time.Now().Add(-2*time.Hour), "true"),
			makeNamespace("ns2", "env2", "2h", "2.0", string(notificationFactors), validTime, time.Now().Add(-1*time.Hour), "true"),
		)
		labelsSet := labels.Set{"kelm.riftonix.io/managed": "true"}
		envs, err := getEnvs(client, labelsSet)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envs) != 2 {
			t.Fatalf("Expected 2 envs, got %d", len(envs))
		}
		if envs["env1"].Name != "env1" || envs["env2"].Name != "env2" {
			t.Errorf("Unexpected env names: %+v", envs)
		}
	})

	t.Run("one env with two namespaces", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			makeNamespace("ns1", "env1", "1h", "1.5", `[0.5,0.8]`, time.Now().UTC().Format(time.RFC3339), time.Now().Add(-2*time.Hour), "true"),
			makeNamespace("ns2", "env1", "2h", "2.0", `[0.5,0.8]`, time.Now().UTC().Format(time.RFC3339), time.Now().Add(-1*time.Hour), "true"),
		)
		labelsSet := labels.Set{"kelm.riftonix.io/managed": "true"}
		envs, err := getEnvs(client, labelsSet)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envs) != 1 {
			t.Fatalf("Expected 1 env, got %d", len(envs))
		}
		env, ok := envs["env1"]
		if !ok {
			t.Fatalf("Expected env1 to be present, got %+v", envs)
		}
		if len(env.Namespaces) != 2 {
			t.Errorf("Expected 2 namespaces in env1, got %d: %+v", len(env.Namespaces), env.Namespaces)
		}
		nsMap := map[string]bool{}
		for _, ns := range env.Namespaces {
			nsMap[ns] = true
		}
		if !nsMap["ns1"] || !nsMap["ns2"] {
			t.Errorf("Expected namespaces ns1 and ns2, got %+v", env.Namespaces)
		}
	})

	t.Run("namespace with invalid annotation is skipped", func(t *testing.T) {
		client := fake.NewSimpleClientset(
			makeNamespace("ns1", "env1", "1h", "bad", string(notificationFactors), validTime, time.Now().Add(-2*time.Hour), "true"),
			makeNamespace("ns2", "env2", "2h", "2.0", string(notificationFactors), validTime, time.Now().Add(-1*time.Hour), "true"),
		)
		labelsSet := labels.Set{"kelm.riftonix.io/managed": "true"}
		envs, err := getEnvs(client, labelsSet)
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
		if len(envs) != 1 {
			t.Fatalf("Expected 1 env, got %d", len(envs))
		}
		if _, ok := envs["env2"]; !ok {
			t.Errorf("Expected env2 to be present, got %+v", envs)
		}
	})

	t.Run("client returns error", func(t *testing.T) {
		// Use a fake client that returns error on List
		client := &fake.Clientset{}
		client.PrependReactor("list", "namespaces", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, errors.New("list error")
		})
		labelsSet := labels.Set{"kelm.riftonix.io/managed": "true"}
		_, err := getEnvs(client, labelsSet)
		if err == nil {
			t.Fatal("Expected error from client, got nil")
		}
	})
}
