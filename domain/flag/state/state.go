// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

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

// SetFlag sets the value of a flag.
// Description is used to describe the flag and it's potential state.
func (s *State) SetFlag(ctx context.Context, flag string, value bool, description string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	query := `
INSERT INTO flag (name, value, description)
VALUES (?, ?, ?)
ON CONFLICT (name) DO UPDATE SET value = excluded.value,
                                 description = excluded.description;
`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, query, flag, value, description)
		if err != nil {
			return errors.Trace(err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if affected != 1 {
			return errors.Errorf("unexpected number of rows affected: %d", affected)
		}
		return nil
	})
	if err != nil {
		return errors.Trace(err)
	}

	s.logger.Debugf("set flag %q to %v", flag, value)

	return nil
}

// GetFlag returns the value of a flag.
func (s *State) GetFlag(ctx context.Context, flag string) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	query := `
SELECT value
FROM flag
WHERE name = ?;
`

	var value bool
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, query, flag)
		if err := row.Scan(&value); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return errors.NotFoundf("flag %q", flag)
			}
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Trace(err)
	}
	return value, nil
}
