package zarf

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sirupsen/logrus"
	zarfcluster "github.com/zarf-dev/zarf/src/pkg/cluster"
	"github.com/zarf-dev/zarf/src/pkg/images"
	"github.com/zarf-dev/zarf/src/pkg/packager"
	"github.com/zarf-dev/zarf/src/pkg/pki"
	"github.com/zarf-dev/zarf/src/pkg/state"
	"github.com/zarf-dev/zarf/src/pkg/transform"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

// RemovePackage removes a deployed Zarf package and all its cluster resources.
// Package metadata is retrieved from the cluster state (no package file required).
//
// namespace is the namespace the package's removal was triggered from. Zarf's
// --namespace override always deploys the whole package into a single namespace
// (it renames every namespace reference in the package to the override value),
// so that namespace's own name is always the correct namespace-override value
// when an override was used. There is no cluster-side signal that says whether
// an override was used at all, so we probe: try the override secret first, and
// fall back to the plain (non-overridden) deployment if it doesn't exist.
func RemovePackage(ctx context.Context, packageName, namespace string) error {
	c, err := zarfcluster.New(ctx)
	if err != nil {
		return fmt.Errorf("connect to zarf cluster: %w", err)
	}

	depPkg, namespaceOverride, err := getDeployedPackage(ctx, c, packageName, namespace)
	if err != nil {
		return fmt.Errorf("get deployed zarf package %q: %w", packageName, err)
	}

	logrus.Infof("Removing zarf package %q (version %s)", packageName, depPkg.Data.Metadata.Version)
	return packager.Remove(ctx, depPkg.Data, packager.RemoveOptions{
		Cluster:           c,
		Timeout:           10 * time.Minute,
		NamespaceOverride: namespaceOverride,
	})
}

// getDeployedPackage looks up the deployed-package secret for packageName, trying
// the namespace-override variant (keyed by namespace) before falling back to the
// plain, non-overridden deployment. It returns the resolved namespace-override
// value (empty if none) alongside the package, so callers can pass it back into
// Zarf's removal APIs, which re-derive the same secret name from it.
func getDeployedPackage(ctx context.Context, c *zarfcluster.Cluster, packageName, namespace string) (*state.DeployedPackage, string, error) {
	if namespace != "" {
		depPkg, err := c.GetDeployedPackage(ctx, packageName, state.WithPackageNamespaceOverride(namespace))
		if err == nil {
			return depPkg, namespace, nil
		}
		if !kerrors.IsNotFound(err) {
			return nil, "", err
		}
	}
	depPkg, err := c.GetDeployedPackage(ctx, packageName)
	return depPkg, "", err
}

// PackageSecretName returns the name of the Kubernetes secret Zarf uses to store
// the deployed package state for the given namespace-override value (empty for a
// plain, non-overridden deployment).
func PackageSecretName(packageName, namespaceOverride string) string {
	depPkg := state.DeployedPackage{Name: packageName, NamespaceOverride: namespaceOverride}
	return depPkg.GetSecretName()
}

// PruneImages removes images from the Zarf internal registry that are no longer
// referenced by any deployed package.
func PruneImages(ctx context.Context) error {
	c, err := zarfcluster.New(ctx)
	if err != nil {
		return fmt.Errorf("connect to zarf cluster: %w", err)
	}

	zarfState, err := c.LoadState(ctx)
	if err != nil {
		return fmt.Errorf("load zarf state: %w", err)
	}

	zarfPackages, err := c.GetDeployedZarfPackages(ctx)
	if err != nil {
		return fmt.Errorf("get deployed zarf packages: %w", err)
	}

	registryEndpoint, tunnel, err := c.ConnectToZarfRegistryEndpoint(ctx, zarfState.RegistryInfo)
	if err != nil {
		return fmt.Errorf("connect to zarf registry: %w", err)
	}

	options := []crane.Option{}
	if zarfState.RegistryInfo.ShouldUseMTLS() {
		t, err := zarfRegistryMTLSTransport(ctx, c)
		if err != nil {
			return err
		}
		options = append(options, crane.WithTransport(t))
	}

	if tunnel != nil {
		logrus.Infof("Opening tunnel to Zarf registry at %s", registryEndpoint)
		defer tunnel.Close()
		return tunnel.Wrap(func() error {
			return pruneImagesFromRegistry(ctx, options, zarfState, zarfPackages, registryEndpoint)
		})
	}

	return pruneImagesFromRegistry(ctx, options, zarfState, zarfPackages, registryEndpoint)
}

func zarfRegistryMTLSTransport(ctx context.Context, c *zarfcluster.Cluster) (http.RoundTripper, error) {
	certs, err := c.GetRegistryClientMTLSCert(ctx)
	if err != nil {
		return nil, err
	}
	return pki.TransportWithKey(certs)
}

func pruneImagesFromRegistry(ctx context.Context, options []crane.Option, s *state.State, zarfPackages []state.DeployedPackage, registryEndpoint string) error {
	_ = ctx
	options = append(options, images.WithPushAuth(s.RegistryInfo))

	logrus.Info("Finding images to prune")

	// Collect digests of all images referenced by deployed packages
	pkgImages := map[string]bool{}
	for _, pkg := range zarfPackages {
		deployedComponents := map[string]bool{}
		for _, depComponent := range pkg.DeployedComponents {
			deployedComponents[depComponent.Name] = true
		}
		for _, component := range pkg.Data.Components {
			if _, ok := deployedComponents[component.Name]; !ok {
				continue
			}
			for _, image := range component.GetImages() {
				transformedImage, err := transform.ImageTransformHostWithoutChecksum(registryEndpoint, image)
				if err != nil {
					return err
				}
				digest, err := crane.Digest(transformedImage, options...)
				if err != nil {
					if isManifestUnknownError(err) {
						logrus.Warnf("Image manifest not found in registry, skipping: %s", transformedImage)
						continue
					}
					return err
				}
				pkgImages[digest] = true
			}
		}
	}

	// List all images currently in the registry
	imageCatalog, err := crane.Catalog(registryEndpoint, options...)
	if err != nil {
		return err
	}
	referenceToDigest := map[string]string{}
	for _, image := range imageCatalog {
		imageRef := fmt.Sprintf("%s/%s", registryEndpoint, image)
		tags, err := crane.ListTags(imageRef, options...)
		if err != nil {
			return err
		}
		for _, tag := range tags {
			taggedRef := fmt.Sprintf("%s:%s", imageRef, tag)
			digest, err := crane.Digest(taggedRef, options...)
			if err != nil {
				if isManifestUnknownError(err) {
					logrus.Warnf("Image manifest not found in registry, skipping: %s", taggedRef)
					continue
				}
				return err
			}
			referenceToDigest[taggedRef] = digest
		}
	}

	// Find images in the registry that are not referenced by any package
	imageDigestsToPrune := map[string]bool{}
	for digestRef, digest := range referenceToDigest {
		if pkgImages[digest] {
			continue
		}
		refInfo, err := transform.ParseImageRef(digestRef)
		if err != nil {
			return err
		}
		imageDigestsToPrune[fmt.Sprintf("%s@%s", refInfo.Name, digest)] = true
	}

	if len(imageDigestsToPrune) == 0 {
		logrus.Info("No images to prune")
		return nil
	}

	logrus.Infof("Pruning %d image(s) from Zarf registry", len(imageDigestsToPrune))
	for digestRef := range imageDigestsToPrune {
		if err := crane.Delete(digestRef, options...); err != nil {
			return err
		}
		logrus.Debugf("Pruned image: %s", digestRef)
	}
	return nil
}

func isManifestUnknownError(err error) bool {
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		for _, d := range transportErr.Errors {
			if d.Code == transport.ManifestUnknownErrorCode {
				return true
			}
		}
	}
	return false
}
