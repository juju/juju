High Availability (HA)
======================


High Availability in general terms means that we have 3 or more (up to 7) set up State Machines, 
each one can be used as the master.

This is an overview of how it works:

### Mongo
_Mongo_ is always started in [replicaset mode](http://docs.mongodb.org/manual/replication/):

 If not in HA, this will behave as if it ware a single mongodb and, in practical terms there is no difference with a regular setup.

### Ensure availability

There is a `ensure-availabiity` command for juju, it takes `-n` (minimum number of state machines) as an optional parameter, if its not provided it will default to **3**:

 This needs to be an odd number in order to prevent ties on the voting.
 
 The number can not be larger than seven (making the current possibilities: **3**, **5** and **7**) due to limitations of mongodb, which cannot have more than **7** replica set members.
 
 The number can be increased but not decreased: In the first case juju will bring up as many machines as necessary to meet the requirement, in the second nothing will happent since the rule tries to have _"at least that many"_
 
 At present there is no way to reduce the number of machines, you can kill by hand enough machines to reduce to a number you need, but this is risky and **not recommended**. If you kill less than half of the machines (half+1 remaining) running `ensure-availability` again will fix the set, if you kill more there is no way to fix it as there are not enough voting machines.
 
 Ensure availability will report what is going to be done after running it. 

### The API 

 There is an API server running on all State Machines, all of these talk to the master mongo db.
 
 The units will talk to any of the APIs.
 
 The first of the list to be tried is chosen at random, succesful connection's address will be stored to be used as first on subsequent connections.

### The peergrouper:
 
 The most relevant bits are in `worker/peergrouper/desired.go`

 It looks at the current state and decides what the peergroup members should look like, it continually tries to maintain those members

 The reason for its existence is that it can often take a while for mongo to allow a peer group change, so we can't change it directly in the API

 Its worker loop continally watches 

 1. The current set of state servers 
 2. The addresses of the current state servers 
 3. The status of the current mongo peergroup
 
It feeds all that information into `desiredPeerGroup`, which provides the peer group that we want to be and continually tries to set that peer group in mongo until it succeeds.
 
**NOTE:** There is one current situation that doesn't work currently which is that if you've only got one state server, you can't switch to another one 
