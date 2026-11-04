// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent defines the agent-creating commands of the jujuagentd
// binary.
//
// An agent is a process that runs a dependency engine
// (dependency.NewEngine) to start and manage the workers for a particular
// domain entity in a particular deployment environment. The workers an
// agent runs are declared by manifolds: files conventionally called
// manifolds.go that construct a dependency.Manifolds value, which the agent
// installs into its engine with dependency.Install.
//
// The agents defined here are:
//
//   - The machine agent (NewMachineAgentCommand, in machine.go). It runs the
//     manifolds from the machine package: K8sManifolds when the agent runs
//     as the controller container in a Kubernetes deployment, and
//     IAASManifolds otherwise.
//   - The modeloperator agent (NewModelCommand, in model.go), which runs the
//     manifolds from the modeloperator package: the workers of the
//     modeloperator pod in a Kubernetes deployment.
//
// The controller commands - bootstrap, init, controller configuration,
// the controller application, safe-mode, and the DB REPL - belong to the
// jujud binary, not to jujuagentd.
//
// See github.com/juju/juju/cmd/jujuagentd for the binary that registers
// these commands. The unit agent for machine deployments is defined in
// github.com/juju/juju/internal/worker/deployer, and the unit agent for
// Kubernetes deployments in the unit package of cmd/containeragent.
package agent
