// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package unit defines the unit agent for Kubernetes deployments: the agent
// that runs the workers for a unit.
//
// The agent is created by the containeragent command (see
// github.com/juju/juju/cmd/containeragent) and runs a dependency engine
// (dependency.NewEngine) over the manifolds constructed by the Manifolds
// function in manifolds.go.
//
// See github.com/juju/juju/internal/worker/deployer for the equivalent unit
// agent in machine deployments.
package unit
