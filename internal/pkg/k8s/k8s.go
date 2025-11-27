package k8s

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NamespaceDeleteResult - structure with ns deletion information
type NamespaceDeleteResult struct {
	Namespace      string
	State          string // "deleted", "force-deleted", "not-found", "timeout", "error"
	DeletionError  error
	FinalizerError error
	Duration       time.Duration
}

// waitForNamespaceDeletion makes API calls until namespace deletion or context expiration
func waitForNamespaceDeletion(ctx context.Context, client kubernetes.Interface, namespaceName string, pollingPeriod time.Duration) bool {
	ticker := time.NewTicker(pollingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			_, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
			if err != nil {
				if errors.IsNotFound(err) {
					return true
				}
				logrus.Warning(err)
			}
		}
	}
}

// forceDeleteNamespaces sequential removes namespaces from list in 2 stages
// 1. Simple remove and waiting
// 2. If can not remove, clear finalizers and wait again
// Why sequential deletion and not parallel deletion?
// Because we don't expect many namespaces in an environment, and the environments themselves are deleted in parallel.
// Also, the parallel deletion code turned out to be too complex; I don't want to maintain it :)
func ForceDeleteNamespaces(
	client kubernetes.Interface,
	namespaceNames []string,
	timeout time.Duration,
	pollingPeriod time.Duration,
) []NamespaceDeleteResult {
	results := make([]NamespaceDeleteResult, 0, len(namespaceNames))

	for _, namespaceName := range namespaceNames {
		result := NamespaceDeleteResult{Namespace: namespaceName}
		start := time.Now()

		// Stage 1: simple removal
		ctx1, cancel1 := context.WithTimeout(context.Background(), timeout)
		defer cancel1()
		err := client.CoreV1().Namespaces().Delete(ctx1, namespaceName, metav1.DeleteOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				result.State = "not-found" // Ok, it's probably manual removal
			} else {
				result.State = "error"
			}
			result.DeletionError = err
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		// Stage 1 polling: wait for remove or timeout
		deleted := waitForNamespaceDeletion(ctx1, client, namespaceName, pollingPeriod)
		if deleted {
			result.State = "deleted"
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		// Stage 2: finalizers removal
		ctx2, cancel2 := context.WithTimeout(context.Background(), timeout)
		defer cancel2()
		ns, err := client.CoreV1().Namespaces().Get(ctx2, namespaceName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				result.State = "deleted" // Well, namespace removed succesfully on stage 1
			} else {
				result.State = "error"
			}
			result.FinalizerError = err
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}
		ns.Finalizers = nil
		_, err = client.CoreV1().Namespaces().Finalize(ctx2, ns, metav1.UpdateOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				result.State = "deleted" // Well, namespace removed succesfully on stage 1
			} else {
				result.State = "error"
			}
			result.State = "error"
			result.FinalizerError = err
			result.Duration = time.Since(start)
			results = append(results, result)
			continue
		}

		// Stage 2 polling: wait for remove or timeout
		deleted = waitForNamespaceDeletion(ctx2, client, namespaceName, pollingPeriod)
		if deleted {
			result.State = "force-deleted"
		} else {
			result.State = "timeout"
		}
		result.Duration = time.Since(start)
		results = append(results, result)
	}
	return results
}
