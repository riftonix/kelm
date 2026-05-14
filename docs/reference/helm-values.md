# Helm Values

## Image

| Value | Default | Description |
|---|---|---|
| `image.registry` | `ghcr.io` | Container registry. |
| `image.repository` | `riftonix/kelm` | Image repository. |
| `image.pullPolicy` | `IfNotPresent` | Kubernetes image pull policy. |
| `image.tag` | `0.1.4` | Image tag. |

## Service Account

| Value | Default | Description |
|---|---|---|
| `serviceAccount.create` | `true` | Create a service account through the chart. |
| `serviceAccount.annotations` | `{}` | Service account annotations. |
| `serviceAccount.name` | `""` | Existing service account name or generated chart name. |

## Zarf

| Value | Default | Description |
|---|---|---|
| `zarf.enabled` | `false` | Enables Zarf package removal. |
| `zarf.namespace` | `zarf` | Namespace that stores Zarf package state secrets. |

## Timing

| Value | Default | Description |
|---|---|---|
| `retryDelay` | `1h` | Delay before retrying failed namespace deletion. |
| `watchRetryDelay` | `10s` | Delay before reconnecting a closed namespace watch. |
| `resyncInterval` | `5m` | Periodic full resync interval for managed namespaces. |

## Environment

| Value | Default | Description |
|---|---|---|
| `microservice.envs.IGNORED_NAMESPACES` | `default,kube-system,kube-node-lease,kube-public,cert-manager` | Comma-separated namespace ignore list passed to the operator. |

