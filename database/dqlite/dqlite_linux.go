//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqlite

import "github.com/canonical/go-dqlite"

const (
	// Enabled is true if dqlite is enabled.
	Enabled = true
)

// NodeInfo holds information about a single server.
type NodeInfo = dqlite.NodeInfo

// ReconfigureMembership can be used to recover a cluster whose majority of
// nodes have died, and therefore has become unavailable.
//
// It forces appending a new configuration to the raft log stored in the given
// directory, effectively replacing the current configuration.
func ReconfigureMembership(dir string, cluster []NodeInfo) error {
	return dqlite.ReconfigureMembership(dir, cluster)
}
