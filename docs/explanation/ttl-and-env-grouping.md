# TTL and Environment Grouping

Kelm models an ephemeral environment as a group of namespaces that share the same `kelm.riftonix.io/env.name`.

This allows applications with multiple namespaces to expire as one unit. For example, a preview environment can contain an API namespace, a database namespace, and a worker namespace. If they all use the same environment name, Kelm starts one countdown and deletes them together.

## Why the Maximum Values Win

Kelm uses maximum values when it builds the group:

- The maximum TTL prevents one namespace with a shorter TTL from deleting the whole environment too early.
- The latest creation timestamp and update timestamp represent the newest known activity in the group.
- The maximum replenish ratio keeps the most permissive group setting.

This behavior makes the environment group conservative: when namespace parts disagree, Kelm keeps the group alive for the longest calculated lifetime.

## Event Recalculation

Kelm does not keep namespace configuration as a static snapshot. Namespace events cause a recalculation for the affected environment group. This means changing labels or annotations can move a namespace into a group, remove it from management, or extend the group lifetime.

The periodic resync protects against missed watch events and reconstructs all countdowns from current cluster state.

