// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent contains the agent-creating commands for the jujud
// binary. An agent is any process that runs a dependency engine
// (dependency.NewEngine) to start and manage the workers for a
// particular domain entity in a particular deployment environment.
//
// The workers an agent runs are declared by a set of manifolds — files
// conventionally called *manifolds.go that construct a
// dependency.Manifolds value — which are installed into the engine
// with dependency.Install.
//
// The agents defined here are:
//
//   - NewMachineAgentCommand defines the machine agent: an agent with
//     multiple modes that runs different sets of workers depending on
//     whether the target is a machine (in a machine deployment; with
//     subdistinctions based on whether it is a regular machine or a
//     container running on top of a machine, and whether the machine
//     carries a controller, a model or a unit) or a container (the
//     controller container in a Kubernetes deployment, with
//     subdistinctions for whether it hosts a model as well).
//   - NewModelCommand defines the modeloperator agent: the agent that
//     runs the modeloperator pod in a Kubernetes deployment.
//
// The unit agent for machine deployments is defined in
// internal/worker/deployer, and the unit agent for Kubernetes
// deployments in cmd/containeragent/unit.
package agent
