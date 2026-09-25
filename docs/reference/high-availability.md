---
myst:
  html_meta:
    description: "High availability in Juju: multi-replica controllers, database replication, and resilient application deployment on machines and Kubernetes."
---

(high-availability)=
# High availability (HA)

```{ibnote}
See also:

- {ref}`make-a-controller-highly-available`
- {ref}`make-an-application-highly-available`
```

In the context of a cloud deployment in general, **high availability (HA)** is the concept of making software resilient to failures by means of running multiple replicas with shared and synchronised software context -- something usually achieved through coordinated {ref}`scaling (out and up) <scaling>`. In Juju, it is supported for controllers on machine clouds and for regular applications on both machine and Kubernetes clouds

Note: Controller high availability is currently only supported on machine clouds -- it is not supported (along with backup and restore) for Kubernetes controllers. See {ref}`manage-controllers`.


```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset
:alt: Three machine nodes side by side, each running a controller agent with an embedded Dqlite database, connected by replicate-arrows between the databases.
:caption: Topology: Controller high availability (machine clouds). Juju controllers can be made highly-available by enabling more than one machine to each run a separate controller unit with a separate controller agent instance, where each machine effectively becomes an instance of the controller. This set of Juju agents collectively use a Dqlite database replicaset to achieve data synchronisation amongst them.
```

Controller and agent behaviour when running controllers in HA mode:

## Dqlite

Each controller is a [Dqlite] node. The `dbaccessor` worker on each controller is
responsible for maintaining the Dqlite cluster. When entering HA mode, the
`dbaccessor` worker will configure the local Dqlite node as a member of the
cluster.

When starting Dqlite, the worker must bind it to an IP address. The address is
read from the controller configuration file populated by the controller charm.
If there is no address to use for binding, the worker will wait for one to be
written to the file before attempting to join the cluster.
See _Controller Charm_ below.

Each Dqlite node has a role within the cluster. Juju does not manage node
roles; this is handled within Dqlite itself. A cluster is constituted by:
- one _leader_ to which all database reads and writes are redirected,
- up to two other _voters_ that participate in leader elections,
- _stand-bys_; and
- _spares_.

If the number of controller instances is reduced to one, the `dbaccessor`
worker detects this scenario and reconfigures the cluster with the local node
as the only member.

## Controller charm

The controller charm propagates bind addresses to the `dbaccessor` worker by
writing them to the controller configuration file. Each controller unit shares
its resolved bind address with the other units via the `db-cluster` peer
relation. The charm must be able to determine a unique address in the
local-cloud scope before it is shared with other units and written to the
configuration file. If no unique address can be determined, the user must supply
an endpoint binding for the relation using a space that ensures a unique IP
address.

## API addresses for agents

When machines in the control plane change,  the `api-address-updater` worker
for each agent re-writes the agent's configuration file with usable API
addresses from all controllers. Agents will try these addresses in random order
until they establish a successful controller connection.

The list of addresses supplied to agent configuration can be influenced by the
`juju-mgmt-space` controller configuration value. This is supplied with a space
name so that agent-controller communication can be isolated to specific
networks.

## API addresses for clients

Each time the Juju client establishes a connection to the Juju controller, the
controller sends the current list of API addresses and the client updates these
in the local store. The client's first connection attempt is always to the last
address that it used successfully. Others are tried subsequently if required.

Addresses used by clients are not influenced by the `juju-mgmt-space`
configuration.

## Single instance workers

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
