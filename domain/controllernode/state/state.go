// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"strconv"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
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
