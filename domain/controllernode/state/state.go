// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
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

// AddDqliteNodeID ensures a controller node exists for the supplied ID.
// It only inserts the controller_id; the dqlite_node_id and
// dqlite_bind_address columns are left NULL. These are populated later by
// AddDqliteNode when the Dqlite cluster admits the node.
// This separation exists because a controller pod registers its identity
// during UnitIntroduction (before joining the Dqlite cluster), and the
// Dqlite node information is added in a separate step once the cluster
// acknowledges the new peer.
func (st *State) AddDqliteNodeID(ctx context.Context, controllerID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	controllerNode := dbControllerNode{ControllerID: controllerID}
	insertStmt, err := st.Prepare(`
INSERT INTO controller_node (controller_id)
VALUES ($dbControllerNode.controller_id)
ON CONFLICT (controller_id) DO NOTHING
`, controllerNode)
	if err != nil {
		return errors.Errorf("preparing insert controller node statement: %w", err)
	}

	checkAliveStmt, err := st.Prepare(`
SELECT life_id AS &dbControllerNode.life_id
FROM controller_node
WHERE controller_id = $dbControllerNode.controller_id
`, controllerNode)
	if err != nil {
		return errors.Errorf("preparing controller node life query: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, insertStmt, controllerNode).Run(); err != nil {
			return errors.Capture(err)
		}
		var result dbControllerNode
		if err := tx.Query(ctx, checkAliveStmt, controllerNode).Get(&result); err != nil {
			return errors.Capture(err)
		}
		if result.LifeID != 0 {
			return errors.Errorf("controller node is not alive").Add(controllernodeerrors.NotFound)
		}
		return nil
	}))
}

// AddDqliteNode adds the Dqlite node ID and bind address for the input
// controller ID. If the controller ID already exists, it updates the
// Dqlite node ID and bind address.
//
// This is called separately from AddDqliteNodeID because the controller
// node identity is registered during UnitIntroduction (before the Dqlite
// cluster admits the peer). The Dqlite node information is only available
// once the cluster acknowledges the new node, which happens in a distinct
// lifecycle step.
func (st *State) AddDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	db, err := st.DB(ctx)
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
INSERT INTO controller_node (
    controller_id,
    dqlite_node_id,
    dqlite_bind_address
) VALUES ($dbControllerNode.*)
ON CONFLICT (controller_id) DO UPDATE SET
    dqlite_node_id = excluded.dqlite_node_id,
    dqlite_bind_address = excluded.dqlite_bind_address
WHERE controller_node.life_id = 0
`
	stmt, err := st.Prepare(q, controllerNode)
	if err != nil {
		return errors.Errorf("preparing update controller node statement: %w", err)
	}
	checkAliveStmt, err := st.Prepare(`
SELECT controller_id AS &dbControllerNode.controller_id
FROM controller_node
WHERE controller_id = $dbControllerNode.controller_id
AND life_id = 0
`, controllerNode)
	if err != nil {
		return errors.Errorf("preparing controller node life query: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, controllerNode).Run(); err != nil {
			return errors.Capture(err)
		}
		var result dbControllerNode
		if err := tx.Query(ctx, checkAliveStmt, controllerNode).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("controller node is not alive").Add(controllernodeerrors.NotFound)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	}))
}

// SelectDatabaseNamespace is responsible for selecting and returning the
// database namespace specified by namespace. If no namespace is registered an
// error satisfying [errors.NotFound] is returned.
func (st *State) SelectDatabaseNamespace(ctx context.Context, namespace string) (string, error) {
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	selectArchIdStmt, err := st.Prepare(`
SELECT id AS &architecture.id FROM architecture WHERE name = $architecture.name
`, architecture{})
	if err != nil {
		return errors.Capture(err)
	}

	selectControllerNodeStmt, err := st.Prepare(`
SELECT controller_id AS &controllerNodeAgentVersion.*
FROM controller_node
WHERE controller_id = $controllerNodeAgentVersion.controller_id
AND life_id = 0
	`, controllerNodeAgentVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	// We only update the version on upsert because there will never be a time
	// the same controller node will change its architecture.
	upsertControllerNodeAgentVerStmt, err := st.Prepare(`
INSERT INTO controller_node_agent_version (*) VALUES ($controllerNodeAgentVersion.*)
ON CONFLICT (controller_id) DO
UPDATE SET version = excluded.version
`, controllerNodeAgentVersion{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Ensure controller id exists in controller node before upserting controller node agent version.
		controllerNode := controllerNodeAgentVersion{ControllerID: controllerID}
		err := tx.Query(ctx, selectControllerNodeStmt, controllerNode).Get(&controllerNode)

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

		arch := architecture{Name: version.Arch}
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

		controllerNodeAgentVersionToSet := controllerNodeAgentVersion{
			ControllerID:   controllerID,
			Version:        version.Number.String(),
			ArchitectureID: arch.ID,
		}
		return tx.Query(ctx, upsertControllerNodeAgentVerStmt, controllerNodeAgentVersionToSet).Run()
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// IsControllerNode returns true if the supplied nodeID is a controller node.
func (st *State) IsControllerNode(ctx context.Context, nodeID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	controllerNode := dbControllerNode{ControllerID: nodeID}

	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &dbControllerNodeCount.count
FROM controller_node
WHERE controller_id = $dbControllerNode.controller_id
AND life_id < 2`, controllerNode, dbControllerNodeCount{})
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

// SetAPIAddresses replaces the client and agent address projections for the
// supplied controllers. All addresses are published for clients; addresses
// marked IsAgent are also published for agents. Both projections are updated
// in one transaction, preserving UUIDs for addresses that already exist.
//
// The following errors can be expected:
// - [controllernodeerrors.NotFound] if a controller node is missing or not alive.
func (st *State) SetAPIAddresses(ctx context.Context, addresses map[string]controllernode.APIAddresses) error {
	if len(addresses) == 0 {
		return nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	checkControllerExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM controller_node AS node
WHERE node.controller_id IN ($controllerIDs[:])
AND node.life_id = 0
`, countResult{}, controllerIDs{})
	if err != nil {
		return errors.Capture(err)
	}

	clients, agents, controllers := encodeAPIAddresses(addresses)
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var count countResult
		if err := tx.Query(ctx, checkControllerExistsStmt, controllers).Get(&count); err != nil {
			return errors.Errorf("checking controller nodes exist: %w", err)
		}
		if count.Count != len(controllers) {
			return errors.Errorf("controller nodes do not exist").Add(controllernodeerrors.NotFound)
		}
		if err := st.setAddressProjection(ctx, tx, "controller_client_address", controllers, clients); err != nil {
			return errors.Capture(err)
		}
		return st.setAddressProjection(ctx, tx, "controller_agent_address", controllers, agents)
	}))
}

// setAddressProjection reconciles addresses only for the supplied controllers.
// table must be one of the fixed projection table names used above.
func (st *State) setAddressProjection(ctx context.Context, tx *sqlair.TX, table string, controllers controllerIDs, addresses []controllerAddress) error {
	getExistingStmt, err := st.Prepare(fmt.Sprintf(`
SELECT address.* AS &controllerAddress.*
FROM %s AS address
WHERE address.controller_id IN ($controllerIDs[:])
`, table), controllerAddress{}, controllers)
	if err != nil {
		return errors.Capture(err)
	}
	deleteStmt, err := st.Prepare(fmt.Sprintf(`
DELETE FROM %s AS address
WHERE address.uuid = $controllerAddress.uuid
`, table), controllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	upsertStmt, err := st.Prepare(fmt.Sprintf(`
INSERT INTO %s AS address (uuid, controller_id, address, scope, priority)
VALUES ($controllerAddress.*)
ON CONFLICT (controller_id, address) DO UPDATE
SET scope = excluded.scope, priority = excluded.priority
WHERE address.scope != excluded.scope OR address.priority != excluded.priority
`, table), controllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}

	var existing []controllerAddress
	if err := tx.Query(ctx, getExistingStmt, controllers).GetAll(&existing); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("reading %s: %w", table, err)
	}
	// Comparing controller ID and address preserves independently published
	// rows when multiple controllers report the same address.
	type addressKey struct {
		controllerID string
		address      string
	}
	current := make(map[addressKey]struct{}, len(addresses))
	for _, address := range addresses {
		current[addressKey{address.ControllerID, address.Address}] = struct{}{}
	}
	for _, address := range existing {
		if _, ok := current[addressKey{address.ControllerID, address.Address}]; ok {
			continue
		}
		if err := tx.Query(ctx, deleteStmt, address).Run(); err != nil {
			return errors.Errorf("deleting from %s: %w", table, err)
		}
	}
	if len(addresses) > 0 {
		if err := tx.Query(ctx, upsertStmt, addresses).Run(); err != nil {
			return errors.Errorf("writing %s: %w", table, err)
		}
	}
	return nil
}

// GetAPIAddressesForAgents returns APIAddresses available for agents.
func (st *State) GetAPIAddressesForAgents(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var controllerAddresses []controllerAPIAddress
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		controllerAddresses, err = st.getAllAPIAddressesForAgents(ctx, tx)
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAPIAddresses(controllerAddresses), nil
}

// GetAPIAddressesForClients returns APIAddresses available for clients. These are
// APIAddresses independent of is_agent value.
func (st *State) GetAPIAddressesForClients(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var controllerAddresses []controllerAPIAddress
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		controllerAddresses, err = st.getAllAPIAddressesForClients(ctx, tx)
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return decodeAPIAddresses(controllerAddresses), nil
}

// GetAllCloudLocalAPIAddresses returns a string slice of api
// addresses available for clients. The list only contains cloud
// local addresses. The returned strings are IP address only without
// port numbers.
func (st *State) GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &controllerAPIAddressStr.*
FROM   controller_api_address
WHERE  scope = "local-cloud"
`, controllerAPIAddressStr{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddressStr
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return controllernodeerrors.EmptyAPIAddresses
		} else if err != nil {
			return errors.Errorf("getting all cloud local api addresses for controller nodes: %w", err)
		}
		return err
	}); err != nil {
		return nil, errors.Capture(err)
	}

	returnStrings := make([]string, 0, len(result))
	for i := range result {
		returnStrings = append(returnStrings, result[i].Address)
	}
	return returnStrings, nil
}

// GetControllerIDs returns the list of controller IDs from the controller node
// records.
func (st *State) GetControllerIDs(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &controllerID.* 
FROM controller_node
WHERE life_id < 2
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

func (st *State) getAllAPIAddressesForClients(ctx context.Context, tx *sqlair.TX) ([]controllerAPIAddress, error) {
	stmt, err := st.Prepare(`
SELECT &controllerAPIAddress.* 
FROM controller_api_address
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

func decodeAPIAddresses(addrs []controllerAPIAddress) map[string]controllernode.APIAddresses {
	result := make(map[string]controllernode.APIAddresses, 0)
	for _, addr := range addrs {
		if addr.Address == "" {
			continue
		}

		controllerID := addr.ControllerID
		if _, ok := result[controllerID]; !ok {
			result[controllerID] = controllernode.APIAddresses{}
		}

		controllerNodeAddr := controllernode.APIAddress{
			Address: addr.Address,
			IsAgent: addr.IsAgent,
			Scope:   network.Scope(addr.Scope),
		}
		result[controllerID] = append(result[controllerID], controllerNodeAddr)
	}

	return result
}

func encodeAPIAddresses(controllerAddrs map[string]controllernode.APIAddresses) (clients, agents []controllerAddress, controllers controllerIDs) {
	controllers = make(controllerIDs, 0, len(controllerAddrs))
	for controllerID, addrs := range controllerAddrs {
		controllers = append(controllers, controllerID)
		// As with the legacy projection, the last entry for an address wins.
		// Resolve duplicates before splitting the two audiences.
		byAddress := make(map[string]controllernode.APIAddress, len(addrs))
		for _, addr := range addrs {
			byAddress[addr.Address] = addr
		}
		for _, addr := range byAddress {
			row := controllerAddress{
				UUID:         addr.UUID,
				ControllerID: controllerID,
				Address:      addr.Address,
				Scope:        string(addr.Scope),
				Priority:     addr.Priority,
			}
			clients = append(clients, row)
			if addr.IsAgent {
				agents = append(agents, row)
			}
		}
	}
	return clients, agents, controllers
}
