The **`catacomb`** package is a subpackage in the Go `worker` library that leverages [`tomb`](tomb-package.md) to bind
the
lifetimes of, and track the errors of, a group of related [workers](worker.md). In Juju it is used in precisely this
way.


> See more: [Go packages | `worker` > `catacomb`](https://pkg.go.dev/github.com/juju/worker/v3@v3.3.0/catacomb)