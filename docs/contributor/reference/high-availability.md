(high-availability-cont)=
# High Availability (HA)

See first: [Juju user docs | How to make a controller highly available]

This document details controller and agent behaviour when running controllers
in
HA mode.

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

[Juju user docs | How to make a controller highly available]: https://juju.is/docs/juju/manage-controllers#heading--make-a-controller-highly-available
[Dqlite]: https://dqlite.io/
