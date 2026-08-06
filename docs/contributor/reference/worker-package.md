---
myst:
  html_meta:
    description: "Learn about the Juju worker package providing utilities, interfaces, and dependency management for long-lived worker goroutines in Juju."
---

(worker-package)=
# Worker package

The **`worker`  package** is a Go package that provides utilities for handling long-lived Go workers.

In Juju, it provides {ref}`the worker interface <worker-interface>` that is used to define
all {ref}`workers <worker-cont>`, and
{ref}`the dependency package <dependency-package>` and {ref}`the catacomb package <catacomb-package>` package.

> See more: [Go packages | `worker`](https://pkg.go.dev/github.com/juju/worker), [Go packages | `worker` >
`worker.go` (worker interface)](https://pkg.go.dev/github.com/juju/worker#Worker)