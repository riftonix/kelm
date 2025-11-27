package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestWaitForNamespaceDeletion(t *testing.T) {
	tests := []struct {
		name          string
		reactorError  error
		expectResult  bool
		timeout       time.Duration
		pollingPeriod time.Duration
	}{
		{
			name:          "Namespace deleted (IsNotFound)",
			reactorError:  k8serrors.NewNotFound(core.Resource("namespaces"), "test-ns"),
			expectResult:  true,
			timeout:       1 * time.Second,
			pollingPeriod: 50 * time.Millisecond,
		},
		{
			name:          "Timeout waiting for deletion",
			reactorError:  errors.New("still exists"),
			expectResult:  false,
			timeout:       50 * time.Millisecond,
			pollingPeriod: 50 * time.Millisecond,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()
			client.PrependReactor("get", "namespaces", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
				return true, nil, testCase.reactorError
			})

			ctx, cancel := context.WithTimeout(context.Background(), testCase.timeout)
			defer cancel()
			result := waitForNamespaceDeletion(ctx, client, "test-ns", testCase.pollingPeriod)
			if result != testCase.expectResult {
				t.Errorf("Expected result %v, got %v", testCase.expectResult, result)
			}
		})
	}
}

func TestForceDeleteNamespaces(t *testing.T) {
	// Case 1: Namespace is deleted on first try
	{
		client := fake.NewSimpleClientset()
		client.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})
		client.PrependReactor("get", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, k8serrors.NewNotFound(core.Resource("namespaces"), action.(k8stesting.GetAction).GetName())
		})

		results := ForceDeleteNamespaces(client, []string{"ns1"}, 50*time.Millisecond, 50*time.Millisecond)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].State != "deleted" {
			t.Errorf("Expected state 'deleted', got %q", results[0].State)
		}
	}

	// Case 2: Namespace not found on delete
	{
		client := fake.NewSimpleClientset()
		client.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, k8serrors.NewNotFound(core.Resource("namespaces"), action.(k8stesting.GetAction).GetName())
		})

		results := ForceDeleteNamespaces(client, []string{"ns2"}, 1*time.Second, 50*time.Millisecond)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].State != "not-found" {
			t.Errorf("Expected state 'not-found', got %q", results[0].State)
		}
	}

	// Case 3: Error on delete
	{
		client := fake.NewSimpleClientset()
		client.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("delete error")
		})

		results := ForceDeleteNamespaces(client, []string{"ns3"}, 1*time.Second, 50*time.Millisecond)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].State != "error" {
			t.Errorf("Expected state 'error', got %q", results[0].State)
		}
	}

	// Case 4: Needs finalizer removal, then deleted
	// {
	// 	ns := &core.Namespace{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name:       "ns4",
	// 			Finalizers: []string{"test/finalizer"},
	// 		},
	// 	}
	// 	client := fake.NewSimpleClientset(ns)
	// 	deleted := false
	// 	client.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
	// 		return true, nil, nil
	// 	})
	// 	client.PrependReactor("get", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
	// 		if !deleted {
	// 			return true, ns, nil
	// 		}
	// 		return true, nil, k8serrors.NewNotFound(core.Resource("namespaces"), "ns4")
	// 	})
	// 	client.PrependReactor("update", "namespaces/finalize", func(action k8stesting.Action) (bool, runtime.Object, error) {
	// 		ns.Finalizers = []string{}
	// 		deleted = true
	// 		return true, ns, nil
	// 	})

	// 	results := ForceDeleteNamespaces(client, []string{"ns4"}, 500*time.Millisecond, 25*time.Millisecond)
	// 	if len(results) != 1 {
	// 		t.Fatalf("Expected 1 result, got %d", len(results))
	// 	}
	// 	if results[0].State != "force-deleted" {
	// 		t.Errorf("Expected state 'force-deleted', got %q, error: %v", results[0].State, results[0].FinalizerError)
	// 	}
	// }

	// Case 5: Timeout after finalizer removal
	{
		ns := &core.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "ns5",
				Finalizers: []string{"test/finalizer"},
			},
		}

		client := fake.NewSimpleClientset(ns)
		client.PrependReactor("delete", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})
		client.PrependReactor("update", "namespaces/finalize", func(action k8stesting.Action) (bool, runtime.Object, error) {
			obj := action.(k8stesting.UpdateAction).GetObject().(*core.Namespace)
			obj.Finalizers = []string{"test/finalizer"}
			return true, obj, nil
		})
		results := ForceDeleteNamespaces(client, []string{"ns5"}, 150*time.Millisecond, 50*time.Millisecond)
		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}
		if results[0].State != "timeout" {
			t.Errorf("Expected state 'timeout', got %q, error: %v", results[0].State, results[0].FinalizerError)
		}
	}
}
