// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/domain"
)

// State represents database interactions dealing with controller nodes.
type State struct {
	*domain.StateBase
}

// NewState returns a new controller node state
// based on the input database factory method.
func NewState(factory domain.TxnRunnerFactory) *State {
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

// UpdateBootstrapNodeBindAddress sets the input address as the Dqlite
// bind address of the original bootstrapped controller node.
func (st *State) UpdateBootstrapNodeBindAddress(ctx context.Context, addr string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// This ensures a no-op if previously set.
	q := `
UPDATE controller_node 
SET    bind_address = ? 
WHERE  controller_id = 0
AND    bind_address = '127.0.0.1'`

	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, addr)
		return errors.Trace(err)
	}))
}

// SelectModelUUID simply selects the input model UUID from the
// model_list table, thereby verifying whether it exists.
func (st *State) SelectModelUUID(ctx context.Context, modelUUID string) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var uuid string
	err = db.StdTxn(ctx, func(ctx context.Context, db *sql.Tx) error {
		row := db.QueryRowContext(ctx, "SELECT uuid FROM model_list WHERE uuid = ?", modelUUID)

		if err := row.Scan(&uuid); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.NotFoundf("model UUID %q", modelUUID)
			}
			return errors.Trace(err)
		}

		if err := row.Err(); err != nil {
			return errors.Trace(err)
		}

		return nil
	})

	return uuid, errors.Trace(err)
}
