// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
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

// NamespaceForWatchControllerAgentAddresses returns the namespace for watching
// controller agent addresses.
func (st *State) NamespaceForWatchControllerAgentAddresses() string {
	return "controller_agent_address"
}

// NamespaceForWatchControllerClientAddresses returns the namespace for watching
// controller client addresses.
func (st *State) NamespaceForWatchControllerClientAddresses() string {
	return "controller_client_address"
}

// NamespaceForWatchControllerPeerAddresses returns the namespace for watching
// controller peer addresses.
func (st *State) NamespaceForWatchControllerPeerAddresses() string {
	return "controller_peer_address"
}

// SetAPIAddresses atomically replaces all client, agent and peer address
// projections. Empty projections are authoritative. The named projection keys
// must exactly match the alive or dying controller membership. The empty key
// represents shared client and agent addresses and is excluded from membership.
// Callers must not supply peer addresses under the empty key because peer
// addresses require a controller identity.
//
// The following errors can be expected:
// - [controllernodeerrors.StaleControllerMembership] if the named projection
// keys do not exactly match the alive or dying controller nodes.
func (st *State) SetAPIAddresses(ctx context.Context, projections controllernode.APIAddressProjections) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	controllerIDsStmt, err := st.Prepare(`
SELECT node.controller_id AS &controllerID.controller_id
FROM controller_node AS node
WHERE node.life_id < 2
`, controllerID{})
	if err != nil {
		return errors.Capture(err)
	}

	clients, agents, peers := encodeAPIAddressProjections(projections)
	expected := make(map[string]struct{}, len(projections))
	for controllerID := range projections {
		if controllerID == "" {
			continue
		}
		expected[controllerID] = struct{}{}
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var controllers []controllerID
		if err := tx.Query(ctx, controllerIDsStmt).GetAll(&controllers); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("reading controller membership: %w", err)
		}
		if len(controllers) != len(expected) {
			return errors.Errorf("controller membership changed: %w", controllernodeerrors.StaleControllerMembership)
		}
		for _, controller := range controllers {
			if _, ok := expected[controller.ID]; !ok {
				return errors.Errorf("controller membership changed: %w", controllernodeerrors.StaleControllerMembership)
			}
		}
		if err := st.setClientAddressProjection(ctx, tx, clients); err != nil {
			return errors.Capture(err)
		}
		if err := st.setAgentAddressProjection(ctx, tx, agents); err != nil {
			return errors.Capture(err)
		}
		return st.setPeerAddressProjection(ctx, tx, peers)
	}))
}

func (st *State) setClientAddressProjection(ctx context.Context, tx *sqlair.TX, desired []publishedControllerAddress) error {
	readStmt, err := st.Prepare(`
SELECT address.* AS &publishedControllerAddress.*
FROM controller_client_address AS address
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	removeStmt, err := st.Prepare(`
DELETE FROM controller_client_address AS address
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	updateStmt, err := st.Prepare(`
UPDATE controller_client_address AS address
SET scope = $publishedControllerAddress.scope,
    priority = $publishedControllerAddress.priority
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO controller_client_address AS address (uuid, controller_id, address, scope, priority)
VALUES ($publishedControllerAddress.*)
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	return reconcileAddressProjection(ctx, tx, "controller_client_address", desired, readStmt, removeStmt, updateStmt, insertStmt)
}

func (st *State) setAgentAddressProjection(ctx context.Context, tx *sqlair.TX, desired []publishedControllerAddress) error {
	readStmt, err := st.Prepare(`
SELECT address.* AS &publishedControllerAddress.*
FROM controller_agent_address AS address
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	removeStmt, err := st.Prepare(`
DELETE FROM controller_agent_address AS address
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	updateStmt, err := st.Prepare(`
UPDATE controller_agent_address AS address
SET scope = $publishedControllerAddress.scope,
    priority = $publishedControllerAddress.priority
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO controller_agent_address AS address (uuid, controller_id, address, scope, priority)
VALUES ($publishedControllerAddress.*)
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	return reconcileAddressProjection(ctx, tx, "controller_agent_address", desired, readStmt, removeStmt, updateStmt, insertStmt)
}

func (st *State) setPeerAddressProjection(ctx context.Context, tx *sqlair.TX, desired []publishedControllerAddress) error {
	readStmt, err := st.Prepare(`
SELECT address.* AS &publishedControllerAddress.*
FROM controller_peer_address AS address
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	removeStmt, err := st.Prepare(`
DELETE FROM controller_peer_address AS address
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	updateStmt, err := st.Prepare(`
UPDATE controller_peer_address AS address
SET scope = $publishedControllerAddress.scope,
    priority = $publishedControllerAddress.priority
WHERE address.uuid = $publishedControllerAddress.uuid
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	insertStmt, err := st.Prepare(`
INSERT INTO controller_peer_address AS address (uuid, controller_id, address, scope, priority)
VALUES ($publishedControllerAddress.*)
`, publishedControllerAddress{})
	if err != nil {
		return errors.Capture(err)
	}
	return reconcileAddressProjection(ctx, tx, "controller_peer_address", desired, readStmt, removeStmt, updateStmt, insertStmt)
}

func reconcileAddressProjection(
	ctx context.Context,
	tx *sqlair.TX,
	projection string,
	desired []publishedControllerAddress,
	readStmt, removeStmt, updateStmt, insertStmt *sqlair.Statement,
) error {
	var existing []publishedControllerAddress
	if err := tx.Query(ctx, readStmt).GetAll(&existing); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("reading %s: %w", projection, err)
	}
	type addressKey struct {
		controllerID sql.Null[string]
		address      string
	}
	desiredByKey := make(map[addressKey]publishedControllerAddress, len(desired))
	for _, address := range desired {
		desiredByKey[addressKey{controllerID: address.ControllerID, address: address.Address}] = address
	}
	for _, address := range existing {
		key := addressKey{controllerID: address.ControllerID, address: address.Address}
		want, ok := desiredByKey[key]
		if !ok {
			if err := tx.Query(ctx, removeStmt, address).Run(); err != nil {
				return errors.Errorf("deleting from %s: %w", projection, err)
			}
			continue
		}
		delete(desiredByKey, key)
		if address.Scope == want.Scope && address.Priority == want.Priority {
			continue
		}
		want.UUID = address.UUID
		if err := tx.Query(ctx, updateStmt, want).Run(); err != nil {
			return errors.Errorf("updating %s: %w", projection, err)
		}
	}
	for _, address := range desiredByKey {
		if err := tx.Query(ctx, insertStmt, address).Run(); err != nil {
			return errors.Errorf("writing %s: %w", projection, err)
		}
	}
	return nil
}

func encodeAPIAddressProjections(projections controllernode.APIAddressProjections) (clients, agents, peers []publishedControllerAddress) {
	for controllerID, projection := range projections {
		identity := sql.Null[string]{V: controllerID, Valid: controllerID != ""}
		clients = append(clients, encodePublishedAddresses(identity, projection.Clients)...)
		agents = append(agents, encodePublishedAddresses(identity, projection.Agents)...)
		peers = append(peers, encodePublishedAddresses(identity, projection.Peers)...)
	}
	return clients, agents, peers
}

func encodePublishedAddresses(controllerID sql.Null[string], addresses controllernode.APIAddresses) []publishedControllerAddress {
	result := make([]publishedControllerAddress, len(addresses))
	for i, address := range addresses {
		result[i] = publishedControllerAddress{
			UUID:         address.UUID,
			ControllerID: controllerID,
			Address:      address.Address,
			Scope:        string(address.Scope),
			Priority:     address.Priority,
		}
	}
	return result
}

// GetAPIAddressesForAgents returns the agent projection, grouped by controller
// ID. Shared endpoints are grouped under the empty controller ID.
func (st *State) GetAPIAddressesForAgents(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	return st.getAPIAddresses(ctx, "controller_agent_address")
}

// GetAPIAddressesForClients returns the client projection, grouped by controller
// ID. Shared endpoints are grouped under the empty controller ID.
func (st *State) GetAPIAddressesForClients(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	return st.getAPIAddresses(ctx, "controller_client_address")
}

// GetAPIAddressesForPeers returns the peer projection, grouped by controller
// ID. Peer addresses always have a controller identity.
func (st *State) GetAPIAddressesForPeers(ctx context.Context) (map[string]controllernode.APIAddresses, error) {
	return st.getAPIAddresses(ctx, "controller_peer_address")
}

// getAPIAddresses reads one projection in priority order within each group.
// table must be one of the fixed projection table names used above.
func (st *State) getAPIAddresses(ctx context.Context, table string) (map[string]controllernode.APIAddresses, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	query, ok := map[string]string{
		"controller_agent_address": `
SELECT address.uuid AS &controllerAddress.uuid,
       COALESCE(address.controller_id, '') AS &controllerAddress.controller_id,
       address.address AS &controllerAddress.address,
       address.scope AS &controllerAddress.scope,
       address.priority AS &controllerAddress.priority
FROM controller_agent_address AS address
ORDER BY address.controller_id, address.priority, address.address
`,
		"controller_client_address": `
SELECT address.uuid AS &controllerAddress.uuid,
       COALESCE(address.controller_id, '') AS &controllerAddress.controller_id,
       address.address AS &controllerAddress.address,
       address.scope AS &controllerAddress.scope,
       address.priority AS &controllerAddress.priority
FROM controller_client_address AS address
ORDER BY address.controller_id, address.priority, address.address
`,
		"controller_peer_address": `
SELECT address.uuid AS &controllerAddress.uuid,
       address.controller_id AS &controllerAddress.controller_id,
       address.address AS &controllerAddress.address,
       address.scope AS &controllerAddress.scope,
       address.priority AS &controllerAddress.priority
FROM controller_peer_address AS address
ORDER BY address.controller_id, address.priority, address.address
`,
	}[table]
	if !ok {
		return nil, errors.Errorf("unsupported controller address projection %q", table)
	}
	stmt, err := st.Prepare(query, controllerAddress{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var addresses []controllerAddress
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&addresses)
		if errors.Is(err, sqlair.ErrNoRows) {
			return controllernodeerrors.EmptyAPIAddresses
		} else if err != nil {
			return errors.Errorf("reading %s: %w", table, err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return decodeAPIAddresses(addresses), nil
}

// GetAllCloudLocalAPIAddresses returns client API addresses with cloud-local
// scope, including shared endpoints. Addresses include port numbers.
func (st *State) GetAllCloudLocalAPIAddresses(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT address.address AS &controllerAPIAddressStr.address
FROM controller_client_address AS address
WHERE address.scope = 'local-cloud'
ORDER BY address.controller_id, address.priority, address.address
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

// GetAllAPIAddressesForCertificates returns the union of client and peer
// addresses in one database snapshot. An empty result is valid when neither
// projection contains an address.
func (st *State) GetAllAPIAddressesForCertificates(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
WITH certificate_addresses AS (
    SELECT client.address AS address
    FROM controller_client_address AS client
    WHERE client.address != ''
    UNION
    SELECT peer.address AS address
    FROM controller_peer_address AS peer
    WHERE peer.address != ''
)
SELECT certificate.address AS &controllerAPIAddressStr.address
FROM certificate_addresses AS certificate
ORDER BY certificate.address
`, controllerAPIAddressStr{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []controllerAPIAddressStr
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = nil
		if err := tx.Query(ctx, stmt).GetAll(&result); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting controller certificate addresses: %w", err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	addresses := make([]string, len(result))
	for i, address := range result {
		addresses[i] = address.Address
	}
	return addresses, nil
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

func decodeAPIAddresses(addrs []controllerAddress) map[string]controllernode.APIAddresses {
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
			UUID:     addr.UUID,
			Address:  addr.Address,
			Scope:    network.Scope(addr.Scope),
			Priority: addr.Priority,
		}
		result[controllerID] = append(result[controllerID], controllerNodeAddr)
	}

	return result
}
