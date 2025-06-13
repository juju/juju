(dependency-package)=
# Dependency package

The **`dependency`** package is a subpackage in {ref}`the Go worker package <worker-package>` that provides constructs
that help
manage shared resources and their lifetimes. In Juju, it provides constructs that enable {ref}`agents <agent>` to run
acyclic graphs of {ref}`workers <worker>` (i.e., workers and their dependencies, i.e., other workers).

> See more: [Go packages | `worker` > `dependency`](https://pkg.go.dev/github.com/juju/worker/v3@v3.3.0/dependency)

## List of most important `dependency` package constructs

This section lists all the most relevant `dependency` package constructs, following a top-down order (going from agent
to worker).

(newengine)=
## `NewEngine`

`dependency.NewEngine` is a function that is used in the definition of an agent to create and initialise an engine that
will maintain the dependency graph defined by any installed instances of the {ref}`manifolds` type.

> See more: [
`NewEngine`](https://github.com/juju/worker/blob/HEAD/dependency/engine.go), [example usage of
`NewEngine` in the model agent](https://github.com/juju/juju/blob/HEAD/cmd/jujud/agent/model.go)

## `Install`

`dependency.Install` is a function that is used in the definition of an agent to install instances of the {ref}`manifolds` type.

> See more: [
`Install`](https://github.com/juju/worker/blob/HEAD/dependency/util.go),
> [example usage of
`Install` in the model agent](https://github.com/juju/juju/blob/HEAD/cmd/jujud/agent/model.go)

(manifolds)=
## `Manifolds`

`dependency.Manifolds` is a type that is used in the definition of an agent to define a collection of things of the  {ref}`manifold` type.

> See more: [
`Manifolds`](https://github.com/juju/worker/blob/HEAD/dependency/interface.go),
> [example usage of
`Manifolds` in the model agent](https://github.com/juju/juju/blob/HEAD/cmd/jujud/agent/model.go)

(manifold)=
## `Manifold`

`dependency.Manifold` is a type that is used in the definition of a worker to declare its inputs, outputs, and start
function.

> See more: [
`Manifold`](https://github.com/juju/worker/blob/HEAD/dependency/interface.go),
> [example usage of
`Manifold` in the firewaller worker](https://github.com/juju/juju/blob/HEAD/internal/worker/firewaller/manifold.go)

<!--
cmd/jujud/main.go has the jujud.Register calls for its sub-commands. Within the ones that start agents (machine, model) you'll see calls to dependency.NewEngine to create an engine with its config, then later dependency.Install, which accepts a Manifolds value and starts the graph of workers that it represents
-->