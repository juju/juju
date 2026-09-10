// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbControllerNode is the database representation of a controller node.
type dbControllerNode struct {
	// ControllerID is the nodes controller ID.
	ControllerID string `db:"controller_id"`

	// LifeID is the lifecycle state of the controller node.
	LifeID int `db:"life_id"`

	// DqliteNodeID is the uint64 from Dqlite NodeInfo, stored as text (due to
	// db issues when the high bit is set).
	DqliteNodeID string `db:"dqlite_node_id"`
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

// controllerNodeAPIAddress is the database representation of an address for a
// specific controller node.
type controllerNodeAPIAddress struct {
	// ControllerID is the controller node id.
	ControllerID string `db:"controller_id"`
	// Address is the address of the controller node.
	Address string `db:"address"`
	// Scope is the address scope.
	Scope string `db:"scope"`
}

// controllerAPIAddress is retained for address-delta unit tests.
type controllerAPIAddress struct {
	ControllerID string
	Address      string
	IsAgent      bool
}

type agentAPIAddress struct {
	Address     string `db:"address"`
	IsAgentOnly bool   `db:"is_agent_only"`
	Scope       string `db:"scope"`
}

type clientAPIAddress struct {
	Address string `db:"address"`
	Scope   string `db:"scope"`
}

// countResult is the database representation of a count result.
type countResult struct {
	Count int `db:"count"`
}

// controllerID is the database representation of a controller node id.
type controllerID struct {
	ID string `db:"controller_id"`
}

type controllerIDs []string
