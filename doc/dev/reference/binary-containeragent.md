> See first: [Juju | `containeragent`](https://juju.is/docs/juju/containeragent-binary)

The `containeragent` binary provides the [
`containerAgentCommand`](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/containeragent/main_nix.go#L89),
which is used
in [cmd/containeragent/unit/agent.go](https://github.com/juju/juju/blob/7a9eb97bee51d965f8e07f684b1f8929ab18d1f4/cmd/containeragent/unit/agent.go#L315)
to spawn the unit agent for units in a Kubernetes deployment.