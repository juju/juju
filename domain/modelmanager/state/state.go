// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/errors"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/modelmanager/service"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory domain.DBFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// Create takes a model UUID and creates a new model.
// Note: no validation is performed on the UUID, as that is performed at the
// service layer.
func (s *State) Create(ctx context.Context, uuid service.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO model_list (uuid) VALUES (?);"
		result, err := tx.ExecContext(ctx, stmt, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		if num, err := result.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return errors.Errorf("expected 1 row to be inserted, got %d", num)
		}
		return nil
	})
}
