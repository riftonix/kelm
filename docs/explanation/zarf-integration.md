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

## Removal Flow

When the environment expires, Kelm:

1. Looks up the deployed Zarf package from cluster state.
2. Removes the package with the Zarf Go API.
3. Attempts to prune unused images from the Zarf internal registry.
4. Force-deletes the environment namespaces.

If package removal fails with a not-found error, Kelm treats the package as already removed. Other package removal errors are logged, and Kelm attempts to delete the Zarf package secret from `ZARF_NAMESPACE`.

## Permissions

The Helm chart grants broader cluster permissions when Zarf integration is enabled because Zarf package removal may need to delete Kubernetes resources created by Helm charts and custom resources.

