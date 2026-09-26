// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package machine defines the manifolds for the jujud machine agent.
// The IAASManifolds and CAASManifolds functions each construct a
// dependency.Manifolds value: the set of workers the machine agent
// runs when its target is a machine in a machine deployment or the
// controller container in a Kubernetes deployment, respectively. Both
// are variations over the common set built by commonManifolds.
//
// The manifolds are installed into the agent's dependency engine by
// the agent definition in cmd/jujud/agent (machine.go), which selects
// between the two depending on the agent's configuration.
package machine
