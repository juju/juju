(worker-dev)=
# Worker
<!---To see previous major version of this doc, see version 13.-->

> See first: [Juju | Worker](https://juju.is/docs/juju/worker)

In Juju, a **worker** is any type that implements [the worker interface](worker-interface.md).

Examples of workers include [the dependency engine](dependency-package.md#newengine), instances run by the dependency
engine (the typical usage of the term "worker"),
and [watchers](). <!-- TODO: this link was broken in original doc in discourse -->

Note: A Juju [agent](agent.md) runs one or more workers at the same time in parallel. A worker may run / be run by
another worker.

<!--
In Juju, a **worker** is, in its simplest form, a goroutine. Almost always, it watches for events and performs or dispatches work based on those events. (This is the essence of an agent-based system.) We never proactively emit events to workers – they’re just always watching and responding to changes (perform tasks based on the declared Juju status; the workers responsible for a unit / the uniter they watch state and then fire hooks to the charm).**
-->

## List of workers run by the dependency engine

In Juju, the term "worker" is most commonly used to denote types whose instances are run by the dependency engine.

> The most important workers to know about are: the [`uniter`](#uniter), the [`deployer`](#deployer),
> the [`provisioner`](#provisioner), and the [
`caasapplicationprovisioner`](#caasapplicationprovisioner), the [`charmdownloader`](#charmdownloader),
> and the [`undertaker`](#undertaker).

- [Worker](#worker)
  - [List of workers run by the dependency engine](#list-of-workers-run-by-the-dependency-engine)
  - [`agent`](#agent)
  - [`agentconfigupdater`](#agentconfigupdater)
  - [`apiaddressupdater`](#apiaddressupdater)
  - [`apicaller`](#apicaller)
  - [`apiconfigwatcher`](#apiconfigwatcher)
  - [`apiserver`](#apiserver)
  - [`apiservercertwatcher`](#apiservercertwatcher)
  - [`applicationscaler`](#applicationscaler)
  - [`auditconfigupdater`](#auditconfigupdater)
  - [`authenticationworker`](#authenticationworker)
  - [`caasadmission`](#caasadmission)
  - [`caasapplicationprovisioner`](#caasapplicationprovisioner)
  - [`caasbroker`](#caasbroker)
  - [`caasenvironupgrader`](#caasenvironupgrader)
  - [`caasfirewaller`](#caasfirewaller)
  - [`caasfirewallersidecar`](#caasfirewallersidecar)
  - [`caasmodelconfigmanager`](#caasmodelconfigmanager)
  - [`caasmodeloperator`](#caasmodeloperator)
  - [`caasoperator`](#caasoperator)
  - [`caasoperatorprovisioner`](#caasoperatorprovisioner)
  - [`caasprober`](#caasprober)
  - [`caasrbacmapper`](#caasrbacmapper)
  - [`caasunitprovisioner`](#caasunitprovisioner)
  - [`caasunitsmanager`](#caasunitsmanager)
  - [`caasunitterminationworker`](#caasunitterminationworker)
  - [`caasupgrader`](#caasupgrader)
  - [`centralhub`](#centralhub)
  - [`certupdater`](#certupdater)
  - [`changestream`](#changestream)
  - [`charmdownloader`](#charmdownloader)
  - [`charmrevision`](#charmrevision)
  - [`cleaner`](#cleaner)
  - [`common`](#common)
  - [`containerbroker`](#containerbroker)
  - [`controllerport`](#controllerport)
  - [`credentialvalidator`](#credentialvalidator)
  - [`dbaccessor`](#dbaccessor)
  - [`deployer`](#deployer)
  - [`diskmanager`](#diskmanager)
  - [`environ`](#environ)
  - [`environupgrader`](#environupgrader)
  - [`externalcontrollerupdater`](#externalcontrollerupdater)
  - [`fanconfigurer`](#fanconfigurer)
  - [`filenotifywatcher`](#filenotifywatcher)
  - [`firewaller`](#firewaller)
  - [`fortress`](#fortress)
  - [`gate`](#gate)
  - [`hostkeyreporter`](#hostkeyreporter)
  - [`httpserver`](#httpserver)
  - [`httpserverargs`](#httpserverargs)
  - [`identityfilewriter`](#identityfilewriter)
  - [`instancemutater`](#instancemutater)
  - [`instancepoller`](#instancepoller)
  - [`introspection`](#introspection)
  - [`leadership`](#leadership)
  - [`lease`](#lease)
  - [`leaseexpiry`](#leaseexpiry)
  - [`lifeflag`](#lifeflag)
  - [`logforwarder`](#logforwarder)
  - [`logger`](#logger)
  - [`logsender`](#logsender)
  - [`machineactions`](#machineactions)
  - [`machiner`](#machiner)
  - [`machineundertaker`](#machineundertaker)
  - [`meterstatus`](#meterstatus)
  - [`metrics`](#metrics)
  - [`metricworker`](#metricworker)
  - [`migrationflag`](#migrationflag)
  - [`migrationmaster`](#migrationmaster)
  - [`migrationminion`](#migrationminion)
  - [`mocks`](#mocks)
  - [`modelcache`](#modelcache)
  - [`modelworkermanager`](#modelworkermanager)
  - [`multiwatcher`](#multiwatcher)
  - [`muxhttpserver`](#muxhttpserver)
  - [`peergrouper`](#peergrouper)
  - [`provisioner`](#provisioner)
  - [`proxyupdater`](#proxyupdater)
  - [`pruner`](#pruner)
  - [`pubsub`](#pubsub)
  - [`querylogger`](#querylogger)
  - [`reboot`](#reboot)
  - [`remoterelations`](#remoterelations)
  - [`retrystrategy`](#retrystrategy)
  - [`s3caller`](#s3caller)
  - [`secretbackendrotate`](#secretbackendrotate)
  - [`secretdrainworker`](#secretdrainworker)
  - [`secretexpire`](#secretexpire)
  - [`secretrotate`](#secretrotate)
  - [`simplesignalhandler`](#simplesignalhandler)
  - [`singular`](#singular)
  - [`state`](#state)
  - [`stateconfigwatcher`](#stateconfigwatcher)
  - [`stateconverter`](#stateconverter)
  - [`storageprovisioner`](#storageprovisioner)
  - [`syslogger`](#syslogger)
  - [`terminationworker`](#terminationworker)
  - [`toolsversionchecker`](#toolsversionchecker)
  - [`undertaker`](#undertaker)
  - [`unitassigner`](#unitassigner)
  - [`uniter`](#uniter)
  - [`upgradedatabase`](#upgradedatabase)
  - [`upgrader`](#upgrader)
  - [`upgradeseries`](#upgradeseries)
  - [`upgradesteps`](#upgradesteps)


## `agent`
> See more: [`juju/worker/agent`](https://github.com/juju/juju/tree/3.3/worker/agent)

## `agentconfigupdater`
> See more: [`juju/worker/agentconfigupdater`](https://github.com/juju/juju/tree/3.3/worker/agentconfigupdater)

## `apiaddressupdater`

The `apiaddressupdater` worker watches and stores the controllers' addresses.

> See more: [`juju/worker/apiaddressupdater`](https://github.com/juju/juju/tree/3.3/worker/apiaddressupdater)

## `apicaller`
> See more: [`juju/worker/apicaller`](https://github.com/juju/juju/tree/3.3/worker/apicaller)

## `apiconfigwatcher`

> See more: [`juju/worker/apiconfigwatcher`](https://github.com/juju/juju/tree/3.3/worker/apiconfigwatcher)

## `apiserver`
> See more: [`juju/worker/apiserver`](https://github.com/juju/juju/tree/3.3/worker/apiserver)

## `apiservercertwatcher`
> See more: [`juju/worker/apiservercertwatcher`](https://github.com/juju/juju/tree/3.3/worker/apiservercertwatcher)

## `applicationscaler`
> See more: [`juju/worker/applicationscaler`](https://github.com/juju/juju/tree/3.3/worker/applicationscaler)

## `auditconfigupdater`
> See more: [`juju/worker/auditconfigupdater`](https://github.com/juju/juju/tree/3.3/worker/auditconfigupdater)

## `authenticationworker`
> See more: [`juju/worker/authenticationworker`](https://github.com/juju/juju/tree/3.3/worker/authenticationworker)

## `caasadmission`
> See more: [`juju/worker/caasadmission`](https://github.com/juju/juju/tree/3.3/worker/caasadmission)

## `caasapplicationprovisioner`

The `caasapplicationprovisioner` worker is actually two workers:

- `provisioner`: Watches a Kubernetes model and starts a new worker of the `appWorker` type whenever an application is
  created.
- `appWorker`: Drives the Kubernetes provider to create, manage, and destroy Kubernetes resources to match a requested
  state. Also writes the state of created resources (application/unit status, application/unit IP addresses & ports,
  filesystem info, etc.)  back into the database.

> See more: [
`juju/worker/caasapplicationprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasapplicationprovisioner)

## `caasbroker`
> See more: [`juju/worker/caasbroker`](https://github.com/juju/juju/tree/3.3/worker/caasbroker)

## `caasenvironupgrader`
> See more: [`juju/worker/caasenvironupgrader`](https://github.com/juju/juju/tree/3.3/worker/caasenvironupgrader)

## `caasfirewaller`
> See more: [`juju/worker/caasfirewaller`](https://github.com/juju/juju/tree/3.3/worker/caasfirewaller)

## `caasfirewallersidecar`
> See more: [`juju/worker/caasfirewallersidecar`](https://github.com/juju/juju/tree/3.3/worker/caasfirewallersidecar)

## `caasmodelconfigmanager`
> See more: [`juju/worker/caasmodelconfigmanager`](https://github.com/juju/juju/tree/3.3/worker/caasmodelconfigmanager)

## `caasmodeloperator`
> See more: [`juju/worker/caasmodeloperator`](https://github.com/juju/juju/tree/3.3/worker/caasmodeloperator)

## `caasoperator`
> See more: [`juju/worker/caasoperator`](https://github.com/juju/juju/tree/3.3/worker/caasoperator)

## `caasoperatorprovisioner`
> See more: [
`juju/worker/caasoperatorprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasoperatorprovisioner)

## `caasprober`
> See more: [`juju/worker/caasprober`](https://github.com/juju/juju/tree/3.3/worker/caasprober)

## `caasrbacmapper`
> See more: [`juju/worker/caasrbacmapper`](https://github.com/juju/juju/tree/3.3/worker/caasrbacmapper)

## `caasunitprovisioner`
> See more: [`juju/worker/caasunitprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasunitprovisioner)

## `caasunitsmanager`
> See more: [`juju/worker/caasunitsmanager`](https://github.com/juju/juju/tree/3.3/worker/caasunitsmanager)

## `caasunitterminationworker`
> See more: [
`juju/worker/caasunitterminationworker`](https://github.com/juju/juju/tree/3.3/worker/caasunitterminationworker)

## `caasupgrader`
> See more: [`juju/worker/caasupgrader`](https://github.com/juju/juju/tree/3.3/worker/caasupgrader)

## `centralhub`
> See more: [`juju/worker/centralhub`](https://github.com/juju/juju/tree/3.3/worker/centralhub)

## `certupdater`
> See more: [`juju/worker/certupdater`](https://github.com/juju/juju/tree/3.3/worker/certupdater)

## `changestream`
> See more: [`juju/worker/changestream`](https://github.com/juju/juju/tree/3.3/worker/changestream)

## `charmdownloader`
> See more: [`juju/worker/charmdownloader`](https://github.com/juju/juju/tree/3.3/worker/charmdownloader)

## `charmrevision`

The charm revision updater worker is responsible for polling Charmhub every 24 hours to check if there are new revisions
available of any repository charm deployed in the model. If so, it will put a document in the Juju database, so that the
next time the user runs `juju status` they can see that there is an update available. This worker also sends anonymised
usage metrics to Charmhub when it polls.

This worker doesn't contain much business logic - most of the work is delegated to the facade call.

> See more: [`juju/worker/charmrevision`](https://github.com/juju/juju/tree/3.3/worker/charmrevision)

## `cleaner`

The `cleaner` worker handles database clean-up events.

> See more: [`juju/worker/cleaner`](https://github.com/juju/juju/tree/3.3/worker/cleaner)

## `common`
> See more: [`juju/worker/common`](https://github.com/juju/juju/tree/3.3/worker/common)

## `containerbroker`

The `containerbroker` worker's sole responsibility is to manage the lifecycle of an instance-broker.
Configuration of the instance-broker relies on talking to the provisioner to ensure that we correctly configure the
correct availability zones. Failure to do so, will result in an error.

The instance-broker is created for LXD types only -- any other container types cause the worker to uninstall itself.

> See more: [`juju/worker/containerbroker`](https://github.com/juju/juju/tree/3.3/worker/containerbroker)

## `controllerport`
> See more: [`juju/worker/controllerport`](https://github.com/juju/juju/tree/3.3/worker/controllerport)

## `credentialvalidator`
> See more: [`juju/worker/credentialvalidator`](https://github.com/juju/juju/tree/3.3/worker/credentialvalidator)

## `dbaccessor`
> See more: [`juju/worker/dbaccessor`](https://github.com/juju/juju/tree/3.3/worker/dbaccessor)

## `deployer`
> See more: [`juju/worker/deployer`](https://github.com/juju/juju/tree/3.3/worker/deployer)

## `diskmanager`

The `diskmanager` worker periodically lists block devices on the machine it runs on.

This worker will be run on all Juju-managed machines (one per machine agent).


> See more: [`juju/worker/diskmanager`](https://github.com/juju/juju/tree/3.3/worker/diskmanager)

## `environ`
> See more: [`juju/worker/environ`](https://github.com/juju/juju/tree/3.3/worker/environ)

## `environupgrader`
> See more: [`juju/worker/environupgrader`](https://github.com/juju/juju/tree/3.3/worker/environupgrader)

## `externalcontrollerupdater`
> See more: [
`juju/worker/externalcontrollerupdater`](https://github.com/juju/juju/tree/3.3/worker/externalcontrollerupdater)

## `fanconfigurer`
> See more: [`juju/worker/fanconfigurer`](https://github.com/juju/juju/tree/3.3/worker/fanconfigurer)

## `filenotifywatcher`
> See more: [`juju/worker/filenotifywatcher`](https://github.com/juju/juju/tree/3.3/worker/filenotifywatcher)

## `firewaller`

The `firewaller` worker modifies provider networks when a user exposes/de-exposes applications, or when a unit
closes/opens ports.

> See more: [`juju/worker/firewaller`](https://github.com/juju/juju/tree/3.3/worker/firewaller)

## `fortress`

The `fortress` worker implements a convenient metaphor for an RWLock.

A "fortress" is constructed via a manifold's Start func, and accessed via its
Output func as either a Guard or a Guest. To begin with, it's considered to be
locked, and inaccessible to Guests; when the Guard Unlocks it, the Guests can
Visit it until the Guard calls Lockdown. At that point, new Visits are blocked,
and existing Visits are allowed to complete; the Lockdown returns once all
Guests' Visits have completed.

The original motivating use case was for a component to mediate charm directory
access between the uniter and the metrics collector. The metrics collector must
be free to run its own independent hooks while the uniter is active; but metrics
hooks and charm upgrades cannot be allowed to tread on one another's toes.

> See more: [`juju/worker/fortress`](https://github.com/juju/juju/tree/3.3/worker/fortress)

## `gate`

The `gate` worker provides a mechanism by which independent workers can wait for one another to finish a task, without
introducing explicit dependencies between those workers.

> See more: [`juju/worker/gate`](https://github.com/juju/juju/tree/3.3/worker/gate)

## `hostkeyreporter`
> See more: [`juju/worker/hostkeyreporter`](https://github.com/juju/juju/tree/3.3/worker/hostkeyreporter)

## `httpserver`
> See more: [`juju/worker/httpserver`](https://github.com/juju/juju/tree/3.3/worker/httpserver)

## `httpserverargs`
> See more: [`juju/worker/httpserverargs`](https://github.com/juju/juju/tree/3.3/worker/httpserverargs)

## `identityfilewriter`
> See more: [`juju/worker/identityfilewriter`](https://github.com/juju/juju/tree/3.3/worker/identityfilewriter)

## `instancemutater`

Package `instancemutater` defines workers that compare the list of lxd profiles
applied to a machine with the list of expected profiles based on the
application versions which should be running on the machine. In particular, it
creates two workers from the same code with different configurations; the
ContainerWorker and the EnvironWorker.

The ContainerWorker runs on a machine and watches for containers to be
created on it.

```
    	┌──────────────────────────────┐
    	│       MACHINE                │
    	│                              │
    	│                              │
    	│   ┌─────────────────────┐    │
    	│   │                     │    │
    	│   │     CONTAINER       │    │
  ┌────┼───►                      │    │
  │    │   │                      │    │
  │    │   │  ┌────────────────┐  │    │
  │    │   │  │   UNIT         │  │    │
  │    │   │  │                │  │    │
  │    │   │  │                │  │    │
  │    │   │  │ ┌────────────┐ │  │    │
  │    │   │  │ │ CHARM      │ │  │    │
  │    │   │  │ │            │ │  │    │
  │    │   │  │ └─────┬──────┘ │  │    │
  │    │   │  │       │        │  │    │
  │    │   │  └───────┼────────┘  │    │
  │    │   │          │           │    │
  │    │   └──────────┼───────────┘    │
  │    │              │                │
  │    └──────────────┼────────────────┘
  │                   │
  └───────────────────┘
```

	LXD PROFILE

The EnvironWorker watches for machines in the model to be created.

```
  	┌─────────────────────────────────┐
  	│       MACHINE                   │
  	│                                 │
 ┌────►                               │
 │    │   ┌──────────────────────┐    │
 │    │   │                      │    │
 │    │   │     CONTAINER        │    │
 │    │   │                      │    │
 │    │   │                      │    │
 │    │   │  ┌────────────────┐  │    │
 │    │   │  │   UNIT         │  │    │
 │    │   │  │                │  │    │
 │    │   │  │                │  │    │
 │    │   │  │ ┌────────────┐ │  │    │
 │    │   │  │ │ CHARM      │ │  │    │
 │    │   │  │ │            │ │  │    │
 │    │   │  │ └─────┬──────┘ │  │    │
 │    │   │  │       │        │  │    │
 │    │   │  └───────┼────────┘  │    │
 │    │   │          │           │    │
 │    │   └──────────┼───────────┘    │
 │    │              │                │
 │    └──────────────┼────────────────┘
 │                   │
 └───────────────────┘
```

LXD PROFILE

To understand this better with a similar mechanism, take a look at the
provisioner worker as well.




> See more: [`juju/worker/instancemutater`](https://github.com/juju/juju/tree/3.3/worker/instancemutater)

## `instancepoller`

The `instancepoller` worker updates network addresses and any related information for providers.

> See more: [`juju/worker/instancepoller`](https://github.com/juju/juju/tree/3.3/worker/instancepoller)

## `introspection`
> See more: [`juju/worker/introspection`](https://github.com/juju/juju/tree/3.3/worker/introspection)

## `leadership`
> See more: [`juju/worker/leadership`](https://github.com/juju/juju/tree/3.3/worker/leadership)

## `lease`

Package `lease`, also known as "the manager", manages the leases used by individual Juju workers.

Workers will claim a lease, and they are either attributed (i.e., the workers gets the lease ) or blocked (i.e., the
worker is waiting for a lease to become available). In the latter case, the manager will keep track of all the blocked
claims. When a worker's lease expires or gets revoked, then the manager will re-attribute it to one of other workers,
thus unblocking them and satisfying their claim.

In the special case where a worker is upgrading an application, it will ask the manager to "pin" the lease. This means
that the lease will not expire or be revoked during the upgrade, and the validity of the lease will get refreshed once
the upgrade has completed. The overall effect is that the application unit does not lose leadership during an upgrade.

> See more: [`juju/worker/lease`](https://github.com/juju/juju/tree/3.3/worker/lease)

## `leaseexpiry`
> See more: [`juju/worker/leaseexpiry`](https://github.com/juju/juju/tree/3.3/worker/leaseexpiry)

## `lifeflag`
> See more: [`juju/worker/lifeflag`](https://github.com/juju/juju/tree/3.3/worker/lifeflag)

## `logforwarder`
> See more: [`juju/worker/logforwarder`](https://github.com/juju/juju/tree/3.3/worker/logforwarder)

## `logger`
> See more: [`juju/worker/logger`](https://github.com/juju/juju/tree/3.3/worker/logger)

The `logger` worker watches the local logger configuration and reconfigures it when needed.

## `logsender`
> See more: [`juju/worker/logsender`](https://github.com/juju/juju/tree/3.3/worker/logsender)

## `machineactions`
> See more: [`juju/worker/machineactions`](https://github.com/juju/juju/tree/3.3/worker/machineactions)

## `machiner`

The `machiner` worker terminates the agent when the machine has a fatal error.

> See more: [`juju/worker/machiner`](https://github.com/juju/juju/tree/3.3/worker/machiner)

## `machineundertaker`
> See more: [`juju/worker/machineundertaker`](https://github.com/juju/juju/tree/3.3/worker/machineundertaker)

## `meterstatus`

The `meterstatus` worker executes the meter-status-changed hook periodically.

> See more: [`juju/worker/meterstatus`](https://github.com/juju/juju/tree/3.3/worker/meterstatus)

## `metrics`
> See more: [`juju/worker/metrics`](https://github.com/juju/juju/tree/3.3/worker/metrics)

## `metricworker`
> See more: [`juju/worker/metricworker`](https://github.com/juju/juju/tree/3.3/worker/metricworker)

## `migrationflag`
> See more: [`juju/worker/migrationflag`](https://github.com/juju/juju/tree/3.3/worker/migrationflag)

## `migrationmaster`
> See more: [`juju/worker/migrationmaster`](https://github.com/juju/juju/tree/3.3/worker/migrationmaster)

## `migrationminion`
> See more: [`juju/worker/migrationminion`](https://github.com/juju/juju/tree/3.3/worker/migrationminion)

## `mocks`

The `mocks` worker contains common worker mocks.

Run `go generate` to regenerate the mock interfaces.

> See more: [`juju/worker/mocks`](https://github.com/juju/juju/tree/3.3/worker/mocks)

## `modelcache`
> See more: [`juju/worker/modelcache`](https://github.com/juju/juju/tree/3.3/worker/modelcache)

## `modelworkermanager`
> See more: [`juju/worker/modelworkermanager`](https://github.com/juju/juju/tree/3.3/worker/modelworkermanager)

## `multiwatcher`

The `multiwatcher` worker provides watchers that watch the entire model.

It is responsible for creating, feeding, and cleaning up after multiwatchers.

The core worker gets an event stream from an AllWatcherBacking, and manages the multiwatcher Store.

The behaviour of the multiwatchers is very much tied to the Store implementation. The store provides a mechanism to get
changes over time.

> See more: [`juju/worker/multiwatcher`](https://github.com/juju/juju/tree/3.3/worker/multiwatcher)

## `muxhttpserver`
> See more: [`juju/worker/muxhttpserver`](https://github.com/juju/juju/tree/3.3/worker/muxhttpserver)

## `peergrouper`

The `peergrouper` worker maintains the MongoDB replica set.

> See more: [`juju/worker/peergrouper`](https://github.com/juju/juju/tree/3.3/worker/peergrouper)

## `provisioner`

The `provisioner` worker watches LXC and KVM instances, and provisions and decommissions them when needed.

<!-- Provision and decommission instances when needed-->

> See more: [`juju/worker/provisioner`](https://github.com/juju/juju/tree/3.3/worker/provisioner)

## `proxyupdater`
> See more: [`juju/worker/proxyupdater`](https://github.com/juju/juju/tree/3.3/worker/proxyupdater)

## `pruner`
> See more: [`juju/worker/pruner`](https://github.com/juju/juju/tree/3.3/worker/pruner)

## `pubsub`
> See more: [`juju/worker/pubsub`](https://github.com/juju/juju/tree/3.3/worker/pubsub)

## `querylogger`
> See more: [`juju/worker/querylogger`](https://github.com/juju/juju/tree/3.3/worker/querylogger)

## `reboot`
> See more: [`juju/worker/reboot`](https://github.com/juju/juju/tree/3.3/worker/reboot)

## `remoterelations`

Package `remoterelations` defines workers which manage the operation of cross model relations.

- `Worker`: Top level worker. Watches SaaS applications/proxies and creates a worker for each.
- `remoteApplicationWorker`: Manages operations for a consumer or offer proxy. Consumes and publishes relation data and
  status changes.
- `remoteRelationsWorker`: Runs on the consuming model to manage relations to the offer.
- `relationUnitsWorker`: Runs on the consuming model to receive and publish changes to each relation unit data bag.

The consuming side pushes relation updates from the consumer application to the model containing
the offer. It also polls the offered application to record relation changes from the offer into
the consuming model.


> See more: [`juju/worker/remoterelations`](https://github.com/juju/juju/tree/3.3/worker/remoterelations)

## `retrystrategy`
> See more: [`juju/worker/retrystrategy`](https://github.com/juju/juju/tree/3.3/worker/retrystrategy)

## `s3caller`
> See more: [`juju/worker/s3caller`](https://github.com/juju/juju/tree/3.3/worker/s3caller)

## `secretbackendrotate`

The `secretbackendrotate` worker tracks and rotates a secret backend token.

> See more: [`juju/worker/secretbackendrotate`](https://github.com/juju/juju/tree/3.3/worker/secretbackendrotate)

## `secretdrainworker`

The `secretdrainworker` runs on the agent and drains secrets to the new active backend when the model changes secret
backends.

> See more: [`juju/worker/secretdrainworker`](https://github.com/juju/juju/tree/3.3/worker/secretdrainworker)

## `secretexpire`

The `secretexpire` worker tracks and notifies when a secret revision should expire.

> See more: [`juju/worker/secretexpire`](https://github.com/juju/juju/tree/3.3/worker/secretexpire)

## `secretrotate`

The `secretrotate` worker tracks a secret and notifies when it should be rotated.

> See more: [`juju/worker/secretrotate`](https://github.com/juju/juju/tree/3.3/worker/secretrotate)

## `simplesignalhandler`

The `simplesignalhandler` worker responds to OS signals and returns a pre-defined error from this worker when the signal
is received.

> See more: [`juju/worker/simplesignalhandler`](https://github.com/juju/juju/tree/3.3/worker/simplesignalhandler)

## `singular`
> See more: [`juju/worker/singular`](https://github.com/juju/juju/tree/3.3/worker/singular)

## `state`
> See more: [`juju/worker/state`](https://github.com/juju/juju/tree/3.3/worker/state)

## `stateconfigwatcher`
> See more: [`juju/worker/stateconfigwatcher`](https://github.com/juju/juju/tree/3.3/worker/stateconfigwatcher)

## `stateconverter`
> See more: [`juju/worker/stateconverter`](https://github.com/juju/juju/tree/3.3/worker/stateconverter)

## `storageprovisioner`

The `storageprovisioner` worker manages the provisioning and deprovisioning of storage volumes and filesystems,attaching
them to and detaching them from machines.

A `storageprovisioner` worker is run at each model manager, which manages model-scoped storage such as virtual disk
services of the cloud provider. In addition to this, each machine agent runs a machine `storageprovisioner` worker that
manages storage scoped to that machine, such as loop devices, temporary filesystems (tmpfs), and rootfs.

The `storageprovisioner` worker consists of the following major components:

- a set of watchers for provisioning and attachment events
- a schedule of pending operations
- event-handling code fed by the watcher, that identifies interesting changes (unprovisioned -> provisioned, etc.),
  ensures prerequisites are met (e.g. volume and machine are both provisioned before attachment is attempted), and
  populates operations into the schedule
- operation execution code fed by the schedule, that groups operations to make bulk calls to storage providers; updates
  status; and reschedules operations upon failure

> See more: [`juju/worker/storageprovisioner`](https://github.com/juju/juju/tree/3.3/worker/storageprovisioner)

## `syslogger`
> See more: [`juju/worker/syslogger`](https://github.com/juju/juju/tree/3.3/worker/syslogger)

## `terminationworker`

The `terminationworker` stops the agent when it has been signalled to do so.

> See more: [`juju/worker/terminationworker`](https://github.com/juju/juju/tree/3.3/worker/terminationworker)

## `toolsversionchecker`
> See more: [`juju/worker/toolsversionchecker`](https://github.com/juju/juju/tree/3.3/worker/toolsversionchecker)

## `undertaker`
> See more: [`juju/worker/undertaker`](https://github.com/juju/juju/tree/3.3/worker/undertaker)

## `unitassigner`
> See more: [`juju/worker/unitassigner`](https://github.com/juju/juju/tree/3.3/worker/unitassigner)

## `uniter`

The `uniter` worker implements the capabilities of the unit agent, for example running a charm's hooks in response to
model events. The uniter worker sets up the various components which make that happen and then runs the top level event
loop.

<!--
A unit worker, is a state-machine that manages a unit's workload operations in a unit agent. It is single-threaded, and runs a sequence of functions called *modes*.

There are two fundamental components of a unit worker:

1. *Filter* : Talks to the API server and the outside world to deliver relevant events to modes,
2. *Uniter* : Exposes a number of methods that can be called by the modes.

A unit worker is often simply referred to as a *uniter*.

-->

> See more: [`juju/worker/uniter`](https://github.com/juju/juju/tree/3.3/worker/uniter)

## `upgradedatabase`
> See more: [`juju/worker/upgradedatabase`](https://github.com/juju/juju/tree/3.3/worker/upgradedatabase)

## `upgrader`

The `upgrader` worker schedules upgrades of the agent's binary, i.e. it upgrades the agent itself.

<!-- : A worker that runs an upgraded code binary once the old binary has been replaced-->

> See more: [`juju/worker/upgrader`](https://github.com/juju/juju/tree/3.3/worker/upgrader)

## `upgradeseries`
> See more: [`juju/worker/upgradeseries`](https://github.com/juju/juju/tree/3.3/worker/upgradeseries)

## `upgradesteps`
> See more: [`juju/worker/upgradesteps`](https://github.com/juju/juju/tree/3.3/worker/upgradesteps)


<!--
* *Machine Environment Worker* : Watches proxy configuration and reconfigures the machine, >> WHICH ONE IS IT?
* *Resumer* : Resumes incomplete MongoDB transactions. >> COULDN'T FIND IT IN THE CODE.


[note]
Many Controller Agents are run in the same machine where the MongoDB replica-set master runs. When the master changes, these workers all stop.
[/note]
-->
