// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// DeleteUnit deletes the specified unit.
func (st *State) DeleteUnit(ctx context.Context, unitName string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	unitIDParam := sqlair.M{"unit_id": unitName}

	queryUnit := `SELECT uuid as &M.uuid FROM unit WHERE unit_id = $M.unit_id`
	queryUnitStmt, err := st.Prepare(queryUnit, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteUnit := `DELETE FROM unit WHERE unit_id = $M.unit_id`
	deleteUnitStmt, err := st.Prepare(deleteUnit, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM unit WHERE unit_id = $M.unit_id) 
`
	deleteNodeStmt, err := st.Prepare(deleteNode, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err = tx.Query(ctx, queryUnitStmt, unitIDParam).Get(result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for unit %q", unitName)
		}
		// Unit already deleted is a no op.
		if len(result) == 0 {
			return nil
		}
		if err := tx.Query(ctx, deleteUnitStmt, unitIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting unit %q", unitName)
		}
		if err := tx.Query(ctx, deleteNodeStmt, unitIDParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for unit  %q", unitName)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting unit %q", unitName)
}
