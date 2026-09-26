// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package deployer provides the deployer worker, which deploys and
// removes units on a machine, and the unit agent for machine
// deployments (unit_agent.go): the agent that runs the workers for a
// unit in a machine deployment. The unit agent runs a dependency
// engine (dependency.NewEngine) over the manifolds constructed in
// unit_manifolds.go.
package deployer
