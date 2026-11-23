---
myst:
  html_meta:
    description: "Juju controller reference: what a controller is made of, its storage, nodes, Dqlite cluster, API addresses, access, configuration, and destruction."
---

(controller)=
# Controller

```{ibnote}
See also:

- {ref}`manage-controllers`
- {ref}`high-availability`
```

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the bootstrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`) -- each of which includes
    - a {ref}`unit agent <unit-agent>`,
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code, and
    - a {ref}`controller agent <controller-agent>` running, among other things, the Juju API server and an in-process embedded [Dqlite](https://canonical.com/dqlite) {ref}`database <database>`. <p>
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in the internal Dqlite {ref}`database <database>`.

(controller-storage)=
## Controller storage

A Juju controller has two basic persistent storage needs: {ref}`database <database>` access and blob storage. By default, the blob store is a file store: the blobs are files in the `objectstore` directory under the controller agent's data directory. No controller configuration key selects the blob store.

<!-- ADD BACK IN WHEN WE MOVE THOSE VALUES INTO SECRETS.

However, either during bootstrap or later, you can (and, in a production-setting, should!) specify any S3-compatible object store you want (e.g., AWS S3, MicroCeph, MinIO, etc.) using the object-store-related controller configuration keys ({ref}`controller-config-object-store-type`, {ref}`controller-config-object-store-s3-endpoint`, {ref}`controller-config-object-store-s3-static-key`, {ref}`controller-config-object-store-s3-static-secret`, {ref}`controller-config-object-store-s3-static-session`, {ref}`controller-config-object-store-s3-region`).

Also, Juju will apply default S3 policy permissions, but you are free to change them, so long as they satisfy the following as a minimum (at least, during model creation):

```text
{
   "Version" : "2012-10-17",
   "Statement" : [
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:CreateBucket",
            "s3:PutBucketPolicy",
            "s3:PutBucketTagging",
            "s3:PutBucketVersioning",
            "s3:PutBucketObjectLockConfiguration"
         ],
         "Resource" : "arn:aws:s3:::*"
      },
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:ListBucket",
            "s3:ListBucketVersions",
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
            "s3:GetBucketPolicy",
            "s3:GetBucketTagging",
            "s3:GetBucketVersioning",
            "s3:GetBucketObjectLockConfiguration",
            "s3:GetObject",
            "s3:GetObjectLegalHold",
            "s3:GetObjectRetention",
            "s3:PutObject",
            "s3:PutObjectLegalHold",
            "s3:BypassGovernanceRetention",
            "s3:PutObjectRetention",
            "s3:DeleteObject"
         ],
         "Resource" : "arn:aws:s3:::*"
      }
   ]
}
``` -->

(controller-nodes)=
## Controller nodes

A controller is a unit of the `controller` application in the controller model. Each unit has its own controller node, which has a Dqlite node. Juju identifies a controller node by the number of its unit: the node of unit `controller/2` has the controller ID `2`.

The number of controllers is the number of units of the `controller` application, and running more than one makes the controller highly available. Dqlite decides how many of the controllers vote (see {ref}`Dqlite cluster of the controllers <dqlite-cluster-of-the-controllers>`).

(dqlite-cluster-of-the-controllers)=
## Dqlite cluster of the controllers

Each controller is a [Dqlite] node. The `dbaccessor` worker on each controller is
responsible for maintaining the Dqlite cluster. When entering HA mode, the
`dbaccessor` worker will configure the local Dqlite node as a member of the
cluster.

When starting Dqlite, the worker must bind it to an IP address. The address is
read from the controller configuration file populated by the controller charm.
If there is no address to use for binding, the worker will wait for one to be
written to the file before attempting to join the cluster.
See _Controller Charm_ below.

Each Dqlite node has a role within the cluster. Juju does not set node
roles; this is handled within Dqlite itself, with its defaults of three voters
and three stand-bys. A cluster is constituted by:
- one _leader_ to which all database reads and writes are redirected,
- up to two other _voters_ that participate in leader elections,
- up to three _stand-bys_, which do not vote but take over if a voter fails; and
- _spares_, which are the nodes beyond these.

With four controllers there are therefore three voters and one stand-by; with
more than six, the extra nodes are spares.

For a machine in the controller model, `juju status --format yaml` shows the Dqlite role of its controller node in `controller-cluster-role`: `voter`, `standby` or `spare`. The role is read from Dqlite when `status` is called; a node that is down is not reported as down. `has-vote` and `wants-vote` are both `true` for a `voter` and for a `standby`, although a stand-by does not vote in Dqlite, so use `controller-cluster-role` to tell them apart.

If the number of controller instances is reduced to one, and the one remaining
controller's own address is the only address in the cluster configuration, the
`dbaccessor` worker reconfigures the cluster with the local node as the only
member and restarts. It does this only when Dqlite on that node is not running.

(controller-charm)=
## Controller charm

The controller charm propagates bind addresses to the `dbaccessor` worker by
writing them to the controller configuration file. Each controller unit shares
its resolved bind address with the other units via the `dbcluster` peer
relation. The charm must be able to determine a unique address in the
local-cloud scope before it is shared with other units and written to the
configuration file. If no unique address can be determined, the user must supply
an endpoint binding for the relation using a space that ensures a unique IP
address.

(controller-api-addresses-for-agents)=
## Controller API addresses for agents

On the controller, the `apiaddresssetter` worker publishes the addresses of each
controller node, in three separate sets: for clients, for agents and for the
other controllers. It recomputes them when the controller nodes, the controller
configuration or the controller's network facts change.

When machines in the control plane change, the `api-address-updater` worker
for each agent re-writes the agent's configuration file with usable API
addresses from all controllers. Agents will try these addresses in random order
until they establish a successful controller connection.

The list of addresses supplied to agent configuration, and the addresses that
controllers use to reach each other, can be influenced by the
`juju-mgmt-space` controller configuration value. This is supplied with a space
name so that agent-controller communication can be isolated to specific
networks.

(controller-api-addresses-for-clients)=
## Controller API addresses for clients

Each time the Juju client establishes a connection to the Juju controller, the
controller sends the current list of API addresses and the client updates these
in the local store. The client's first connection attempt is always to the last
address that it used successfully. Others are tried subsequently if required.

Addresses used by clients are not influenced by the `juju-mgmt-space`
configuration.

(controller-ssh-server-host-key-and-port)=
## Controller SSH server host key and port

A caller with `login` access can read the controller's SSH server host key and port. When no port has been set, it is 17022. The `ssh-server-port` configuration key is not read for this.

(controller-version-and-identity-provider)=
## Controller version and identity provider

Any user can read the version and git commit of the running controller binary, and the identity provider URL, which is the value of the `identity-url` configuration key, or empty.

(controller-access)=
## Controller access

Every controller operation is available only to a connection authenticated as a user. A permission failure is the error `unauthorized access` (code `unauthorized access`).

| Operation | Permission required |
| --- | --- |
| Set controller configuration, destroy the controller, remove blocks, list blocked models, list all models, modify a user's controller access | `superuser` on the controller |
| Read the SSH server host key and port | `login` on the controller |
| Read the controller configuration, the controller version, the identity provider URL | none beyond being a user |
| Read a user's controller access | `superuser` for any user's access; any other caller can read only their own, and a failure is reported per entry |

(adding-a-controller)=
## Adding a controller

A controller is added by adding units to the `controller` application in the controller model. The number to add is the count of new units, not the total number of controllers wanted. This is how controller high availability is enabled: the `enable-ha` command was removed in Juju 4.0.

The caller needs write access to the controller model, and the operation is refused while a change block is set on it. The new unit becomes a {ref}`controller node <controller-nodes>` and joins the {ref}`Dqlite cluster <dqlite-cluster-of-the-controllers>`.

```{ibnote}
See more: {ref}`make-a-controller-highly-available`
```

(removing-a-controller)=
## Removing a controller

A controller is removed by removing its unit from the `controller` application, or the machine that hosts the unit. Removing a machine also removes the units on it. The caller needs write access to the controller model, and the operation is refused while a remove block is set on it. The remaining controllers receive the updated addresses before the removed node disappears.

Juju does not check against removing the last controller by removing its unit or machine.

| Error | Code | Trigger | Remedy |
| --- | --- | --- | --- |
| `cannot scale controller application to 0 units` | none | The number of units of the `controller` application is set to 0. | Keep at least one unit. |

Juju also refuses to remove the `controller` application.

(reading-and-setting-controller-configuration)=
## Reading and setting controller configuration

The controller keeps its configuration in its Dqlite database. Any user can read the whole configuration; the controller filters no key from the result.

Only a caller with `superuser` access can set configuration. No change block is consulted. The request names keys and values; keys that it does not name are unchanged, and a key cannot be removed.

Only the keys that can change after bootstrap can be set: 32 keys, among them `auditing-enabled`, the `audit-log-*`, `agent-logfile-*` and `query-tracing-*` keys, `features`, `juju-mgmt-space`, `public-dns-address`, `max-debug-log-duration` and `ssh-max-concurrent-connections`. `api-port`, `ssh-server-port`, `controller-uuid` and `ca-cert` are not among them. The complete list is in {ref}`list-of-controller-configuration-keys`.

The controller applies these rules in order: the caller must be a superuser; if the request names `caas-image-repo`, the rules for that key apply (below); every key is validated and its value coerced to the key's type; then all the values are written in one transaction. If any key fails, the whole call fails and nothing changes.

| Error | Code | Trigger | Remedy |
| --- | --- | --- | --- |
| `unauthorized access` | `unauthorized access` | The caller is not a superuser. | Ask a superuser. |
| `unknown controller config setting "<key>"` | none | The name is not a controller configuration key. | Use a key from the key list. |
| `can not change "<key>" after bootstrap` | none | The key is a controller key but is not among the keys that can change after bootstrap. | None: the key is set only at bootstrap. |
| `unable to coerce controller config key "<key>"` | none | The value does not convert to the key's type. | Give a value of the key's type. |

The key `caas-image-repo` has further rules:

| Error | Code | Trigger |
| --- | --- | --- |
| `cannot parse caas-image-repo: ...` | `not valid` | The value cannot be parsed. |
| `caas-image-repo expected a string got ...` | `not valid` | The value is not a string. |
| `cannot change caas-image-repo as it is not currently set` | `not valid` | The key has no value yet. |
| `cannot change caas-image-repo: repository read-only, only authentication can be updated` | none | The value changes the repository part. |
| `cannot change caas-image-repo: unable to add authentication details` | none | The value adds authentication details. |
| `cannot change caas-image-repo: unable to remove authentication details` | none | The value removes authentication details. |

A change to `query-tracing-enabled`, `query-tracing-threshold` or `dqlite-busy-timeout` makes the controller agent restart itself. A change to the audit keys, `juju-mgmt-space`, `api-port`, `ssh-max-concurrent-connections` or `caas-image-repo` applies without a restart. The effect of the other keys was not traced.

```{ibnote}
See more: {ref}`manage-controllers`, {ref}`list-of-controller-configuration-keys`
```

(destroying-a-controller)=
## Destroying a controller

Only a `superuser` can destroy a controller, and the connection must be to the controller model. The request carries `destroy-models`, `destroy-storage`, `force` and `max-wait`. Every created model other than the controller model is hosted, including one that is already dying.

`destroy-storage` has three states: absent, which does not destroy storage, so persistent storage refuses the call; false, which releases the storage; and true, which destroys it. `force` absent is false and `max-wait` absent is zero.

The controller applies these checks in order.

1. The caller must be a superuser.
2. The connection must be to the controller model.
3. A block of any kind on any hosted model refuses the call.
4. If `destroy-models` is false and a hosted model exists, the call is refused.
5. A destroy, remove or change block on the controller model refuses the call.
6. Unless `force` is true, a controller model without a valid cloud credential refuses the call.

Because check 3 runs before check 4, a blocked hosted model gives `operation is blocked` even when `destroy-models` is false.

When the checks pass, the controller marks the controller model as dying, unless a migration import is active for it, and schedules the removal job of the controller model. It then removes the hosted models one by one. The call returns when the removals are scheduled; it does not wait for them.

A call can fail after it has started. When a hosted model has persistent storage and `destroy-storage` is absent, the controller model is already dying, the hosted model that refused is already dying, and the hosted models before it are already scheduled for removal. The remedy is to call again with `destroy-storage` set.

| Error | Code | Trigger | Remedy |
| --- | --- | --- | --- |
| `unauthorized access` | `unauthorized access` | The caller is not a superuser. | Ask a superuser. |
| `current model is not the controller model` | `bad request` | The connection is not to the controller model. | Connect to the controller model. |
| `found blocks in controller models` | `operation is blocked` | A hosted model has a block of any kind. | Remove the blocks (`enable-destroy-controller` removes all blocks of all models). |
| `cannot destroy controller with hosted models` | `controller has hosted models` | `destroy-models` is false and a hosted model exists, including one that is dying. | Set `destroy-models`. |
| the message set with the block | `operation is blocked` | The controller model has a destroy, remove or change block. | Remove the block. |
| `invalid cloud credential, use --force` | none | The controller model has no valid cloud credential and `force` is false. | Set `force`. |
| `hosted model "<uuid>" has persistent storage` | `controller/model has persistent storage` | A hosted model has persistent storage and `destroy-storage` is absent. | Set `destroy-storage` to true or false. |

```{ibnote}
See more: {ref}`manage-controllers`
```

[Dqlite]: https://dqlite.io/
