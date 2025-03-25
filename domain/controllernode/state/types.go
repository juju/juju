// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbControllerNode is the database representation of a controller node.
type dbControllerNode struct {
	// ControllerID is the nodes controller ID.
	ControllerID string `db:"controller_id"`

	// DQLiteNodeID is the uint64 from Dqlite NodeInfo, stored as text (due to
	// db issues when the high bit is set).
	DQLiteNodeID string `db:"dqlite_node_id"`

	// BindAddress is the IP address (no port) that Dqlite is bound to.
	BindAddress string `db:"bind_address"`
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
