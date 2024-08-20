<!---To see previous major version of this doc, see version 13.-->

> See first: [Juju | Worker](https://juju.is/docs/juju/worker)

In Juju, a **worker** is any type that implements [the worker interface](/t/11723).

Examples of workers include [the dependency engine](/t/11668#heading--newengine), instances run by the dependency
engine (the typical usage of the term "worker"), and [watchers](/t/).

Note: A Juju [agent](/t/11679) runs one or more workers at the same time in parallel. A worker may run / be run by
another worker.

<!--
In Juju, a **worker** is, in its simplest form, a goroutine. Almost always, it watches for events and performs or dispatches work based on those events. (This is the essence of an agent-based system.) We never proactively emit events to workers – they’re just always watching and responding to changes (perform tasks based on the declared Juju status; the workers responsible for a unit / the uniter they watch state and then fire hooks to the charm).**
-->

**Contents:**

- [List of workers run by the dependency engine](#heading--list-of-workers-run-by-the-dependency-engine)

<a href="#heading--list-of-workers-run-by-the-dependency-engine"><h2 id="heading--list-of-workers-run-by-the-dependency-engine">
List of workers run by the dependency engine</h2></a>

In Juju, the term "worker" is most commonly used to denote types whose instances are run by the dependency engine. These
types are defined by the worker packages listeds below.

[note type=information]
The most important workers to know about are: the [`uniter`](#heading--uniter), the [`deployer`](#heading--deployer),
the [`provisioner`](#heading--provisioner), and the [
`caasapplicationprovisioner`](#heading--caasapplicationprovisioner), the [`charmdownloader`](#heading--charmdownloader),
and the [`undertaker`](#heading--undertaker).
[/note]

- [`actionpruner`](#heading--actionpruner)
- [`agent`](#heading--agent)
- [`agentconfigupdater`](#heading--agentconfigupdater)
- [`apiaddressupdater`](#heading--apiaddressupdater)
- [`apicaller`](#heading--apicaller)
- [`apiconfigwatcher`](#heading--apiconfigwatcher)
- [`apiserver`](#heading--apiserver)
- [`apiservercertwatcher`](#heading--apiservercertwatcher)
- [`applicationscaler`](#heading--applicationscaler)
- [`auditconfigupdater`](#heading--auditconfigupdater)
- [`authenticationworker`](#heading--authenticationworker)
- [`caasadmission`](#heading--caasadmission)
- [`caasapplicationprovisioner`](#heading--caasapplicationprovisioner)
- [`caasbroker`](#heading--caasbroker)
- [`caasenvironupgrader`](#heading--caasenvironupgrader)
- [`caasfirewaller`](#heading--caasfirewaller)
- [`caasfirewallersidecar`](#heading--caasfirewallersidecar)
- [`caasmodelconfigmanager`](#heading--caasmodelconfigmanager)
- [`caasmodeloperator`](#heading--caasmodeloperator)
- [`caasoperator`](#heading--caasoperator)
- [`caasoperatorprovisioner`](#heading--caasoperatorprovisioner)
- [`caasprober`](#heading--caasprober)
- [`caasrbacmapper`](#heading--caasrbacmapper)
- [`caasunitprovisioner`](#heading--caasunitprovisioner)
- [`caasunitsmanager`](#heading--caasunitsmanager)
- [`caasunitterminationworker`](#heading--caasunitterminationworker)
- [`caasupgrader`](#heading--caasupgrader)
- [`centralhub`](#heading--centralhub)
- [`certupdater`](#heading--certupdater)
- [`changestream`](#heading--changestream)
- [`charmdownloader`](#heading--charmdownloader)
- [`charmrevision`](#heading--charmrevision)
- [`cleaner`](#heading--cleaner)
- [`common`](#heading--common)
- [`containerbroker`](#heading--containerbroker)
- [`controllerport`](#heading--controllerport)
- [`credentialvalidator`](#heading--credentialvalidator)
- [`dbaccessor`](#heading--dbaccessor)
- [`deployer`](#heading--deployer)
- [`diskmanager`](#heading--diskmanager)
- [`environ`](#heading--environ)
- [`environupgrader`](#heading--environupgrader)
- [`externalcontrollerupdater`](#heading--externalcontrollerupdater)
- [`fanconfigurer`](#heading--fanconfigurer)
- [`filenotifywatcher`](#heading--filenotifywatcher)
- [`firewaller`](#heading--firewaller)
- [`fortress`](#heading--fortress)
- [`gate`](#heading--gate)
- [`hostkeyreporter`](#heading--hostkeyreporter)
- [`httpserver`](#heading--httpserver)
- [`httpserverargs`](#heading--httpserverargs)
- [`identityfilewriter`](#heading--identityfilewriter)
- [`instancemutater`](#heading--instancemutater)
- [`instancepoller`](#heading--instancepoller)
- [`introspection`](#heading--introspection)
- [`leadership`](#heading--leadership)
- [`lease`](#heading--lease)
- [`leaseexpiry`](#heading--leaseexpiry)
- [`lifeflag`](#heading--lifeflag)
- [`logforwarder`](#heading--logforwarder)
- [`logger`](#heading--logger)
- [`logsender`](#heading--logsender)
- [`machineactions`](#heading--machineactions)
- [`machiner`](#heading--machiner)
- [`machineundertaker`](#heading--machineundertaker)
- [`meterstatus`](#heading--meterstatus)
- [`metrics`](#heading--metrics)
- [`metricworker`](#heading--metricworker)
- [`migrationflag`](#heading--migrationflag)
- [`migrationmaster`](#heading--migrationmaster)
- [`migrationminion`](#heading--migrationminion)
- [`minunitsworker`](#heading--minunitsworker)
- [`mocks`](#heading--mocks)
- [`modelcache`](#heading--modelcache)
- [`modelworkermanager`](#heading--modelworkermanager)
- [`multiwatcher`](#heading--multiwatcher)
- [`muxhttpserver`](#heading--muxhttpserver)
- [`peergrouper`](#heading--peergrouper)
- [`presence`](#heading--presence)
- [`provisioner`](#heading--provisioner)
- [`proxyupdater`](#heading--proxyupdater)
- [`pruner`](#heading--pruner)
- [`pubsub`](#heading--pubsub)
- [`querylogger`](#heading--querylogger)
- [`reboot`](#heading--reboot)
- [`remoterelations`](#heading--remoterelations)
- [`retrystrategy`](#heading--retrystrategy)
- [`s3caller`](#heading--s3caller)
- [`secretbackendrotate`](#heading--secretbackendrotate)
- [`secretdrainworker`](#heading--secretdrainworker)
- [`secretexpire`](#heading--secretexpire)
- [`secretrotate`](#heading--secretrotate)
- [`simplesignalhandler`](#heading--simplesignalhandler)
- [`singular`](#heading--singular)
- [`state`](#heading--state)
- [`stateconfigwatcher`](#heading--stateconfigwatcher)
- [`stateconverter`](#heading--stateconverter)
- [`statushistorypruner`](#heading--statushistorypruner)
- [`storageprovisioner`](#heading--storageprovisioner)
- [`syslogger`](#heading--syslogger)
- [`terminationworker`](#heading--terminationworker)
- [`toolsversionchecker`](#heading--toolsversionchecker)
- [`undertaker`](#heading--undertaker)
- [`unitassigner`](#heading--unitassigner)
- [`uniter`](#heading--uniter)
- [`upgradedatabase`](#heading--upgradedatabase)
- [`upgrader`](#heading--upgrader)
- [`upgradeseries`](#heading--upgradeseries)
- [`upgradesteps`](#heading--upgradesteps)

<a href="#heading--actionpruner"><h2 id="heading--actionpruner">`actionpruner`</h2></a>
> See more: [`juju/worker/actionpruner`](https://github.com/juju/juju/tree/3.3/worker/actionpruner)

<a href="#heading--agent"><h2 id="heading--agent">`agent`</h2></a>
> See more: [`juju/worker/agent`](https://github.com/juju/juju/tree/3.3/worker/agent)

<a href="#heading--agentconfigupdater"><h2 id="heading--agentconfigupdater">`agentconfigupdater`</h2></a>
> See more: [`juju/worker/agentconfigupdater`](https://github.com/juju/juju/tree/3.3/worker/agentconfigupdater)

<a href="#heading--apiaddressupdater"><h2 id="heading--apiaddressupdater">`apiaddressupdater`</h2></a>

The `apiaddressupdater` worker watches and stores the controllers' addresses.

> See more: [`juju/worker/apiaddressupdater`](https://github.com/juju/juju/tree/3.3/worker/apiaddressupdater)

<a href="#heading--apicaller"><h2 id="heading--apicaller">`apicaller`</h2></a>
> See more: [`juju/worker/apicaller`](https://github.com/juju/juju/tree/3.3/worker/apicaller)

<a href="#heading--apiconfigwatcher"><h2 id="heading--apiconfigwatcher">`apiconfigwatcher`</h2></a>

> See more: [`juju/worker/apiconfigwatcher`](https://github.com/juju/juju/tree/3.3/worker/apiconfigwatcher)

<a href="#heading--apiserver"><h2 id="heading--apiserver">`apiserver`</h2></a>
> See more: [`juju/worker/apiserver`](https://github.com/juju/juju/tree/3.3/worker/apiserver)

<a href="#heading--apiservercertwatcher"><h2 id="heading--apiservercertwatcher">`apiservercertwatcher`</h2></a>
> See more: [`juju/worker/apiservercertwatcher`](https://github.com/juju/juju/tree/3.3/worker/apiservercertwatcher)

<a href="#heading--applicationscaler"><h2 id="heading--applicationscaler">`applicationscaler`</h2></a>
> See more: [`juju/worker/applicationscaler`](https://github.com/juju/juju/tree/3.3/worker/applicationscaler)

<a href="#heading--auditconfigupdater"><h2 id="heading--auditconfigupdater">`auditconfigupdater`</h2></a>
> See more: [`juju/worker/auditconfigupdater`](https://github.com/juju/juju/tree/3.3/worker/auditconfigupdater)

<a href="#heading--authenticationworker"><h2 id="heading--authenticationworker">`authenticationworker`</h2></a>
> See more: [`juju/worker/authenticationworker`](https://github.com/juju/juju/tree/3.3/worker/authenticationworker)

<a href="#heading--caasadmission"><h2 id="heading--caasadmission">`caasadmission`</h2></a>
> See more: [`juju/worker/caasadmission`](https://github.com/juju/juju/tree/3.3/worker/caasadmission)

<a href="#heading--caasapplicationprovisioner"><h2 id="heading--caasapplicationprovisioner">
`caasapplicationprovisioner`</h2></a>

The `caasapplicationprovisioner` worker is actually two workers:

- `provisioner`: Watches a Kubernetes model and starts a new worker of the `appWorker` type whenever an application is
  created.
- `appWorker`: Drives the Kubernetes provider to create, manage, and destroy Kubernetes resources to match a requested
  state. Also writes the state of created resources (application/unit status, application/unit IP addresses & ports,
  filesystem info, etc.)  back into the database.

> See more: [
`juju/worker/caasapplicationprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasapplicationprovisioner)

<a href="#heading--caasbroker"><h2 id="heading--caasbroker">`caasbroker`</h2></a>
> See more: [`juju/worker/caasbroker`](https://github.com/juju/juju/tree/3.3/worker/caasbroker)

<a href="#heading--caasenvironupgrader"><h2 id="heading--caasenvironupgrader">`caasenvironupgrader`</h2></a>
> See more: [`juju/worker/caasenvironupgrader`](https://github.com/juju/juju/tree/3.3/worker/caasenvironupgrader)

<a href="#heading--caasfirewaller"><h2 id="heading--caasfirewaller">`caasfirewaller`</h2></a>
> See more: [`juju/worker/caasfirewaller`](https://github.com/juju/juju/tree/3.3/worker/caasfirewaller)

<a href="#heading--caasfirewallersidecar"><h2 id="heading--caasfirewallersidecar">`caasfirewallersidecar`</h2></a>
> See more: [`juju/worker/caasfirewallersidecar`](https://github.com/juju/juju/tree/3.3/worker/caasfirewallersidecar)

<a href="#heading--caasmodelconfigmanager"><h2 id="heading--caasmodelconfigmanager">`caasmodelconfigmanager`</h2></a>
> See more: [`juju/worker/caasmodelconfigmanager`](https://github.com/juju/juju/tree/3.3/worker/caasmodelconfigmanager)

<a href="#heading--caasmodeloperator"><h2 id="heading--caasmodeloperator">`caasmodeloperator`</h2></a>
> See more: [`juju/worker/caasmodeloperator`](https://github.com/juju/juju/tree/3.3/worker/caasmodeloperator)

<a href="#heading--caasoperator"><h2 id="heading--caasoperator">`caasoperator`</h2></a>
> See more: [`juju/worker/caasoperator`](https://github.com/juju/juju/tree/3.3/worker/caasoperator)

<a href="#heading--caasoperatorprovisioner"><h2 id="heading--caasoperatorprovisioner">`caasoperatorprovisioner`</h2></a>
> See more: [
`juju/worker/caasoperatorprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasoperatorprovisioner)

<a href="#heading--caasprober"><h2 id="heading--caasprober">`caasprober`</h2></a>
> See more: [`juju/worker/caasprober`](https://github.com/juju/juju/tree/3.3/worker/caasprober)

<a href="#heading--caasrbacmapper"><h2 id="heading--caasrbacmapper">`caasrbacmapper`</h2></a>
> See more: [`juju/worker/caasrbacmapper`](https://github.com/juju/juju/tree/3.3/worker/caasrbacmapper)

<a href="#heading--caasunitprovisioner"><h2 id="heading--caasunitprovisioner">`caasunitprovisioner`</h2></a>
> See more: [`juju/worker/caasunitprovisioner`](https://github.com/juju/juju/tree/3.3/worker/caasunitprovisioner)

<a href="#heading--caasunitsmanager"><h2 id="heading--caasunitsmanager">`caasunitsmanager`</h2></a>
> See more: [`juju/worker/caasunitsmanager`](https://github.com/juju/juju/tree/3.3/worker/caasunitsmanager)

<a href="#heading--caasunitterminationworker"><h2 id="heading--caasunitterminationworker">
`caasunitterminationworker`</h2></a>
> See more: [
`juju/worker/caasunitterminationworker`](https://github.com/juju/juju/tree/3.3/worker/caasunitterminationworker)

<a href="#heading--caasupgrader"><h2 id="heading--caasupgrader">`caasupgrader`</h2></a>
> See more: [`juju/worker/caasupgrader`](https://github.com/juju/juju/tree/3.3/worker/caasupgrader)

<a href="#heading--centralhub"><h2 id="heading--centralhub">`centralhub`</h2></a>
> See more: [`juju/worker/centralhub`](https://github.com/juju/juju/tree/3.3/worker/centralhub)

<a href="#heading--certupdater"><h2 id="heading--certupdater">`certupdater`</h2></a>
> See more: [`juju/worker/certupdater`](https://github.com/juju/juju/tree/3.3/worker/certupdater)

<a href="#heading--changestream"><h2 id="heading--changestream">`changestream`</h2></a>
> See more: [`juju/worker/changestream`](https://github.com/juju/juju/tree/3.3/worker/changestream)

<a href="#heading--charmdownloader"><h2 id="heading--charmdownloader">`charmdownloader`</h2></a>
> See more: [`juju/worker/charmdownloader`](https://github.com/juju/juju/tree/3.3/worker/charmdownloader)

<a href="#heading--charmrevision"><h2 id="heading--charmrevision">`charmrevision`</h2></a>

The charm revision updater worker is responsible for polling Charmhub every 24 hours to check if there are new revisions
available of any repository charm deployed in the model. If so, it will put a document in the Juju database, so that the
next time the user runs `juju status` they can see that there is an update available. This worker also sends anonymised
usage metrics to Charmhub when it polls.

This worker doesn't contain much business logic - most of the work is delegated to the facade call.

> See more: [`juju/worker/charmrevision`](https://github.com/juju/juju/tree/3.3/worker/charmrevision)

<a href="#heading--cleaner"><h2 id="heading--cleaner">`cleaner`</h2></a>

The `cleaner` worker handles database clean-up events.

> See more: [`juju/worker/cleaner`](https://github.com/juju/juju/tree/3.3/worker/cleaner)

<a href="#heading--common"><h2 id="heading--common">`common`</h2></a>
> See more: [`juju/worker/common`](https://github.com/juju/juju/tree/3.3/worker/common)

<a href="#heading--containerbroker"><h2 id="heading--containerbroker">`containerbroker`</h2></a>

The `containerbroker` worker's sole responsibility is to manage the lifecycle of an instance-broker.
Configuration of the instance-broker relies on talking to the provisioner to ensure that we correctly configure the
correct availability zones. Failure to do so, will result in an error.

The instance-broker is created for LXD types only -- any other container types cause the worker to uninstall itself.

> See more: [`juju/worker/containerbroker`](https://github.com/juju/juju/tree/3.3/worker/containerbroker)

<a href="#heading--controllerport"><h2 id="heading--controllerport">`controllerport`</h2></a>
> See more: [`juju/worker/controllerport`](https://github.com/juju/juju/tree/3.3/worker/controllerport)

<a href="#heading--credentialvalidator"><h2 id="heading--credentialvalidator">`credentialvalidator`</h2></a>
> See more: [`juju/worker/credentialvalidator`](https://github.com/juju/juju/tree/3.3/worker/credentialvalidator)

<a href="#heading--dbaccessor"><h2 id="heading--dbaccessor">`dbaccessor`</h2></a>
> See more: [`juju/worker/dbaccessor`](https://github.com/juju/juju/tree/3.3/worker/dbaccessor)

<a href="#heading--deployer"><h2 id="heading--deployer">`deployer`</h2></a>
> See more: [`juju/worker/deployer`](https://github.com/juju/juju/tree/3.3/worker/deployer)

<a href="#heading--diskmanager"><h2 id="heading--diskmanager">`diskmanager`</h2></a>

The `diskmanager` worker periodically lists block devices on the machine it runs on.

This worker will be run on all Juju-managed machines (one per machine agent).


> See more: [`juju/worker/diskmanager`](https://github.com/juju/juju/tree/3.3/worker/diskmanager)

<a href="#heading--environ"><h2 id="heading--environ">`environ`</h2></a>
> See more: [`juju/worker/environ`](https://github.com/juju/juju/tree/3.3/worker/environ)

<a href="#heading--environupgrader"><h2 id="heading--environupgrader">`environupgrader`</h2></a>
> See more: [`juju/worker/environupgrader`](https://github.com/juju/juju/tree/3.3/worker/environupgrader)

<a href="#heading--externalcontrollerupdater"><h2 id="heading--externalcontrollerupdater">
`externalcontrollerupdater`</h2></a>
> See more: [
`juju/worker/externalcontrollerupdater`](https://github.com/juju/juju/tree/3.3/worker/externalcontrollerupdater)

<a href="#heading--fanconfigurer"><h2 id="heading--fanconfigurer">`fanconfigurer`</h2></a>
> See more: [`juju/worker/fanconfigurer`](https://github.com/juju/juju/tree/3.3/worker/fanconfigurer)

<a href="#heading--filenotifywatcher"><h2 id="heading--filenotifywatcher">`filenotifywatcher`</h2></a>
> See more: [`juju/worker/filenotifywatcher`](https://github.com/juju/juju/tree/3.3/worker/filenotifywatcher)

<a href="#heading--firewaller"><h2 id="heading--firewaller">`firewaller`</h2></a>

The `firewaller` worker modifies provider networks when a user exposes/de-exposes applications, or when a unit
closes/opens ports.

> See more: [`juju/worker/firewaller`](https://github.com/juju/juju/tree/3.3/worker/firewaller)

<a href="#heading--fortress"><h2 id="heading--fortress">`fortress`</h2></a>

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

<a href="#heading--gate"><h2 id="heading--gate">`gate`</h2></a>

The `gate` worker provides a mechanism by which independent workers can wait for one another to finish a task, without
introducing explicit dependencies between those workers.

> See more: [`juju/worker/gate`](https://github.com/juju/juju/tree/3.3/worker/gate)

<a href="#heading--hostkeyreporter"><h2 id="heading--hostkeyreporter">`hostkeyreporter`</h2></a>
> See more: [`juju/worker/hostkeyreporter`](https://github.com/juju/juju/tree/3.3/worker/hostkeyreporter)

<a href="#heading--httpserver"><h2 id="heading--httpserver">`httpserver`</h2></a>
> See more: [`juju/worker/httpserver`](https://github.com/juju/juju/tree/3.3/worker/httpserver)

<a href="#heading--httpserverargs"><h2 id="heading--httpserverargs">`httpserverargs`</h2></a>
> See more: [`juju/worker/httpserverargs`](https://github.com/juju/juju/tree/3.3/worker/httpserverargs)

<a href="#heading--identityfilewriter"><h2 id="heading--identityfilewriter">`identityfilewriter`</h2></a>
> See more: [`juju/worker/identityfilewriter`](https://github.com/juju/juju/tree/3.3/worker/identityfilewriter)

<a href="#heading--instancemutater"><h2 id="heading--instancemutater">`instancemutater`</h2></a>

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

<a href="#heading--instancepoller"><h2 id="heading--instancepoller">`instancepoller`</h2></a>

The `instancepoller` worker updates network addresses and any related information for providers.

> See more: [`juju/worker/instancepoller`](https://github.com/juju/juju/tree/3.3/worker/instancepoller)

<a href="#heading--introspection"><h2 id="heading--introspection">`introspection`</h2></a>
> See more: [`juju/worker/introspection`](https://github.com/juju/juju/tree/3.3/worker/introspection)

<a href="#heading--leadership"><h2 id="heading--leadership">`leadership`</h2></a>
> See more: [`juju/worker/leadership`](https://github.com/juju/juju/tree/3.3/worker/leadership)

<a href="#heading--lease"><h2 id="heading--lease">`lease`</h2></a>

Package `lease`, also known as "the manager", manages the leases used by individual Juju workers.

Workers will claim a lease, and they are either attributed (i.e., the workers gets the lease ) or blocked (i.e., the
worker is waiting for a lease to become available). In the latter case, the manager will keep track of all the blocked
claims. When a worker's lease expires or gets revoked, then the manager will re-attribute it to one of other workers,
thus unblocking them and satisfying their claim.

In the special case where a worker is upgrading an application, it will ask the manager to "pin" the lease. This means
that the lease will not expire or be revoked during the upgrade, and the validity of the lease will get refreshed once
the upgrade has completed. The overall effect is that the application unit does not lose leadership during an upgrade.

> See more: [`juju/worker/lease`](https://github.com/juju/juju/tree/3.3/worker/lease)

<a href="#heading--leaseexpiry"><h2 id="heading--leaseexpiry">`leaseexpiry`</h2></a>
> See more: [`juju/worker/leaseexpiry`](https://github.com/juju/juju/tree/3.3/worker/leaseexpiry)

<a href="#heading--lifeflag"><h2 id="heading--lifeflag">`lifeflag`</h2></a>
> See more: [`juju/worker/lifeflag`](https://github.com/juju/juju/tree/3.3/worker/lifeflag)

<a href="#heading--logforwarder"><h2 id="heading--logforwarder">`logforwarder`</h2></a>
> See more: [`juju/worker/logforwarder`](https://github.com/juju/juju/tree/3.3/worker/logforwarder)

<a href="#heading--logger"><h2 id="heading--logger">`logger`</h2></a>
> See more: [`juju/worker/logger`](https://github.com/juju/juju/tree/3.3/worker/logger)

The `logger` worker watches the local logger configuration and reconfigures it when needed.

<a href="#heading--logsender"><h2 id="heading--logsender">`logsender`</h2></a>
> See more: [`juju/worker/logsender`](https://github.com/juju/juju/tree/3.3/worker/logsender)

<a href="#heading--machineactions"><h2 id="heading--machineactions">`machineactions`</h2></a>
> See more: [`juju/worker/machineactions`](https://github.com/juju/juju/tree/3.3/worker/machineactions)

<a href="#heading--machiner"><h2 id="heading--machiner">`machiner`</h2></a>

The `machiner` worker terminates the agent when the machine has a fatal error.

> See more: [`juju/worker/machiner`](https://github.com/juju/juju/tree/3.3/worker/machiner)

<a href="#heading--machineundertaker"><h2 id="heading--machineundertaker">`machineundertaker`</h2></a>
> See more: [`juju/worker/machineundertaker`](https://github.com/juju/juju/tree/3.3/worker/machineundertaker)

<a href="#heading--meterstatus"><h2 id="heading--meterstatus">`meterstatus`</h2></a>

The `meterstatus` worker executes the meter-status-changed hook periodically.

> See more: [`juju/worker/meterstatus`](https://github.com/juju/juju/tree/3.3/worker/meterstatus)

<a href="#heading--metrics"><h2 id="heading--metrics">`metrics`</h2></a>
> See more: [`juju/worker/metrics`](https://github.com/juju/juju/tree/3.3/worker/metrics)

<a href="#heading--metricworker"><h2 id="heading--metricworker">`metricworker`</h2></a>
> See more: [`juju/worker/metricworker`](https://github.com/juju/juju/tree/3.3/worker/metricworker)

<a href="#heading--migrationflag"><h2 id="heading--migrationflag">`migrationflag`</h2></a>
> See more: [`juju/worker/migrationflag`](https://github.com/juju/juju/tree/3.3/worker/migrationflag)

<a href="#heading--migrationmaster"><h2 id="heading--migrationmaster">`migrationmaster`</h2></a>
> See more: [`juju/worker/migrationmaster`](https://github.com/juju/juju/tree/3.3/worker/migrationmaster)

<a href="#heading--migrationminion"><h2 id="heading--migrationminion">`migrationminion`</h2></a>
> See more: [`juju/worker/migrationminion`](https://github.com/juju/juju/tree/3.3/worker/migrationminion)

<a href="#heading--minunitsworker"><h2 id="heading--minunitsworker">`minunitsworker`</h2></a>
> See more: [`juju/worker/minunitsworker`](https://github.com/juju/juju/tree/3.3/worker/minunitsworker)

<a href="#heading--mocks"><h2 id="heading--mocks">`mocks`</h2></a>

The `mocks` worker contains common worker mocks.

Run `go generate` to regenerate the mock interfaces.

> See more: [`juju/worker/mocks`](https://github.com/juju/juju/tree/3.3/worker/mocks)

<a href="#heading--modelcache"><h2 id="heading--modelcache">`modelcache`</h2></a>
> See more: [`juju/worker/modelcache`](https://github.com/juju/juju/tree/3.3/worker/modelcache)

<a href="#heading--modelworkermanager"><h2 id="heading--modelworkermanager">`modelworkermanager`</h2></a>
> See more: [`juju/worker/modelworkermanager`](https://github.com/juju/juju/tree/3.3/worker/modelworkermanager)

<a href="#heading--multiwatcher"><h2 id="heading--multiwatcher">`multiwatcher`</h2></a>

The `multiwatcher` worker provides watchers that watch the entire model.

It is responsible for creating, feeding, and cleaning up after multiwatchers.

The core worker gets an event stream from an AllWatcherBacking, and manages the multiwatcher Store.

The behaviour of the multiwatchers is very much tied to the Store implementation. The store provides a mechanism to get
changes over time.

> See more: [`juju/worker/multiwatcher`](https://github.com/juju/juju/tree/3.3/worker/multiwatcher)

<a href="#heading--muxhttpserver"><h2 id="heading--muxhttpserver">`muxhttpserver`</h2></a>
> See more: [`juju/worker/muxhttpserver`](https://github.com/juju/juju/tree/3.3/worker/muxhttpserver)

<a href="#heading--peergrouper"><h2 id="heading--peergrouper">`peergrouper`</h2></a>

The `peergrouper` worker maintains the MongoDB replica set.

> See more: [`juju/worker/peergrouper`](https://github.com/juju/juju/tree/3.3/worker/peergrouper)

<a href="#heading--presence"><h2 id="heading--presence">`presence`</h2></a>
> See more: [`juju/worker/presence`](https://github.com/juju/juju/tree/3.3/worker/presence)

<a href="#heading--provisioner"><h2 id="heading--provisioner">`provisioner`</h2></a>

The `provisioner` worker watches LXC and KVM instances, and provisions and decommissions them when needed.

<!-- Provision and decommission instances when needed-->

> See more: [`juju/worker/provisioner`](https://github.com/juju/juju/tree/3.3/worker/provisioner)

<a href="#heading--proxyupdater"><h2 id="heading--proxyupdater">`proxyupdater`</h2></a>
> See more: [`juju/worker/proxyupdater`](https://github.com/juju/juju/tree/3.3/worker/proxyupdater)

<a href="#heading--pruner"><h2 id="heading--pruner">`pruner`</h2></a>
> See more: [`juju/worker/pruner`](https://github.com/juju/juju/tree/3.3/worker/pruner)

<a href="#heading--pubsub"><h2 id="heading--pubsub">`pubsub`</h2></a>
> See more: [`juju/worker/pubsub`](https://github.com/juju/juju/tree/3.3/worker/pubsub)

<a href="#heading--querylogger"><h2 id="heading--querylogger">`querylogger`</h2></a>
> See more: [`juju/worker/querylogger`](https://github.com/juju/juju/tree/3.3/worker/querylogger)

<a href="#heading--reboot"><h2 id="heading--reboot">`reboot`</h2></a>
> See more: [`juju/worker/reboot`](https://github.com/juju/juju/tree/3.3/worker/reboot)

<a href="#heading--remoterelations"><h2 id="heading--remoterelations">`remoterelations`</h2></a>

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

<a href="#heading--retrystrategy"><h2 id="heading--retrystrategy">`retrystrategy`</h2></a>
> See more: [`juju/worker/retrystrategy`](https://github.com/juju/juju/tree/3.3/worker/retrystrategy)

<a href="#heading--s3caller"><h2 id="heading--s3caller">`s3caller`</h2></a>
> See more: [`juju/worker/s3caller`](https://github.com/juju/juju/tree/3.3/worker/s3caller)

<a href="#heading--secretbackendrotate"><h2 id="heading--secretbackendrotate">`secretbackendrotate`</h2></a>

The `secretbackendrotate` worker tracks and rotates a secret backend token.

> See more: [`juju/worker/secretbackendrotate`](https://github.com/juju/juju/tree/3.3/worker/secretbackendrotate)

<a href="#heading--secretdrainworker"><h2 id="heading--secretdrainworker">`secretdrainworker`</h2></a>

The `secretdrainworker` runs on the agent and drains secrets to the new active backend when the model changes secret
backends.

> See more: [`juju/worker/secretdrainworker`](https://github.com/juju/juju/tree/3.3/worker/secretdrainworker)

<a href="#heading--secretexpire"><h2 id="heading--secretexpire">`secretexpire`</h2></a>

The `secretexpire` worker tracks and notifies when a secret revision should expire.

> See more: [`juju/worker/secretexpire`](https://github.com/juju/juju/tree/3.3/worker/secretexpire)

<a href="#heading--secretrotate"><h2 id="heading--secretrotate">`secretrotate`</h2></a>

The `secretrotate` worker tracks a secret and notifies when it should be rotated.

> See more: [`juju/worker/secretrotate`](https://github.com/juju/juju/tree/3.3/worker/secretrotate)

<a href="#heading--simplesignalhandler"><h2 id="heading--simplesignalhandler">`simplesignalhandler`</h2></a>

The `simplesignalhandler` worker responds to OS signals and returns a pre-defined error from this worker when the signal
is received.

> See more: [`juju/worker/simplesignalhandler`](https://github.com/juju/juju/tree/3.3/worker/simplesignalhandler)

<a href="#heading--singular"><h2 id="heading--singular">`singular`</h2></a>
> See more: [`juju/worker/singular`](https://github.com/juju/juju/tree/3.3/worker/singular)

<a href="#heading--state"><h2 id="heading--state">`state`</h2></a>
> See more: [`juju/worker/state`](https://github.com/juju/juju/tree/3.3/worker/state)

<a href="#heading--stateconfigwatcher"><h2 id="heading--stateconfigwatcher">`stateconfigwatcher`</h2></a>
> See more: [`juju/worker/stateconfigwatcher`](https://github.com/juju/juju/tree/3.3/worker/stateconfigwatcher)

<a href="#heading--stateconverter"><h2 id="heading--stateconverter">`stateconverter`</h2></a>
> See more: [`juju/worker/stateconverter`](https://github.com/juju/juju/tree/3.3/worker/stateconverter)

<a href="#heading--statushistorypruner"><h2 id="heading--statushistorypruner">`statushistorypruner`</h2></a>
> See more: [`juju/worker/statushistorypruner`](https://github.com/juju/juju/tree/3.3/worker/statushistorypruner)

<a href="#heading--storageprovisioner"><h2 id="heading--storageprovisioner">`storageprovisioner`</h2></a>

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

<a href="#heading--syslogger"><h2 id="heading--syslogger">`syslogger`</h2></a>
> See more: [`juju/worker/syslogger`](https://github.com/juju/juju/tree/3.3/worker/syslogger)

<a href="#heading--terminationworker"><h2 id="heading--terminationworker">`terminationworker`</h2></a>

The `terminationworker` stops the agent when it has been signalled to do so.

> See more: [`juju/worker/terminationworker`](https://github.com/juju/juju/tree/3.3/worker/terminationworker)

<a href="#heading--toolsversionchecker"><h2 id="heading--toolsversionchecker">`toolsversionchecker`</h2></a>
> See more: [`juju/worker/toolsversionchecker`](https://github.com/juju/juju/tree/3.3/worker/toolsversionchecker)

<a href="#heading--undertaker"><h2 id="heading--undertaker">`undertaker`</h2></a>
> See more: [`juju/worker/undertaker`](https://github.com/juju/juju/tree/3.3/worker/undertaker)

<a href="#heading--unitassigner"><h2 id="heading--unitassigner">`unitassigner`</h2></a>
> See more: [`juju/worker/unitassigner`](https://github.com/juju/juju/tree/3.3/worker/unitassigner)

<a href="#heading--uniter"><h2 id="heading--uniter">`uniter`</h2></a>

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

<a href="#heading--upgradedatabase"><h2 id="heading--upgradedatabase">`upgradedatabase`</h2></a>
> See more: [`juju/worker/upgradedatabase`](https://github.com/juju/juju/tree/3.3/worker/upgradedatabase)

<a href="#heading--upgrader"><h2 id="heading--upgrader">`upgrader`</h2></a>

The `upgrader` worker schedules upgrades of the agent's binary, i.e. it upgrades the agent itself.

<!-- : A worker that runs an upgraded code binary once the old binary has been replaced-->

> See more: [`juju/worker/upgrader`](https://github.com/juju/juju/tree/3.3/worker/upgrader)

<a href="#heading--upgradeseries"><h2 id="heading--upgradeseries">`upgradeseries`</h2></a>
> See more: [`juju/worker/upgradeseries`](https://github.com/juju/juju/tree/3.3/worker/upgradeseries)

<a href="#heading--upgradesteps"><h2 id="heading--upgradesteps">`upgradesteps`</h2></a>
> See more: [`juju/worker/upgradesteps`](https://github.com/juju/juju/tree/3.3/worker/upgradesteps)


<!--
* *Machine Environment Worker* : Watches proxy configuration and reconfigures the machine, >> WHICH ONE IS IT?
* *Resumer* : Resumes incomplete MongoDB transactions. >> COULDN'T FIND IT IN THE CODE.


[note]
Many Controller Agents are run in the same machine where the MongoDB replica-set master runs. When the master changes, these workers all stop.
[/note]
-->