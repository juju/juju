---
myst:
  html_meta:
    description: "Juju controller reference: control plane architecture, database, high availability, and multi-cloud management capabilities."
---

(controller)=
# Controller

```{ibnote}
See also:

- {ref}`manage-controllers`
- {ref}`high-availability`
```

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the boostrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`) -- each of which includes
    - a {ref}`unit agent <unit-agent>`,
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code, and
    - a {ref}`controller agent <controller-agent>` running, among other things, the Juju API server and an in-process embedded [Dqlite](https://canonical.com/dqlite) {ref}`database <database>`. <p>
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in the internal Dqlite {ref}`database <database>`.

(controller-storage)=
## Controller storage

A Juju controller has two basic persistent storage needs: {ref}`database <database>` access and blob storage. By default, Juju will use the filesystem of the controller's supporting infrastructure.

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

A controller is made highly available by running more than one unit of the `controller` application in the controller model. Each unit has its own controller node, which has a Dqlite node. Juju identifies a controller node by the number of its unit: the node of unit `controller/2` has the controller ID `2`.

The number of controllers is the number of units of the `controller` application. Juju refuses only the value 0. Dqlite decides how many of the controllers vote (see {ref}`Dqlite cluster of the controllers <dqlite-cluster-of-the-controllers>`).

### Adding a controller

A controller is added by adding a unit to the `controller` application in the controller model, with `juju add-unit -m controller controller -n <number of units to add>`. The number is the count of new units, not the total number of controllers wanted. The caller needs write access to the controller model, and the operation is refused while a change block is set on it.

The new unit's machine starts a controller agent. Its `dbaccessor` worker creates the controller node, binds Dqlite to the address that the controller charm has written to the configuration file, and joins the cluster using the addresses of the other controllers.

```{ibnote}
See more: {ref}`make-a-controller-highly-available`
```

### Removing a controller

A controller is removed by removing its unit from the `controller` application, or the machine that hosts the unit. Removing a machine also removes the units on it. The caller needs write access to the controller model, and the operation is refused while a remove block is set on it.

When the removal reaches the unit's deletion, and only after the unit has left its relation scopes, Juju deletes the controller node: it marks it dead, clears its Dqlite ID and addresses, and deletes the rows for its addresses, agent version, password and nonce. The remaining controllers therefore receive the updated bind addresses before the node disappears. The record of the node itself is kept as a tombstone so that a late worker cannot recreate it.

Juju refuses to scale the `controller` application to 0 units, with the error `cannot scale controller application to 0 units`. It refuses to remove the `controller` application. For the removal of a unit or a machine, no check against removing the last controller was found in the code.

### Controller node roles in `status`

For a machine in the controller model, `juju status --format yaml` shows the Dqlite role of its controller node in `controller-cluster-role`: `voter`, `standby` or `spare`. The role is read from Dqlite when `status` is called; a node that is down is not reported as down.

`has-vote` and `wants-vote` are both `true` for a `voter` and for a `standby`, although a stand-by does not vote in Dqlite. Use `controller-cluster-role` to tell them apart.

`status` reads the controller ID as a machine name, but removal and the address setter read it as a unit number. The two agree while unit `controller/N` is on machine `N`; they can differ otherwise.

### Listing controllers and their addresses

The `HighAvailability` API facade, version 3, has the call `ControllerDetails`. It requires `login` access to the controller. It returns, for each controller with client API addresses, the controller ID and those addresses, sorted by ID as strings (so `10` is listed before `2`). If there are no addresses the list is empty and there is no error. Addresses shared by all controllers (for example for Kubernetes) are not listed. The `juju show-status-log` and `juju debug-log` commands use it to reach every controller.

### Enabling controller high availability

Controller high availability is enabled by adding units (see above). The `enable-ha` command was removed in Juju 4.0. The facade call `EnableHA` still exists, in versions 2 and 3, and always returns the error `enable HA not supported` (code `not supported`). It makes no change.

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

(controller-single-instance-workers)=
## Controller single-instance workers

Many workers, such as the `dbaccessor` worker, run on all controller instances,
but there are some workers that must run on exactly one controller instance.
An obvious example of this is a model's compute provisioner - we would never
want more than one actor attempting to start a cloud instance for a new
machine.

Single instance workers are those declared in the model manifolds configuration
that use the `isResponsible` decorator. This in turn is based on a flag set by the
`singular` worker.

The `singular` worker only sets the flag if it is the current lease holder for
the `singular-controller` namespace. See the appropriate documentation for more
information on leases.

[Dqlite]: https://dqlite.io/
