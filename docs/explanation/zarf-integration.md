# Zarf Integration

Kelm can remove Zarf packages before deleting expired namespaces.

This path is intended for ephemeral environments deployed as Zarf packages. It allows package-level cleanup before Kelm force-deletes the remaining namespaces.

## Activation

Zarf integration is active only when `ZARF_ENABLED=true`.

For a managed namespace to use the Zarf path, it must include:

```yaml
labels:
  zarf.dev/agent: "enabled"
annotations:
  zarf.dev/package.name: "package-name"
```

If `zarf.dev/agent=enabled` is present without `zarf.dev/package.name`, Kelm rejects the namespace as invalid.

### Namespace-overridden packages

The same Zarf package can be deployed more than once with a different namespace
each time, using Zarf's `--namespace` override at deploy time. Each override
deployment is tracked as a separate secret in the cluster
(`zarf-package-<package-name>-override-<namespace>` instead of
`zarf-package-<package-name>`).

Zarf's `--namespace` override always deploys the whole package into a single
namespace, so that namespace's own name is always the correct override value —
there is nothing to configure. Kelm first looks for the deployment secret keyed
by the managed namespace's name, and falls back to the plain (non-overridden)
secret if that doesn't exist. This also means Kelm doesn't depend on Zarf having
annotated the namespace itself: when `zarf package deploy --namespace <ns>`
creates `<ns>` automatically, Zarf only adds `app.kubernetes.io/managed-by: zarf`
to it, with no Kelm-specific annotations at all.

## Removal Flow

When the environment expires, Kelm:

1. Looks up the deployed Zarf package from cluster state.
2. Removes the package with the Zarf Go API.
3. Attempts to prune unused images from the Zarf internal registry.
4. Force-deletes the environment namespaces.

If package removal fails with a not-found error, Kelm treats the package as already removed. Other package removal errors are logged, and Kelm attempts to delete the Zarf package secret from `ZARF_NAMESPACE`.

## Permissions

The Helm chart grants broader cluster permissions when Zarf integration is enabled because Zarf package removal may need to delete Kubernetes resources created by Helm charts and custom resources.

