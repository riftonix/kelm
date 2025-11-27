package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
		pollingPeriod int
	}{
		{
			name:          "Namespace deleted (IsNotFound)",
			reactorError:  k8serrors.NewNotFound(core.Resource("namespaces"), "test-ns"),
			expectResult:  true,
			timeout:       1 * time.Second,
			pollingPeriod: 1,
		},
		{
			name:          "Timeout waiting for deletion",
			reactorError:  errors.New("still exists"),
			expectResult:  false,
			timeout:       50 * time.Millisecond,
			pollingPeriod: 1,
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
