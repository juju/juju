---
myst:
  html_meta:
    description: "Understand Juju workers, their interface implementation, and role in the dependency engine for managing concurrent operations."
---

(worker-cont)=
# Worker

In Juju, a **worker** is any type that implements the `worker.Worker` interface.

Examples of workers include {ref}`the dependency engine <newengine>`, instances run by the dependency
engine (the typical usage of the term "worker"),
and [watchers](https://github.com/juju/juju/blob/HEAD/core/watcher/watcher.go).

A Juju {ref}`agent <agent-cont>` runs one or more workers at the same time in parallel. A worker may run / be run by
another worker.
