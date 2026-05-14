# Architecture

Kelm is a Kubernetes operator for lifecycle management of ephemeral namespace groups.

## Control Loop

At startup Kelm creates a Kubernetes client from in-cluster configuration. If that fails, it falls back to the local kubeconfig file for development.

Kelm then lists namespaces with:

```text
kelm.riftonix.io/managed=true
```

Each valid namespace is converted into an environment part. Parts with the same `kelm.riftonix.io/env.name` are merged into one environment group.

## Environment Grouping

An environment group can contain one or more namespaces. Kelm derives group-level values from the namespace set:

- TTL is the maximum `kelm.riftonix.io/ttl.removal` value.
- Replenish ratio is the maximum `kelm.riftonix.io/ttl.replenishRatio` value.
- Creation timestamp is the latest namespace creation timestamp.
- Update timestamp is the latest `kelm.riftonix.io/updateTimestamp` value.
- Notification factors are merged, sorted, and deduplicated.

The countdown is started for the environment group, not for each namespace independently.

## Watch and Resync

Kelm watches namespace events filtered by `kelm.riftonix.io/managed=true`.

When a namespace event arrives, Kelm cancels the existing countdown for that environment group, reads the current namespace state for the group, and starts a new countdown.

Kelm also runs a periodic resync. The resync cancels all countdowns, reads all managed namespaces again, and recreates countdowns from current cluster state.

## Deletion

When a countdown expires, Kelm force-deletes every namespace in the environment group. Namespaces currently being deleted are tracked in memory so watch events from operator-driven deletion do not immediately restart countdowns.

If deletion times out or returns an error, Kelm schedules another countdown using `RETRY_DELAY`.

## Zarf Integration

When `ZARF_ENABLED=true` and a namespace has `zarf.dev/agent=enabled`, Kelm treats the environment as Zarf-managed. The namespace must also define `zarf.dev/package.name`.

On expiration, Kelm attempts to remove the Zarf package with the Zarf Go API, then prunes unused images from the Zarf internal registry. Namespace deletion still runs after the Zarf path.

