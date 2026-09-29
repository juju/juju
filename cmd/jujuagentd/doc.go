// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The jujuagentd binary provides the commands that spawn the Juju agents on
// machines and in the controller container, and forwards hook-tool
// invocations to the unit agent.
//
// An agent is a process that runs a dependency engine to start and manage the
// workers for a particular domain entity in a particular deployment
// environment. The commands registered by this binary (see run.go) include:
//
//   - machine, which spawns the machine agent (NewMachineAgentCommand in
//     the agent package);
//   - model, which spawns the modeloperator agent that runs in a Kubernetes
//     deployment (NewModelCommand in the agent package);
//   - safe-mode and db-repl, which spawn variants of the machine agent with
//     their own sets of workers;
//   - bootstrap-state and check-connection.
//
// When invoked through a symlink named for a hook tool, the binary instead
// forwards the invocation over RPC to the unit agent.
//
// See github.com/juju/juju/cmd/jujuagentd/agent for the agent-creating
// commands and github.com/juju/juju/cmd/containeragent for the equivalent
// binary in a unit pod.
package main
