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
