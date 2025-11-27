# Kelm

**Kelm** is a Kubernetes operator for automated lifecycle management of namespaces. It monitors namespaces labeled as managed, applies time-to-live (TTL) policies, and automatically deletes or notifies about namespaces when their TTL expires. The operator is designed for ephemeral environments, such as test or preview environments, to ensure resources are cleaned up efficiently.

## Features

- **Automatic Namespace Cleanup:** Deletes managed namespaces after a configurable TTL.
- **Namespace group support (enviroments):** Easily manage ttl, when your app need more than 1 namespace
- **Webooks support:** No constant polling, kelm reacts only to webhooks
- **Kubernetes Native:** Integrates with Kubernetes using standard labels and annotations.

## Getting Started

### Prerequisites

- Go 1.24+
- Access to a Kubernetes cluster (with `kubectl` configured)
- [Helm](https://helm.sh/) (for Helm-based deployment)

### Build and Run Locally

Clone the repository and build the operator:

```sh
git clone https://github.com/your-org/kelm.git
cd kelm
go build -o kelm ./cmd
```

Run the operator (requires access to your kubeconfig):

```sh
./kelm
```

### Deploy with Helm

A sample Helm chart is provided in `test/helm/`. To deploy managed namespaces and the operator:

```sh
cd test/helm
helm install kelm .
```

You can customize the namespaces and their policies in `values.yaml`.

## Namespace Configuration

Namespaces are managed by labeling them with `kelm.riftonix.io/managed: "true"` and providing TTL and notification settings via annotations.

Example configuration (`test/helm/values.yaml`):

```yaml
namespaces:
  - name: app1-test
    labels:
      kelm.riftonix.io/managed: "true"
      kelm.riftonix.io/env.name: "test"
    annotations:
      kelm.riftonix.io/ttl.removal: "30m"
      kelm.riftonix.io/ttl.replenishRatio: "0.75"
      kelm.riftonix.io/ttl.notificationFactors: "[0.5,0.8,0.9]"
      kelm.riftonix.io/updateTimestamp: "2025-11-27T10:42:54Z"
```
- **kelm.riftonix.io/managed:** Set to true, if you want to manage namespace.
- **kelm.riftonix.io/env.name:"** Your env name. You can set same name on multiple namespaces and kelm ensures that the namespaces are removed at the same time.
- **kelm.riftonix.io/ttl.removal:** TTL before namespace removal (e.g., "30s", "1h").
- **kelm.riftonix.io/ttl.replenishRatio:** Ratio for TTL replenishment on activity.
- **kelm.riftonix.io/ttl.notificationFactors:** [Currently not supported] When to send notifications before deletion (as fractions of TTL).
- **kelm.riftonix.io/updateTimestamp:** Set your creation/update time. In this way, you can extend the lifespan of the environment.

## How It Works

1. The operator watches for namespaces labeled as managed.
2. For each namespace group (enviroment), it starts a countdown based on the TTL.
3. When TTL expires, the enviroment is force-deleted.
4. The operator continuously watches for changes and recalculates timers as needed.

## Development

Dependencies are managed with Go modules. See [`go.mod`](go.mod:1) for details.

Run tests with:

```sh
go test ./...
```

## License

This project is licensed under the terms of the [LICENSE](LICENSE) file.
