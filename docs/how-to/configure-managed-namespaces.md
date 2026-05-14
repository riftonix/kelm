# Configure Managed Namespaces

Use labels and annotations to opt a namespace into Kelm management.

## Manage a Single Namespace

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: preview-app
  labels:
    kelm.riftonix.io/managed: "true"
    kelm.riftonix.io/env.name: "preview-app"
  annotations:
    kelm.riftonix.io/ttl.removal: "1h"
    kelm.riftonix.io/ttl.replenishRatio: "0.75"
    kelm.riftonix.io/ttl.notificationFactors: "[0.5,0.8,0.9]"
    kelm.riftonix.io/updateTimestamp: "2026-05-13T10:00:00Z"
```

## Group Multiple Namespaces

Set the same `kelm.riftonix.io/env.name` on every namespace that belongs to the same ephemeral environment:

```yaml
metadata:
  name: preview-app-api
  labels:
    kelm.riftonix.io/managed: "true"
    kelm.riftonix.io/env.name: "preview-app"
```

```yaml
metadata:
  name: preview-app-db
  labels:
    kelm.riftonix.io/managed: "true"
    kelm.riftonix.io/env.name: "preview-app"
```

Kelm deletes all namespaces in the group together.

## Extend an Environment

Update the timestamp on one or more namespaces in the group:

```sh
kubectl annotate namespace preview-app-api \
  kelm.riftonix.io/updateTimestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --overwrite
```

Kelm uses the maximum `updateTimestamp` across the environment group.

## Configure Zarf Package Removal

When Zarf integration is enabled, add the Zarf markers to the managed namespace:

```yaml
metadata:
  labels:
    kelm.riftonix.io/managed: "true"
    kelm.riftonix.io/env.name: "preview-app"
    zarf.dev/agent: "enabled"
  annotations:
    zarf.dev/package.name: "preview-app"
```

Kelm removes the Zarf package before forcing namespace deletion.

