//go:build !dqlite

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dqlite

const (
	// Enabled is false if dqlite is disabled.
	Enabled = false
)

type NodeRole int

const (
	// Voter is a full voting member of the cluster.
	Voter NodeRole = iota
	// StandBy is a non-voting member that can be promoted.
	StandBy
	// Spare is a non-voting spare member.
	Spare
)

func (NodeRole) String() string {
	return ""
}

type NodeInfo struct {
	ID      uint64   `yaml:"ID"`
	Address string   `yaml:"Address"`
	Role    NodeRole `yaml:"Role"`
}

func ReconfigureMembership(string, []NodeInfo) error {
	return nil
}
