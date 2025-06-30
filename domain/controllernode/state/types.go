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

// controllerNodeAgentVersion is the database representation of a controller
// node agent version.
type controllerNodeAgentVersion struct {
	ControllerID   string `db:"controller_id"`
	Version        string `db:"version"`
	ArchitectureID int    `db:"architecture_id"`
}

// controllerAPIAddress is the database representation of a controller api
// address with the controller id and whether it is for agents or clients.
type controllerAPIAddress struct {
	// ControllerID is the controller node id.
	ControllerID string `db:"controller_id"`
	// Address is the address of the controller node.
	Address string `db:"address"`
	// IsAgent is whether the address is for agents as well as for clients.
	IsAgent bool `db:"is_agent"`
	// Scope is the address scope.
	Scope string `db:"scope"`
}

// countResult is the database representation of a count result.
type countResult struct {
	Count int `db:"count"`
}

// controllerID is the database representation of a controller node id.
type controllerID struct {
	ID string `db:"controller_id"`
}

// controllerAPIAddressStr is the database representation of a controller api
// address alone.
type controllerAPIAddressStr struct {
	// Address is the address of the controller node.
	Address string `db:"address"`
}

type controllerIDs []string
