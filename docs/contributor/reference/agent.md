---
myst:
  html_meta:
    description: "Learn about Juju agents, their types, dependency engines, and how they manage workers across different deployment environments."
---

(agent-cont)=
# Agent
> See first: {ref}`User docs | Agent <agent>`

In Juju, an **agent** is any process that runs a dependency engine ([`dependency.NewEngine`](#newengine)) to start and manage {ref}`workers <worker-cont>` for a particular domain
entity in a particular deployment environment.

## List of agents

While Juju agents can be counted based on the number of files in the codebase that invoke `dependency.NewEngine`, the
type of process that they represent depends on the specific [`jujuagentd`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/doc.go) / [
`containeragent`](https://github.com/juju/juju/blob/3.6/cmd/containeragent))
agent-creating command that they invoke, and the workers that they bring up depend on the specific manifold
declaration (files conventionally called `*manifolds.go` that invoke [`dependency.Manifolds`](#manifolds))
that is used to configure the dependency engine. Thus, starting from the list of files that invoke a dependency engine,
and factoring in these variations, one can distinguish the following agents:

```{note}
You can think of the list of agents based on invocations of `dependency.NewEngine`  files as the 'actual agents' and of
the list of agents arising from the various splits defined in those files as the 'logical agents'.
```

- [`cmd/jujuagentd/agent/machine.go`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/machine.go) <br>
  Uses [`jujuagentd/main.go` >
  `NewMachineAgentCmd`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/main.go)
  with [cmd/jujuagentd/agent/machine/manifolds.go](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/machine/manifolds.go)
  and [cmd/jujuagentd/agent/model/manifolds.go](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/model/manifolds.go)
  to define an agent with multiple modes that will run different sets of workers depending on whether the target is
    - a machine (in a machine deployment; with subdistinctions based on whether it is a regular machine or rather a
      container running on top of a machine; whether the machine carries a controller, model, unit) or
    - a container (specifically, the controller container in a Kubernetes deployment, with subdistinctions for whether
      it has a model on it as well or not).

- [`internal/worker/deployer/unit_agent.go`](https://github.com/juju/juju/blob/HEAD/internal/worker/deployer/unit_agent.go) <br>
  Uses [`jujuagentd/main.go` >
  `NewMachineAgentCmd`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/main.go) (
  indirectly, through the `deployer` worker from
  the [`cmd/jujuagentd/agent/machine.go`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/machine.go)
  agent)
  and [`internal/worker/deployer/unit_manifolds.go`](https://github.com/juju/juju/blob/HEAD/internal/worker/deployer/unit_manifolds.go)
  to define an agent that will run workers for units in a machine deployment.

- [`cmd/jujuagentd/agent/model.go`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/model.go) <br>
  Uses [`jujuagentd/main.go` >
  `NewModelCommand`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/main.go)
  and [`cmd/jujuagentd/agent/modeloperator/manifolds.go`](https://github.com/juju/juju/blob/HEAD/cmd/jujuagentd/agent/modeloperator/manifolds.go)
  to define an agent that will run the modeloperator pod in a Kubernetes deployment.

- [`cmd/containeragent/unit/agent.go`](https://github.com/juju/juju/blob/3.6/cmd/containeragent/unit/agent.go) <br>
  Uses [`containeragent/main_nix.go` >
  `containerAgentCommand`](https://github.com/juju/juju/blob/3.6/cmd/containeragent/main_nix.go)
  and [cmd/containeragent/unit/manifolds.go](https://github.com/juju/juju/blob/3.6/cmd/containeragent/unit/manifolds.go)
  to define an agent that will run workers for units in a Kubernetes deployment.

- [`cmd/jujuagentd/agent/caasoperator.go`](https://github.com/juju/juju/blob/3.6/cmd/jujuagentd/agent/caasoperator.go)  (
  deprecated)  <br>
  Uses [`jujuagentd/main.go` >
  `NewCaasOperatorAgent`](https://github.com/juju/juju/blob/3.6/cmd/jujuagentd/main.go)
  and [`cmd/jujuagentd/agent/caasoperator/manifolds.go`](https://github.com/juju/juju/blob/3.6/cmd/jujuagentd/agent/caasoperator/manifolds.go)
  to define an agent that will run workers for podspec charms (deprecated).