// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package deployer provides the deployer worker, which deploys and recalls
// unit agents on a machine, and the unit agent for machine deployments.
//
// The unit agent (unit_agent.go) runs the workers for a unit in a machine
// deployment: it runs a dependency engine (dependency.NewEngine) over the
// manifolds constructed in unit_manifolds.go.
//
// See github.com/juju/juju/cmd/containeragent/unit for the equivalent unit
// agent in Kubernetes deployments.
package deployer
