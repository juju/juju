// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbControllerNode is the database representation of a controller node.
type dbControllerNode struct {
	// ControllerID is the nodes controller ID.
	ControllerID string `db:"controller_id"`

	// DqliteNodeID is the uint64 from Dqlite NodeInfo, stored as text (due to
	// db issues when the high bit is set).
	DqliteNodeID string `db:"dqlite_node_id"`

	// DqliteBindAddress is the IP address (no port) that Dqlite is bound to.
	DqliteBindAddress string `db:"dqlite_bind_address"`
}

type dbControllerNodeCount struct {
	Count int `db:"count"`
}

type dbNamespace struct {
	Namespace string `db:"namespace"`
}

// architecture is the database representation of an architecture id-name pair.
type architecture struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

// controllerNodeAgentVersion is the database representation of a controller node agent version.
type controllerNodeAgentVersion struct {
	ControllerID   string `db:"controller_id"`
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}
