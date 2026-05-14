# Environment Variables

Kelm reads runtime configuration from environment variables. The Helm chart maps chart values to these variables.

| Variable | Default | Description |
|---|---|---|
| `IGNORED_NAMESPACES` | `default,kube-system,kube-node-lease,kube-public` | Comma-separated list of namespaces Kelm must ignore. Empty values fall back to defaults. |
| `ZARF_ENABLED` | `false` | Enables Zarf package removal when set to `true`. |
| `ZARF_NAMESPACE` | `zarf` | Namespace where Zarf package state secrets are stored. |
| `RETRY_DELAY` | `1h` | Delay before retrying a failed deletion. Must be a positive Go duration. |
| `WATCH_RETRY_DELAY` | `10s` | Delay before reconnecting a failed or closed Kubernetes namespace watch. Must be a positive Go duration. |
| `RESYNC_INTERVAL` | `5m` | Interval for periodic resync of managed namespaces. Must be a positive Go duration. |

Invalid duration values are logged and replaced with defaults.

