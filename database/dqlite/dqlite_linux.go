//go:build dqlite && linux

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqlite

import (
	"github.com/canonical/go-dqlite/v3"
	"github.com/canonical/go-dqlite/v3/client"
)

const (
	// Enabled is true if dqlite is enabled.
	Enabled = true
)

// NodeInfo holds information about a single server.
type NodeInfo = dqlite.NodeInfo

// NodeRole identifies the role of a node within the Dqlite cluster.
type NodeRole = client.NodeRole

const (
	// Voter is a full voting member of the cluster.
	Voter = client.Voter
	// StandBy is a non-voting member that can be promoted.
	StandBy = client.StandBy
	// Spare is a non-voting spare member.
	Spare = client.Spare
)

// ReconfigureMembership can be used to recover a cluster whose majority of
// nodes have died, and therefore has become unavailable.
//
// It forces appending a new configuration to the raft log stored in the given
// directory, effectively replacing the current configuration.
func ReconfigureMembership(dir string, cluster []NodeInfo) error {
	return dqlite.ReconfigureMembership(dir, cluster)
}
