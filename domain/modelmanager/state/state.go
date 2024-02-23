// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	jujudb "github.com/juju/juju/internal/database"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Create takes a model UUID and creates a new model.
// Note: no validation is performed on the UUID, as that is performed at the
// service layer.
func (s *State) Create(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return Create(ctx, uuid, tx)
	})
}

// List returns a list of all model UUIDs.
// The list of models returned are the ones that are just present in the model
// manager list. This means that the model is not deleted.
func (s *State) List(ctx context.Context) ([]coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var models []coremodel.UUID
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `SELECT uuid FROM model_list;`
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()

		for rows.Next() {
			var model coremodel.UUID
			if err := rows.Scan(&model); err != nil {
				return errors.Trace(err)
			}
			if err := rows.Err(); err != nil {
				return errors.Trace(err)
			}
			models = append(models, model)
		}
		return nil
	})
	return models, errors.Trace(err)
}

// Delete takes a model UUID and deletes a new model.
// Note: no validation is performed on the UUID, as that is performed at the
// service layer.
func (s *State) Delete(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `DELETE FROM model_list WHERE uuid = ?;`
		result, err := tx.ExecContext(ctx, stmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if num, err := result.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return fmt.Errorf("%w %q; expected 1 row to be deleted, got %d", domain.ErrNoRecord, uuid, num)
		}
		return nil
	})
}

// Create takes a model UUID and an established transaction onto the database
// and creates the model.
func Create(ctx context.Context, uuid coremodel.UUID, tx *sql.Tx) error {
	stmt := "INSERT INTO model_list (uuid) VALUES (?);"
	result, err := tx.ExecContext(ctx, stmt, uuid)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf("model for uuid %q %w", uuid, modelerrors.AlreadyExists)
	} else if err != nil {
		return errors.Trace(err)
	}

	if num, err := result.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return errors.Errorf("expected 1 row to be inserted, got %d", num)
	}
	return nil
}
