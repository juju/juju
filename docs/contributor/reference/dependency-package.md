(dependency-package)=
# Dependency Package
The **`dependency`** package is a subpackage in the Go [`worker` library](worker-package.md)  that provides constructs
that help
manage shared resources and their lifetimes. In Juju, it provides constructs that enable [agents](agent.md) to run
acyclic graphs of [workers](worker.md) (i.e., workers and their dependencies, i.e., other workers).

> See more: [Go packages | `worker` > `dependency`](https://pkg.go.dev/github.com/juju/worker/v3@v3.3.0/dependency)

## List of most important `dependency` package constructs

This section lists all the most relevant `dependency` package constructs, following a top-down order (going from agent
to worker).

(newengine)=
## `NewEngine`

`dependency.NewEngine` is a function that is used in the definition of an agent to create and initialise an engine that
will maintain the dependency graph defined by any installed instances of the {ref}`manifolds` type.

> See more: [
`NewEngine`](https://github.com/juju/worker/blob/e43ac123ef3cbdf02d00e8c5f673c473b2188cff/dependency/engine.go#L132), [example usage of
`NewEngine` in the model agent](https://github.com/juju/juju/blob/3.3/cmd/jujud/agent/model.go#L188C2-L188C2)

## `Install`

`dependency.Install` is a function that is used in the definition of an agent to install instances of the {ref}`manifolds` type.

> See more: [
`Install`](https://github.com/juju/worker/blob/e43ac123ef3cbdf02d00e8c5f673c473b2188cff/dependency/util.go#L17),
> [example usage of
`Install` in the model agent](https://github.com/juju/juju/blob/1206b7da23628ec1b31cf5a22ec56c8a1c6c1ab9/cmd/jujud/agent/model.go#L192C4-L192C4)

(manifolds)=
## `Manifolds`

`dependency.Manifolds` is a type that is used in the definition of an agent to define a collection of things of the  {ref}`manifold` type.

> See more: [
`Manifolds`](https://github.com/juju/worker/blob/e43ac123ef3cbdf02d00e8c5f673c473b2188cff/dependency/interface.go#L82),
> [example usage of
`Manifolds` in the model agent](https://github.com/juju/juju/blob/1206b7da23628ec1b31cf5a22ec56c8a1c6c1ab9/cmd/jujud/agent/model/manifolds.go#L137)

(manifold)=
## `Manifold`

`dependency.Manifold` is a type that is used in the definition of a worker to declare its inputs, outputs, and start
function.

> See more: [
`Manifold`](https://github.com/juju/worker/blob/e43ac123ef3cbdf02d00e8c5f673c473b2188cff/dependency/interface.go#L15),
> [example usage of
`Manifold` in the firewaller worker](https://github.com/juju/juju/blob/3113a35d31eea873707b3f1a21f9a2f15be43eca/worker/firewaller/manifold.go#L54)

<!--
cmd/jujud/main.go has the jujud.Register calls for its sub-commands. Within the ones that start agents (machine, model) you'll see calls to dependency.NewEngine to create an engine with its config, then later dependency.Install, which accepts a Manifolds value and starts the graph of workers that it represents
-->