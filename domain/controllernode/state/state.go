// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
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
		return errors.Trace(err)
	}

	// These are never going to be many at a time. Just repeat as required.
	iq := "INSERT INTO controller_node (controller_id) VALUES (?)"
	dq := "DELETE FROM controller_node WHERE controller_id = ?"

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		for _, cID := range insert {
			if _, err := tx.ExecContext(ctx, iq, cID); err != nil {
				return errors.Annotatef(err, "inserting controller node %q", cID)
			}
		}

		for _, cID := range delete {
			if _, err := tx.ExecContext(ctx, dq, cID); err != nil {
				return errors.Annotatef(err, "deleting controller node %q", cID)
			}
		}

		return nil
	})

	return errors.Annotate(err, "curating controller nodes")
}

// UpdateDqliteNode sets the Dqlite node ID and bind address for the input
// controller ID. It is a no-op if they are already set to the same values.
func (st *State) UpdateDqliteNode(ctx context.Context, controllerID string, nodeID uint64, addr string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE controller_node 
SET    dqlite_node_id = ?,
       bind_address = ? 
WHERE  controller_id = ?
AND    (dqlite_node_id != ? OR bind_address != ?)`

	// uint64 values with the high bit set cause the driver to throw an error,
	// so we parse them as strings. The node_id is defined as being TEXT,
	// which makes no difference - it can still be scanned directly into
	// uint64 when querying the table.
	nodeStr := strconv.FormatUint(nodeID, 10)

	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, nodeStr, addr, controllerID, nodeStr, addr)
		return errors.Trace(err)
	}))
}

// SelectModelUUID simply selects the input model UUID from the
// model table, thereby verifying whether it exists. If no model is found
// for the provided uuid then an error satisfying [modelerrors.NotFound] will
// be returned.
func (st *State) SelectModelUUID(ctx context.Context, uuid model.UUID) (model.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.StdTxn(ctx, func(ctx context.Context, db *sql.Tx) error {
		row := db.QueryRowContext(ctx, "SELECT uuid FROM model WHERE uuid = ?", uuid.String())

		return row.Scan(&uuid)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return model.UUID(""), fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return model.UUID(""), fmt.Errorf("selecting model for uuid %q: %w", uuid, domain.CoerceError(err))
	}

	return uuid, nil
}
