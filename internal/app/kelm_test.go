package kelm

import (
	"context"
	"testing"

	core "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// Temp empty tests. It's too hard to create unit tests for these functions
func TestInitDummy(t *testing.T) {
	// Always success
}

func TestWatchDummy(t *testing.T) {
	// Always success
}

func TestGetZarfNamespace(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("ZARF_NAMESPACE", "")
		if got := getZarfNamespace(); got != "zarf" {
			t.Errorf("Expected default zarf namespace, got %q", got)
		}
	})

	t.Run("from env", func(t *testing.T) {
		t.Setenv("ZARF_NAMESPACE", "custom-zarf")
		if got := getZarfNamespace(); got != "custom-zarf" {
			t.Errorf("Expected custom zarf namespace, got %q", got)
		}
	})
}

func TestDeleteZarfPackageSecret(t *testing.T) {
	t.Setenv("ZARF_NAMESPACE", "custom-zarf")
	client := fake.NewSimpleClientset(&core.Secret{
		ObjectMeta: meta.ObjectMeta{
			Name:      "test-package",
			Namespace: "custom-zarf",
		},
	})

	deleteZarfPackageSecret(client, "test-package")

	_, err := client.CoreV1().Secrets("custom-zarf").Get(context.Background(), "test-package", meta.GetOptions{})
	if !kerrors.IsNotFound(err) {
		t.Fatalf("Expected zarf package secret to be deleted, got %v", err)
	}
}
