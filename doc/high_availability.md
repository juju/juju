High Availability (HA)
======================

Juju can be run in high availability (HA) mode. In HA mode Juju runs on 
multiple controller instances making it resilient to outages.

HA mode is invoked via the `enable-ha` command. By default, this will ensure 
three controllers, but using the `-n` flag allows running with five or seven
controllers.

The number of controllers can be reduced by invoking `remove-machine` with a 
controller machine ID, and increased by re-running the `enable-ha` command.

### Dqlite

Each controller is a [Dqlite] node. The `dbaccessor` worker on each controller is 
responsible for maintaining the Dqlite cluster. When entering HA mode, the 
`dbaccessor` worker will configure the local Dqlite node as a member of the 
cluster.

When starting Dqlite, the worker must bind it to an IP address on the local
machine. It is a requirement that there is a unique local-cloud-scoped address
for Dqlite to use. If there is no unique address, new nodes will not be joined 
to the cluster until one can be determined. See _Controller Charm_ below.

Beyond joining nodes to the cluster, Juju does not manage Dqlite node roles.
This is handled within Dqlite itself. A cluster will have one leader, voters
which are eligible to participate in leader elections, stand-bys and spares.

Juju does not predicate any logic on node roles.

If the number of controller instances is reduced to one, the `dbaccessor` 
worker detects this scenario and proactively reconfigure the cluster to be 
constituted by the local node only.

### Controller Charm

The controller charm propagates binding information to the `dbaccessor` worker.
It coordinates with other controller units via the `db-cluster` peer relation.
If there are multiple potential bind addresses for a Dqlite node, the user must
supply an endpoint binding for the relation using a space that ensures a unique
IP address.

### API Addresses for Agents

When machines in the control plane change, agent configuration files are 
re-written with usable API addresses from all controllers. Agents will try
these addresses in random order when connecting, so that an inaccessible 
controller just results in connection to another.

The list of addresses supplied to agent configuration can be influenced by the
`juju-mgmt-space` controller configuration article. This is supplied with a
space name in order that agent-controller communication can be isolated to 
specific networks.

### API Addresses for Clients

Each time the Juju client establishes a connection to the Juju controller, it
is sent the current list of API addresses and updates these in the local store. 
The client's first connection attempt is always to the last address that it 
used successfully. Others are tried subsequently if required.

Addresses used by clients are not influenced by the `juju-mgmt-space` 
configuration.

### Singular Workers

Many workers, such as the `dbaccessor` worker above, run on all controller 
nodes, but there are some workers that must have exactly one instance running. 
An obvious example of this is a model's compute provisioner - we don't want 
multiple actors attempting to start a cloud instance for a new machine.

The controller that such singular workers run on is determined by the lease
sub-system. See the appropriate documentation for more information on leases.

[Dqlite]: https://dqlite.io/