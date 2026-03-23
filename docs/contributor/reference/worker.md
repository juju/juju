---
myst:
  html_meta:
    description: "Understand Juju workers, their interface implementation, and role in the dependency engine for managing concurrent operations."
---

(worker-cont)=
# Worker

> See first: {ref}`User docs | Worker <worker>`

In Juju, a **worker** is any type that implements {ref}`the worker interface <worker-interface>`.

Examples of workers include {ref}`the dependency engine <newengine>`, instances run by the dependency
engine (the typical usage of the term "worker"),
and [watchers](https://github.com/juju/juju/blob/HEAD/core/watcher/watcher.go).

Note: A Juju {ref}`agent <agent-cont>` runs one or more workers at the same time in parallel. A worker may run / be run by
another worker.

## List of workers run by the dependency engine

In Juju, the term "worker" is most commonly used to denote types whose instances are run by the dependency engine.

> The most important workers to know about are: the `uniter`, the `deployer`, the `provisioner`, the `caasapplicationprovisioner`, the `charmdownloader`, and the `undertaker`.
