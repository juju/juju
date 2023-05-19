// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/domain"
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

// Add ....
func (s *State) Add(ctx context.Context, key string, value string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "INSERT INTO controller_config (key, value) VALUES (?, ?);"
		result, err := tx.ExecContext(ctx, stmt, key, value)
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

// Delete ...
func (s *State) Delete(ctx context.Context, key string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := "DELETE FROM controller_config WHERE key = ?;"
		result, err := tx.ExecContext(ctx, stmt, key)
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
