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
	"github.com/juju/juju/domain"
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
		ControllerID: controllerID,
		DQLiteNodeID: nodeStr,
		BindAddress:  addr,
	}

	q := `
UPDATE controller_node 
SET    dqlite_node_id = $dbControllerNode.dqlite_node_id,
       bind_address = $dbControllerNode.bind_address 
WHERE  controller_id = $dbControllerNode.controller_id
AND    (dqlite_node_id != $dbControllerNode.dqlite_node_id OR bind_address != $dbControllerNode.bind_address)`
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
