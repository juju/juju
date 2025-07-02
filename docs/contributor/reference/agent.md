(agent-dev)=
# Agent
> See first: {ref}`User docs | Agent <agent>`

In Juju, an **agent** is any process that runs a dependency engine ([
`dependency.NewEngine`](#newengine)) to start and manage {ref}`workers <worker-cont>` for a particular domain
entity in a particular deployment environment.

## List of agents



While Juju agents can be counted based on the number of files in the codebase that invoke `dependency.NewEngine`, the
type of process that they represent depends on the specific [`jujud`](https://github.com/juju/juju/blob/main/cmd/jujud/doc.go) / [
`containeragent`](binary-containeragent.md)
agent-creating command that they invoke, and the workers that they bring up depend on the specific manifold
declaration (files conventionally called `*manifolds.go` that invoke [
`dependency.Manifolds`](dependency-package.md#manifolds))
that is used to configure the dependency engine. Thus, starting from the list of files that invoke a dependency engine,
and factoring in these variations, one can distinguish the following agents:

[note type=information]
You can think of the list of agents based on invocations of `dependency.NewEngine`  files as the 'actual agents' and of
the list of agents arising from the various splits defined in those files as the 'logical agents'.
[/note]

<!-- TODO: There is a lot of relative link to possible outdated version of code. Maybe we should review it to make it more
relative to the code (and maybe move it into some doc.go or go documentation anyway -->

- [cmd/jujud/agent/machine.go](https://github.com/juju/juju/blob/main/cmd/jujud/agent/machine.go) <br>
  Uses [`jujud/main.go` >
  `NewMachineAgentCmd`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/main.go#L275)
  with [cmd/jujud/agent/machine/manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/machine/manifolds.go#L980)
  and [cmd/jujud/agent/model/manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/model/manifolds.go#L4)
  to define an agent with multiple modes that will run different sets of workers depending on whether the target is
    - a machine (in a machine deployment; with subdistinctions based on whether it is a regular machine or rather a
      container running on top of a machine; whether the machine carries a controller, model, unit) or
    - a container (specifically, the controller container in a Kubernetes deployment, with subdistinctions for whether
      it has a model on it as well or not).

- [worker/deployer/unit_agent.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/worker/deployer/unit_agent.go#L204) <br>
  Uses [`jujud/main.go` >
  `NewMachineAgentCmd`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/main.go#L275) (
  indirectly, through the `deployer` worker from
  the [cmd/jujud/agent/machine.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/machine.go)
  agent)
  and [worker/deployer/unit_manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/worker/deployer/unit_manifolds.go#L4)
  to define an agent that will run workers for units in a machine deployment.


- [cmd/jujud/agent/model.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/model.go#L188) <br>
  Uses [`jujud/main.go` >
  `NewModelCommand`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/main.go#L262)
  and [cmd/jujud/agent/modeloperator/manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/modeloperator/manifolds.go#L4)
  to define an agent that will run the modeloperator pod in a Kubernetes deployment.

- [cmd/containeragent/unit/agent.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/containeragent/unit/agent.go#L315) <br>
  Uses [`containeragent/main_nix.go` >
  `containerAgentCommand`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/containeragent/main_nix.go#L89)
  and [cmd/containeragent/unit/manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/containeragent/unit/manifolds.go#L4)
  to define an agent that will run workers for units in a Kubernetes deployment.

- [cmd/jujud/agent/caasoperator.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/caasoperator.go#L250)  (
  deprecated)  <br>
  Uses [`jujud/main.go` >
  `NewCaasOperatorAgent`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/main.go#L277)
  and [cmd/jujud/agent/caasoperator/manifolds.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/jujud/agent/caasoperator/manifolds.go#L4)
  to define an agent that will run workers for podspec charms (deprecated).