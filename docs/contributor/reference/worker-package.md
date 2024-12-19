(worker-package)=
# Worker package
The **`worker`  package** is a Go package that provides utilities for handling long-lived Go workers.

In Juju, it provides [the worker interface](worker-interface.md) that is used to define
all [workers](worker-interface.md), and
the [
`dependency`](dependency-package.md) and the [`catacomb`](catacomb-package.md) package.

> See more: [Go packages | `worker`](https://pkg.go.dev/github.com/juju/worker), [Go packages | `worker` >
`worker.go` (worker interface)](https://pkg.go.dev/github.com/juju/worker#Worker)