(high-availability-cont)=
# High Availability (HA)

High Availability in general terms means that we have 3 or more (up to 7)
State Machines, each one of which can be used as the master.

This is an overview of how it works:

## Mongo
_Mongo_ is always started in [replicaset mode](http://docs.mongodb.org/manual/replication/).

 If not in HA, this will behave as if it were a single mongodb and, in practical
terms there is no difference with a regular setup.

## Voting

A voting member of the replicaset is a one that has a say in which member is master.

A non-voting member is just a storage backup.

Currently we don't support non-voting members; instead when a member is non-voting it
means that said controller is going to be removed entirely.

## Ensure availability

There is a `ensure-availabiity` command for juju, it takes `-n` (minimum number
 of state machines) as an optional parameter; if it's not provided it will
default to 3.

 This needs to be an odd number in order to prevent ties during voting.

 The number cannot be larger than seven (making the current possibilities: 3,
5 and 7) due to limitations of mongodb, which cannot have more than 7
replica set voting members.

 Currently the number can be increased but not decreased (this is planned).
In the first case Juju will bring up as many machines as necessary to meet the
requirement; in the second nothing will happen since the rule tries to have
_"at least that many"_

 At present there is no way to reduce the number of machines, you can kill by
hand enough machines to reduce to a number you need, but this is risky and
**not recommended**. If you kill less than half of the machines (half+1
remaining) running `enable-ha` again will add more machines to
replace the dead ones. If you kill more there is no way to recover as there
are not enough voting machines.

 The EnableHA API call will report will report the changes that it
made to the model, which will shortly be reflected in reality
### The API

 There is an API server running on all State Machines, these talk to all
the peers but queries and updates are addressed to the mongo master instance.

 Unit and machine agents connect to any of the API servers, by trying to connect
to all the addresses concurrently, but not simultaneously. It starts to try each
address in turn after a short delay. After a successful connection, the
connected address will be stored; it will be tried first when next connecting.

### The peergrouper worker:

 It looks at the current state and decides what the peergroup members should
look like and continually tries to maintain those members.

 The reason for its existence is that it can often take a while for mongo to
allow a peer group change, so we can't change it directly in the
EnableHA API call

 Its worker loop continally watches

 1. The current set of controllers
 2. The addresses of the current controllers
 3. The status of the current mongo peergroup

It feeds all that information into `desiredPeerGroup`, which provides the peer
group that we want to be and continually tries to set that peer group in mongo
until it succeeds.

**NOTE:** There is one situation which currently doesn't work which is
that if you've only got one controller, you can't switch to another one.

### The Singleton Workers

**Note:** This section reflects the current behavior of these workers but
should by no means be taken as an example to follow since most (if not all)
should run concurrently and are going to change in the near future.

The following workers require only a single instance to be running
at any one moment:

 * The environment provisioner
 * The firewaller
 * The charm revision updater
 * The state cleaner
 * The transaction resumer
 * The minunits worker

When a machine agent connects to the state, it decides whether
it is on the same instance as the mongo master instance, and
if so, it runs the singleton workers; otherwise it doesn't run them.

Because we are using `mgo.Strong` consistency semantics,
it's guaranteed that our mongo connection will be dropped
when the master changes, which means that when the
master changes, the machine agent will reconnect to the
state and choose whether to run the singleton workers again.

It also means that we can never accidentally have two
singleton workers performing operations at the same time.
