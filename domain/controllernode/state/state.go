// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strconv"

	"github.com/canonical/sqlair"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/controllernode"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	"github.com/juju/juju/internal/errors"
)

// State represents database interactions dealing with controller nodes.
type State struct {
	*domain.StateBase
}

// NewState returns a new controller node state
// based on the input database factory method.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CurateNodes accepts slices of controller IDs to insert
// and delete from the controller node table.
func (st *State) CurateNodes(ctx context.Context, insert, delete []string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Single dbControllerNode object created here and reused.
	controllerNode := dbControllerNode{}

	// These are never going to be many at a time. Just repeat as required.
	insertStmt, err := st.Prepare(`
INSERT INTO controller_node (controller_id)
VALUES      ($dbControllerNode.*)`, controllerNode)
	if err != nil {
		return errors.Errorf("preparing insert controller node statement: %w", err)
	}
	deleteStmt, err := st.Prepare(`
DELETE FROM controller_node 
WHERE       controller_id = $dbControllerNode.controller_id`, controllerNode)
	if err != nil {
		return errors.Errorf("preparing delete controller node statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, cID := range insert {
			controllerNode.ControllerID = cID
			if err := tx.Query(ctx, insertStmt, controllerNode).Run(); err != nil {
				return errors.Errorf("inserting controller node %q: %w", cID, err)
			}
		}

		for _, cID := range delete {
			controllerNode.ControllerID = cID
			if err := tx.Query(ctx, deleteStmt, controllerNode).Run(); err != nil {
				return errors.Errorf("deleting controller node %q: %w", cID, err)
			}
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("curating controller nodes: %w", err)
	}
	return nil
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID. It is a no-op if they are already set to the same values.
func (st *State) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// uint64 values with the high bit set cause the driver to throw an error,
	// so we parse them as strings. The node_id is defined as being TEXT,
	// which makes no difference - it can still be scanned directly into
	// uint64 when querying the table.
	nodeStr := strconv.FormatUint(nodeID, 10)
	controllerNode := dbControllerNode{
		ControllerID:      controllerID,
		DqliteNodeID:      nodeStr,
		DqliteBindAddress: addr,
	}

	q := `
UPDATE controller_node 
SET    dqlite_node_id = $dbControllerNode.dqlite_node_id,
       dqlite_bind_address = $dbControllerNode.dqlite_bind_address 
WHERE  controller_id = $dbControllerNode.controller_id`
	stmt, err := st.Prepare(q, controllerNode)
	if err != nil {
		return errors.Errorf("preparing update controller node statement: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, controllerNode).Run()
		return errors.Capture(err)
	}))

}

// SelectDatabaseNamespace is responsible for selecting and returning the
// database namespace specified by namespace. If no namespace is registered an
// error satisfying [errors.NotFound] is returned.
func (st *State) SelectDatabaseNamespace(ctx context.Context, namespace string) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	dbNamespace := dbNamespace{Namespace: namespace}

	stmt, err := st.Prepare(`
SELECT &dbNamespace.* from namespace_list 
WHERE  namespace = $dbNamespace.namespace`, dbNamespace)
	if err != nil {
		return "", errors.Errorf("preparing select namespace statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, db *sqlair.TX) error {
		err := db.Query(ctx, stmt, dbNamespace).Get(&dbNamespace)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("namespace %q %w", namespace, controllernodeerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("selecting namespace %q: %w", namespace, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return namespace, nil
}

// SetRunningAgentBinaryVersion sets the running agent binary version for the
// provided controllerID. Any previously set values for this controllerID will
// be overwritten by this call.
//
// The following errors can be expected:
// - [controllernodeerrors.NotFound] if the controller node does not exist.
// - [coreerrors.NotSupported] if the architecture is unknown.
func (st *State) SetRunningAgentBinaryVersion(
	ctx context.Context,
	controllerID string,
	version coreagentbinary.Version,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	arch := architecture{Name: version.Arch}

	selectArchIdStmt, err := st.Prepare(`
SELECT id AS &architecture.id FROM architecture WHERE name = $architecture.name
`, arch)
	if err != nil {
		return errors.Capture(err)
	}

	controllerNodeAgentVer := controllerNodeAgentVersion{
		ControllerID: controllerID,
		Version:      version.Number.String(),
	}

	selectControllerNodeStmt, err := st.Prepare(`
SELECT controller_id AS &controllerNodeAgentVersion.*
FROM controller_node
WHERE controller_id = $controllerNodeAgentVersion.controller_id
	`, controllerNodeAgentVer)
	if err != nil {
		return errors.Capture(err)
	}

	upsertControllerNodeAgentVerStmt, err := st.Prepare(`
INSERT INTO controller_node_agent_version (*) VALUES ($controllerNodeAgentVersion.*)
ON CONFLICT (controller_id) DO
UPDATE SET version = excluded.version, architecture_id = excluded.architecture_id
`, controllerNodeAgentVer)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		// Ensure controller id exists in controller node before upserting controller node agent version.
		err = tx.Query(ctx, selectControllerNodeStmt, controllerNodeAgentVer).Get(&controllerNodeAgentVer)

		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"controller node %q does not exist", controllerID,
			).Add(controllernodeerrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if controller node %q exists: %w",
				controllerID, err,
			)
		}

		err = tx.Query(ctx, selectArchIdStmt, arch).Get(&arch)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is unsupported", version.Arch,
			).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Errorf(
				"looking up id for architecture %q: %w", version.Arch, err,
			)
		}

		controllerNodeAgentVer.ArchitectureID = arch.ID
		return tx.Query(ctx, upsertControllerNodeAgentVerStmt, controllerNodeAgentVer).Run()
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// IsControllerNode returns true if the supplied nodeID is a controller node.
func (st *State) IsControllerNode(ctx context.Context, nodeID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	controllerNode := dbControllerNode{ControllerID: nodeID}

	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &dbControllerNodeCount.count
FROM controller_node
WHERE controller_id = $dbControllerNode.controller_id`, controllerNode, dbControllerNodeCount{})
	if err != nil {
		return false, errors.Errorf("preparing select controller node statement: %w", err)
	}

	var result dbControllerNodeCount
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, controllerNode).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("selecting controller node %q: %w", nodeID, err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	} else if result.Count > 1 {
		// This is impossible with FK, but we should check anyway.
		return false, errors.Errorf("multiple controller nodes with ID %q", nodeID)
	}
	return result.Count == 1, nil
}

// NamespaceForWatchControllerNodes returns the namespace for watching
// controller nodes.
func (st *State) NamespaceForWatchControllerNodes() string {
	return "controller_node"
}

// NamespaceForWatchControllerAPIAddresses returns the namespace for watching
// controller api addresses.
func (st *State) NamespaceForWatchControllerAPIAddresses() string {
	return "controller_api_address"
}

// SetAPIAddresses sets the addresses for the provided controller node. It
// replaces any existing addresses and stores them in the api_controller_address
// table, with the format "host:port" as a string, as well as the is_agent flag
// indicating whether the address is available for agents.
//
// The following errors can be expected:
// - [controllernodeerrors.NotFound] if the controller node does not exist.
func (st *State) SetAPIAddresses(ctx context.Context, ctrlID string, addrs []controllernode.APIAddress) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	ident := controllerID{ID: ctrlID}

	checkControllerExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count 
FROM controller_node 
WHERE controller_id = $controllerID.controller_id
`, countResult{}, ident)
	if err != nil {
		return errors.Capture(err)
	}

	getExistingAddressesStmt, err := st.Prepare(`
SELECT &controllerAPIAddress.* 
FROM controller_api_address
WHERE controller_id = $controllerID.controller_id
`, controllerAPIAddress{}, ident)
	if err != nil {
		return errors.Capture(err)
	}

	type toRemoveAddresses []string
	deleteAddressesStmt, err := st.Prepare(`
DELETE FROM controller_api_address
WHERE controller_id = $controllerID.controller_id
AND address IN ($toRemoveAddresses[:])
`, ident, toRemoveAddresses{})
	if err != nil {
		return errors.Capture(err)
	}

	insertAddressesStmt, err := st.Prepare(`
INSERT INTO controller_api_address (*) VALUES ($controllerAPIAddress.*)
`, controllerAPIAddress{})
	if err != nil {
		return errors.Capture(err)
	}

	updateAddressesStmt, err := st.Prepare(`
UPDATE controller_api_address
SET is_agent = $controllerAPIAddress.is_agent
WHERE controller_id = $controllerAPIAddress.controller_id
AND address = $controllerAPIAddress.address
`, controllerAPIAddress{})
	if err != nil {
		return errors.Capture(err)
	}

	controllerAPIAddresses := encodeAPIAddresses(ctrlID, addrs)

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var countResult countResult
		if err := tx.Query(ctx, checkControllerExistsStmt, ident).Get(&countResult); err != nil {
			return errors.Errorf("checking if controller node %q exists: %w", ctrlID, err)
		}
		if countResult.Count == 0 {
			return errors.Errorf("controller node %q does not exist", ctrlID).Add(controllernodeerrors.NotFound)
		}

		var existingAddresses []controllerAPIAddress
		if err := tx.Query(ctx, getExistingAddressesStmt, ident).GetAll(&existingAddresses); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving existing api addresses for controller node %q: %w", ctrlID, err)
		}

		// Determine addresses to add and remove
		toAdd, toUpdate, toRemove := calculateAddressDeltas(existingAddresses, controllerAPIAddresses)

		if len(toAdd) > 0 {
			if err := tx.Query(ctx, insertAddressesStmt, toAdd).Run(); err != nil {
				return errors.Errorf("inserting api address for controller node %q: %w", ctrlID, err)
			}
		}

		if len(toRemove) > 0 {
			if err := tx.Query(ctx, deleteAddressesStmt, ident, toRemoveAddresses(toRemove)).Run(); err != nil {
				return errors.Errorf("deleting api address for controller node %q: %w", ctrlID, err)

			}
		}
		if len(toUpdate) > 0 {
			if err := tx.Query(ctx, updateAddressesStmt, toUpdate).Run(); err != nil {
				return errors.Errorf("updating api address for controller node %q: %w", ctrlID, err)
			}
		}

		return nil
	}))
}

// GetAPIAddresses returns the list of API addresses for the provided controller
// node.
func (st *State) GetAPIAddresses(ctx context.Context, ctrlID string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := controllerID{ID: ctrlID}

	stmt, err := st.Prepare(`
SELECT &controllerAPIAddressStr.* 
FROM controller_api_address
WHERE controller_id = $controllerID.controller_id
`, controllerAPIAddressStr{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddressStr
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return controllernodeerrors.EmptyAPIAddresses
		} else if err != nil {
			return errors.Errorf("getting api addresses for controller node %q: %w", ctrlID, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAPIAddresses(result), nil
}

// GetAllAPIAddressesForAgents returns a map of controller IDs to their API
// addresses that are available for agents. The map is keyed by controller ID,
// and the values are slices of strings representing the API addresses for each
// controller node.
func (st *State) GetAllAPIAddressesForAgents(ctx context.Context) (map[string][]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddress
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getAllAPIAddressesForAgents(ctx, tx)
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAllAPIAddresses(result), nil
}

func (st *State) getAllAPIAddressesForAgents(ctx context.Context, tx *sqlair.TX) ([]controllerAPIAddress, error) {
	stmt, err := st.Prepare(`
SELECT &controllerAPIAddress.* 
FROM controller_api_address
WHERE is_agent = true
`, controllerAPIAddress{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddress
	err = tx.Query(ctx, stmt).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, controllernodeerrors.EmptyAPIAddresses
	} else if err != nil {
		return nil, errors.Errorf("getting all api addresses for controller nodes: %w", err)
	}
	return result, nil
}

// GetAllAPIAddressesWithScopeForAgents returns all APIAddresses available for
// agents.
func (st *State) GetAllAPIAddressesWithScopeForAgents(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddress
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = st.getAllAPIAddressesForAgents(ctx, tx)
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAllScopedAPIAddresses(result), nil

}

// GetAPIAddressesForAgents returns the list of API address strings including
// port for the provided controller node that are available for agents.
func (st *State) GetAPIAddressesForAgents(ctx context.Context, ctrlID string) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := controllerID{ID: ctrlID}

	stmt, err := st.Prepare(`
SELECT &controllerAPIAddressStr.* 
FROM controller_api_address
WHERE controller_id = $controllerID.controller_id
AND is_agent = true
`, controllerAPIAddressStr{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddressStr
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return controllernodeerrors.EmptyAPIAddresses
		} else if err != nil {
			return errors.Errorf("getting api addresses for controller node %q: %w", ctrlID, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAPIAddresses(result), nil
}

// GetControllerIDs returns the list of controller IDs from the controller node
// records.
func (st *State) GetControllerIDs(ctx context.Context) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &controllerID.* 
FROM controller_node
`, controllerID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var controllerIDs []controllerID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&controllerIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			return controllernodeerrors.EmptyControllerIDs
		} else if err != nil {
			return errors.Errorf("getting controller node ids: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	res := make([]string, len(controllerIDs))
	for i, c := range controllerIDs {
		res[i] = c.ID
	}
	return res, nil
}

// calculateAddressDeltas returns the list of addresses to add, remove, and
// update from the controller node table given the existing and new addresses.
// The updated addresses are the list of addresses for which the IsAgent flag
// has changed.
func calculateAddressDeltas(existing, new []controllerAPIAddress) (toAdd []controllerAPIAddress, toUpdate []controllerAPIAddress, toRemove []string) {
	existingMap := make(map[string]controllerAPIAddress)
	newMap := make(map[string]controllerAPIAddress)

	for _, addr := range existing {
		existingMap[addr.Address] = addr
	}
	for _, addr := range new {
		newMap[addr.Address] = addr
	}

	// Check each address in the new set to determine additions and updates.
	for key, addr := range newMap {
		if existingAddr, found := existingMap[key]; !found {
			// Address doesn't exist in current state, so it needs to be added.
			toAdd = append(toAdd, addr)
		} else if existingAddr.IsAgent != addr.IsAgent {
			// Address exists but the IsAgent flag has changed, so it needs
			// updating.
			toUpdate = append(toUpdate, addr)
		}
		// If address exists with same IsAgent flag, no action needed.
	}

	// Check each address in the existing set to find removals.
	for key, addr := range existingMap {
		if _, found := newMap[key]; !found {
			// Address exists in current state but not in new set, so it needs
			// to be removed.
			toRemove = append(toRemove, addr.Address)
		}
	}

	return toAdd, toUpdate, toRemove
}

func decodeAllAPIAddresses(addrs []controllerAPIAddress) map[string][]string {
	result := make(map[string][]string)
	for _, addr := range addrs {
		if addr.Address == "" {
			continue
		}

		controllerID := addr.ControllerID
		if _, ok := result[controllerID]; !ok {
			result[controllerID] = []string{}
		}
		result[controllerID] = append(result[controllerID], addr.Address)
	}
	return result
}

func decodeAllScopedAPIAddresses(addrs []controllerAPIAddress) map[string]controllernode.APIAddresses {
	result := make(map[string]controllernode.APIAddresses, 0)
	for _, addr := range addrs {
		if addr.Address == "" {
			continue
		}
		controllerID := addr.ControllerID
		if _, ok := result[controllerID]; !ok {
			result[controllerID] = controllernode.APIAddresses{}
		}
		controllernodeAddr := controllernode.APIAddress{
			Address: addr.Address,
			IsAgent: addr.IsAgent,
			Scope:   network.Scope(addr.Scope),
		}
		result[controllerID] = append(result[controllerID], controllernodeAddr)
	}
	return result
}

func decodeAPIAddresses(addrs []controllerAPIAddressStr) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.Address)
	}
	return result
}

func encodeAPIAddresses(controllerID string, addrs []controllernode.APIAddress) []controllerAPIAddress {
	result := make([]controllerAPIAddress, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, controllerAPIAddress{
			ControllerID: controllerID,
			Address:      addr.Address,
			IsAgent:      addr.IsAgent,
			Scope:        string(addr.Scope),
		})
	}
	return result
}
