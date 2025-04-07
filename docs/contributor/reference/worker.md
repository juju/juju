(worker-cont)=
# Worker
<!---To see previous major version of this doc, see version 13.-->

> See first: {ref}`User docs | Worker <worker>`

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

> The most important workers to know about are: the `uniter`, the `deployer`, the `provisioner`, the `caasapplicationprovisioner`, the `charmdownloader`, and the `undertaker`.
