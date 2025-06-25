(catacomb-package)=
# Catacomb package
The **`catacomb`** package is a subpackage in the Go `worker` library that leverages {ref}`the tomb package <tomb-package>` to bind the lifetimes of, and track the errors of, a group of related {ref}`workers <worker>`. In Juju it is used in precisely this way.


> See more: [Go packages | `worker` > `catacomb`](https://pkg.go.dev/github.com/juju/worker/v3@v3.3.0/catacomb)