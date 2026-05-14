# Deploy with Helm

Use this guide to install or configure Kelm through the bundled Helm chart.

## Install with Defaults

```sh
helm install kelm ./helm
```

## Set the Image Version

```sh
helm upgrade --install kelm ./helm \
  --set image.tag=0.1.4
```

## Configure Ignored Namespaces

Kelm skips ignored namespaces even when they contain Kelm labels. The chart passes this list through `IGNORED_NAMESPACES`.

```sh
helm upgrade --install kelm ./helm \
  --set microservice.envs.IGNORED_NAMESPACES="default,kube-system,kube-node-lease,kube-public,cert-manager"
```

## Configure Timing

```sh
helm upgrade --install kelm ./helm \
  --set retryDelay=30m \
  --set watchRetryDelay=10s \
  --set resyncInterval=5m
```

## Enable Zarf Integration

Enable this only for environments deployed as Zarf packages:

```sh
helm upgrade --install kelm ./helm \
  --set zarf.enabled=true \
  --set zarf.namespace=zarf
```

When Zarf integration is enabled, Kelm needs broader permissions because package removal may delete resources created by Helm charts inside Zarf packages.

