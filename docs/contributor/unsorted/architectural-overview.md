(architectural-overview)=
# Juju architectural overview


## Audience

This document is targeted at new developers of Juju, and may be useful to experienced
developers who need a refresher on some aspect of Juju's operation. It is deliberately
light on detail, because the precise mechanisms of various components' operation are
expected to change much faster than the general interactions between components.


## The view From space

A Juju model is a distributed system comprising:

* A data store (MongoDB/DQLite) which describes the desired state of the world, in terms
  of running workloads or *applications*, and the *relations* between them; and of the
  *units* that comprise those applications, and the *machines* on which those units run.
* A bunch of *agents*, each of which runs the `jujud` binary, and which are
  variously responsible for causing reality to converge towards the idealised world-
  state encoded in the data store.
* Some number of *clients* which talk over an API, implemented by the agents, to
  update the desired world-state (and thereby cause the agents to update the world
  to match). The `juju` binary is one of many possible clients; the `juju-dashboard` web
  application, JIMM, and the Juju terraform provider, are other examples.

The whole system depends upon a substrate, or *provider*, which supplies the compute,
storage, and network resources used by the workloads (and by Juju itself; but never
forget that *everything* described in this document is merely supporting infrastructure
geared towards the successful deployment and configuration of the workloads that solve
actual problems for actual users).

## Juju components

Here's the various high level parts of Juju system and how they interact:

```
                   +--------------------------+            +------------------------+
                   |                          |            |                        |
                   |  Machine agent           |            |  Unit agent            |
                   |         +-------------+  |            |       +-------------+  |
                   |         |             |  |            |       |             |  |
                   |         |   workers   |  |            |       |   workers   |  |
                   |         |             |  |            |       |             |  |
                   |         +-----------+-+  |            |       +-------+-----+  |
                   |                     |    |            |               |        |
                   +--------------------------+            +------------------------+
                                         |                                 |
                                         |   Juju API                      |
                                         |       +-------------------------+
                                         |       |
                                         |       |
                  +-----------------------------------------------------------------+
                  |                      |       |                                  |
                  |  Controller agent    |       |                                  |
+------------+    |                     +v-------v----+            +-------------+  |
|            |    |                     |             |  Juju API  |             |  |
|   Client   +-------------------------->  apiserver  +<-----------+   workers   |  |
|            |    |   Juju API          |             |            |             |  |
+------------+    |                     +------+------+            +------+------+  |
                  |                            |                          |         |
                  |                            |                          |         |
                  |                      +-----v-----+             +------v------+  |
                  |                      |           |             |             |  |
                  |                      |   state   |             |  providers  |  |
                  |                      |           |             |             |  |
                  |                      +-----+-----+             +------+------+  |
                  |                            |                          |         |
                  +-----------------------------------------------------------------+
                                               | MongoDB protocol         | cloud API
                                               |                          |
                                         +-----v-----+          +---------V---------+
                                         |           |          |                   |
                                         |  MongoDB  |          |  cloud/substrate  |
                                         |           |          |                   |
                                         +-----------+          +-------------------+
```

At the centre is a *controller agent*. It is responsible for maintaining the
state for one or more Juju models and runs a server which provides the Juju API.
Juju's state is kept in MongoDB/DQLite. Juju's state may only be accessed by the
controller agents.

A controller agent runs a number of *workers*, many of which are specific to
controller tasks. Some workers in the controller agent use the Juju *provider*
implementation to communicate with the underlying cloud substrate using the
substrate's APIs. This is how cloud resources are created, managed and
destroyed.

Almost all workers will interact with Juju's state using Juju's API, even
workers running within a controller agent.

If a Juju deployment has high-availability enabled there will be multiple
controller agents. A consumer of the Juju API may connect to any controller
agent. In HA mode, there will be a MongoDB/DQLite instance on each controller
machine. The data on these instances will replicated between the nodes.

Each Juju deployed machine runs a *machine agent*. Each machine agent runs a
number of workers.

A controller agent is a machine agent with extra responsibilities. It runs all
the workers which a normal machine runs as well as controller specific workers.

A *unit agent* runs for each deployed unit of an application. It is mainly
responsible for installing, running and maintaining charm code. It runs a
different set of workers to a machine agent.

There are a number of *clients* which interact with Juju using the Juju
API. These include the `juju` command line tool.


## The Mongo data store (aka "state")

There's a lot of *detail* to cover, but there's not much to say from an architectural
standpoint. We use a mongodb replicaset to support HA; we use the `mgo` package from
`labix.org` to implement multi-document transactions; we make use of the transaction
log to detect changes to particular documents, and convert them into business-object-
level events that get sent over the API to interested parties.

The mongodb databases run on machines we refer to as *controllers*, and are only
accessed by agents running on those machines; it's important to keep it locked down
(and, honestly, to lock it down further and better than we currently have).

There's some documentation on how to work with [the state package](hacking-state.md);
and plenty more on the [state entities](lifecycles.md) and the details of their
[creation](entity-creation.md) and [destruction](death-and-destruction.md) from various
perspectives; but there's not a lot more to say in this context.

It *is* important to understand that the transaction-log watching is not an ideal
solution, and we'll be retiring it at some point, in favour of an in-memory model
of state and a pub-sub system for watchers; we *know* it's a scalability problem,
but we're not devoting resources to it until it becomes more pressing.

Code for dealing with mongodb is found primarily in the `state`, `state/watcher`,
`replicaset`, and `worker/peergrouper` packages.


## API

Juju controllers expose an API endpoint over a websocket connection. Related
API endpoints are grouped into a set called a "facade". Each facade serves
either the client, the controller, or Juju agents. Each worker that needs to
talk to the API server will get its own facade (for example, `Provisioner`,
`Upgrader`, and `Uniter`, each used by the eponymous worker types).

The API server is implemented in the `apiserver` top level package. Facades
live inside the `apiserver/facades` package. There is a subpackage `agent` for
agent facades, `client` for client facades, and `controller` for controller
facades.

Various facades share functionality; for example, the Life method is used by
many worker facades. In these cases, the method is implemented on a separate
type in the `apiserver/common` package, which can be added as a field to the
facade type, and then the common method can be exposed on the facade. Note that
any public functions on the facade become part of the facade's API.

Each facade is separately versioned. For any Juju version, you can see the list
of supported versions for each facade in `api/facadeversions.go`. When using
two different versions of Juju, they will "negotiate" to find a version of the
facade which is supported by both. A facade's version must be incremented when
the interface changes, and old facade versions can generally only be removed in
a new major version. You can use the `facadecheck` script in the `scripts`
folder to compare supported facade versions for any two versions of Juju.

Moving forward all APIs *should* be implemented such that they can be called in
bulk.

The Juju API client is implemented under the `api` top level package. Client
side API facade are implemented as subpackages underneath `api`.


## The Agents

Agents all use the `jujud` binary, and all follow roughly the same model. When
starting up, they authenticate with an API server; possibly reset their
password, if the one they used has been stored persistently somewhere and is
thus vulnerable; determine their responsibilities; and run a set of tasks in
parallel until one of those tasks returns an error indicating that the agent
should either restart or terminate completely. Tasks that return any other error
will be automatically restarted after a short delay; tasks that return nil are
considered to be complete, and will not be restarted until the whole process is.

When comparing the unit agent with the machine agent, the truth of the above
may not be immediately apparent, because the responsibilities of the unit
agent are so much less varied than those of the machine agent; but we have
scheduled work to integrate the unit agent into the machine agent, rendering
each unit agent a single worker task within its responsible machine agent. It's
still better to consider a unit agent to be a simplistic and/or degenerate
implementation of a machine agent than to attach too much importance to the
differences.

### Jobs, Runners, and Workers

Machine agents all have at least one of two jobs: JobHostUnits and JobManageModel.
Each of these jobs represents a number of tasks the agent needs to execute to
fulfil its responsibilities; in addition, there are a number of tasks that are
executed by every machine agent. The terms *task* and *worker* are generally used
interchangeably in this document and in the source code; it's possible but not
generally helpful to draw the distinction that a worker executes a task. All
tasks are implemented by code in some subpackage of the `worker` package, and the
`worker.Runner` type implements the retry behaviour described above.

It's useful to note that the Runner type is itself a worker, so we can and do
nest Runners inside one another; the details of *exactly* how and where a given
worker comes to be executed are generally elided in this document; but it's worth
being aware of the fact that all the workers that use an API connection share a
single one, mediated by a single Runner, such that when the API connection fails
that single Runner can stop all its workers; shut itself down; be restarted by
its parent worker; and set up a new API connection, which it then uses to start
all its child workers.

Please note that the lists of workers below should *not* be assumed to be
exhaustive. Juju evolves, and the workers evolve with it.

### Common workers

All agents run workers with the following responsibilities:

* Check for scheduled upgrades for their binaries, and replace themselves
  (implemented in `worker/upgrader`)
* Watch logging config, and reconfigure the local logger (`worker/logger`; yes,
  we know; it is not the stupidest name in the codebase)
* Watch and store the latest known addresses for the controllers
  (`worker/apiaddressupdater`)

### Machine Agent Workers

Machine agents additionally do the following:

* Run upgrade code in the new binaries once they're replaced themselves
  (implemented directly in the machine agent's `upgradeWorker` method)
* Handle SIGABRT and permanently stop the agent (`worker/terminationworker`)
* Handle the machine entity's death and permanently stop the agent (`worker/machiner`)
* Watch proxy config, and reconfigure the local machine (`worker/machineenvironmentworker`)
* Watch for contained LXC or KVM machines and provision/decommission them
  (`worker/provisioner`)

All machine agents have JobHostUnits. These run the `worker/deployer` code which
watches for units assigned to the machine, and deploys/recalls upstart configs
for their respective unit agents as the units are assigned/removed. We expect
the deployer implementation to change to just directly run the unit agents'
workers in its own Runner.

### Controller Workers

Machines with JobManageModel also run a number of other workers, which do
the following.

* Run the API server used by all other workers (in this, and other, agents:
  `state/apiserver`)
* Provision/decommission provider instances in response to the creation/
  destruction of machine entities (`worker/provisioner`, just like the
  container provisioners run in all machine agents anyway)
* Manipulate provider networks in response to units opening/closing ports,
  and users exposing/unexposing applications (`worker/firewaller`)
* Update network addresses and associated information for provider instances
  (`worker/instancepoller`)
* Respond to queued DB cleanup events (`worker/cleaner`)
* Maintain the MongoDB replica set (`worker/peergrouper`)
* Resume incomplete MongoDB transactions (`worker/resumer`)

Many of these workers are wrapped as "singular" workers, which only run on the
same machine as the current MongoDB replicaset master. When the master changes,
the state connection is dropped, causing all those workers to also be stopped;
when they're restarted, they won't run because they're no longer running on the
master.

### Unit Agents

Unit agents run all the common workers, and the `worker/uniter` task as well;
this task is probably the single most forbiddingly complex part of Juju. (Side
note: It's a unit-er because it deals with units, and we're bad at names; but
it's also a unite-r because it's where all the various components of Juju come
together to run actual workloads.) It's sufficiently large that it deserves its
own top-level heading, below.


## The Uniter

At the highest level, the Uniter is a state machine. After a "little bit" of setup,
it runs a tight loop in which it calls `Mode` functions one after another, with the
next mode run determined by the result of its predecessor. All mode functions are
implemented in `worker/uniter/modes.go`, which is actually pretty small: just a hair
over 400 lines.

It's deliberately implemented as conceptually single-threaded (just like almost
everything else in Juju -- rampaging concurrency is the root of much evil, and so
we save ourselves a huge number of headaches by hiding concurrency behind event
channels and handling a single event at a time), but this property has degraded
over time; in particular, the `RunListener` code can inject events at unhelpful
times, and while the `hookLock` *probably* renders this safe it's still deeply
suboptimal, because the fact of the concurrency requires that we be *extremely*
careful with further modifications, lest they subtly break assumptions. We hope
to address this by retiring the current implementation of `juju run`, but it's
not entirely clear how to do this; in the meantime, Here Be Dragons.

Leaving these woes aside, the mode functions make use of two fundamental components,
which are glommed together until someone refactors it to make more sense. There's
the `Filter`, which is responsible for communicating with the API server (and the
rest of the outside world) such that relevant events can be delivered to the mode
func via channels exposed on the filter; and then there's the `Uniter` itself, which
exposes a number of methods that are expected to be called by the mode funcs.


### Uniter Modes

XXXX


### Hook contexts

XXXX


### The Relation Model

XXXX


## The Providers

A Juju provider represents a different possible kind of substrate on which a
Juju model can run, and (as far as possible) abstracts away the differences
between them, by making them all conform to the Environ interface. The most
important thing to understand about the various providers is that they're all
implemented without reference to broader Juju concepts; they are squeezed into
a shape that's convenient WRT allowing Juju to make use of them, but if we
allow Juju-level concepts to infect the providers we will suffer greatly,
because we will open a path by which changes to *Juju* end up causing changes
to *all the providers at once*.

However, we lack the ability to enforce this at present, because the package
dependency flows in the wrong direction, thanks primarily (purely?) to the
StateInfo method on Environ; and we jam all sorts of gymnastics into the state
package to allow us to use Environs without doing so explicitly (see the
state.Policy interface, and its many somewhat-inelegant uses). In other places,
we have (quite reasonably) moved code out of the environs package (see both
environs/config.Config, and instances.Instance).

Environ implementations are expected to be goroutine-safe; we don't currently
make much use of that property at the moment, but we will be coming to depend
upon it as we move to eliminate the wasteful proliferation of Environ instances
in the API server.

It's important to note that an environ Config will generally contain sensitive
information -- a user's authentication keys for a cloud provider -- and so we
must always be careful to avoid spreading those around further than we need to.
Basically, if an environ config gets off a controller, we've screwed up.
