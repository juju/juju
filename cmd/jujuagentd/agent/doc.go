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
//     manifolds from the machine package, selecting K8sManifolds when the
//     agent is a CAAS agent (the controller container in a Kubernetes
//     deployment) and IAASManifolds otherwise. On controllers it also
//     starts, for every model, an engine running the manifolds from the model
//     package (IAASManifolds or CAASManifolds, according to the model type).
//   - The modeloperator agent (NewModelCommand, in model.go), which runs the
//     manifolds from the modeloperator package: the workers of the
//     modeloperator pod in a Kubernetes deployment.
//   - The safe-mode and db-repl variants of the machine agent
//     (NewSafeModeAgentCommand and NewDBReplAgentCommand), which run the
//     manifolds from the safemode and dbrepl packages.
//
// See github.com/juju/juju/cmd/jujuagentd for the binary that registers
// these commands. The unit agent for machine deployments is defined in
// github.com/juju/juju/internal/worker/deployer, and the unit agent for
// Kubernetes deployments in github.com/juju/juju/cmd/containeragent/unit.
package agent
