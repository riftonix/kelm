# Labels and Annotations

Kelm manages only namespaces that match the required Kelm contract.

## Labels

| Key | Required | Description |
|---|---:|---|
| `kelm.riftonix.io/managed` | yes | Must be set to `"true"` for Kelm to manage the namespace. |
| `kelm.riftonix.io/env.name` | yes | Environment group name. Namespaces with the same value are deleted together. |
| `zarf.dev/agent` | no | Set to `"enabled"` to mark a namespace as Zarf-managed when Zarf integration is enabled. |

## Annotations

| Key | Required | Description |
|---|---:|---|
| `kelm.riftonix.io/ttl.removal` | yes | TTL before environment removal. Uses Go duration syntax such as `30m`, `1h`, or `24h`. |
| `kelm.riftonix.io/ttl.replenishRatio` | yes | Ratio used by Kelm when calculating replenishment behavior. Kelm stores the maximum value across the environment group. |
| `kelm.riftonix.io/ttl.notificationFactors` | yes | JSON array of notification factors. The current operator parses and stores the values, but notification delivery is not implemented. |
| `kelm.riftonix.io/updateTimestamp` | yes | Creation or update timestamp used to extend the environment lifetime. |
| `zarf.dev/package.name` | required for Zarf namespaces | Zarf package name to remove when the environment expires. |

## Ignored Namespaces

By default Kelm ignores:

- `default`
- `kube-system`
- `kube-node-lease`
- `kube-public`

Set `IGNORED_NAMESPACES` to override the list.

