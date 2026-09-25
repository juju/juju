// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package unit defines the unit agent for Kubernetes deployments: the
// agent that runs the workers for a unit in a CAAS deployment. The
// agent is created by the containeragent command (cmd/containeragent)
// and runs a dependency engine (dependency.NewEngine) over the
// manifolds constructed by the Manifolds function in manifolds.go.
package unit
