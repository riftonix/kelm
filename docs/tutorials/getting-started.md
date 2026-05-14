# Getting Started

This tutorial deploys Kelm with Helm and configures one managed namespace.

## Prerequisites

- A Kubernetes cluster.
- `kubectl` configured for the target cluster.
- Helm.

## Deploy Kelm

Install the chart from the repository root:

```sh
helm install kelm ./helm
```

The default chart deploys the operator image from `ghcr.io/riftonix/kelm`.

## Create a Managed Namespace

Create a namespace with the labels and annotations Kelm requires:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: app1-test
  labels:
    kelm.riftonix.io/managed: "true"
    kelm.riftonix.io/env.name: "app1-test"
  annotations:
    kelm.riftonix.io/ttl.removal: "30m"
    kelm.riftonix.io/ttl.replenishRatio: "0.75"
    kelm.riftonix.io/ttl.notificationFactors: "[0.5,0.8,0.9]"
    kelm.riftonix.io/updateTimestamp: "2026-05-13T10:00:00Z"
```

Apply it:

```sh
kubectl apply -f namespace.yaml
```

## Verify the Operator

Check the operator logs:

```sh
kubectl logs deploy/kelm
```

Kelm watches namespaces with `kelm.riftonix.io/managed=true`, groups them by `kelm.riftonix.io/env.name`, and starts a deletion countdown for each environment group.

## Extend the Lifetime

Update `kelm.riftonix.io/updateTimestamp` to a newer timestamp:

```sh
kubectl annotate namespace app1-test \
  kelm.riftonix.io/updateTimestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --overwrite
```

Kelm receives the namespace event, recalculates the environment group, and restarts the countdown.

