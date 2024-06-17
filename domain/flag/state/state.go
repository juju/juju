// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// SetFlag sets the value of a flag.
// Description is used to describe the flag and its potential state.
func (s *State) SetFlag(ctx context.Context, flagName string, value bool, description string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	flag := dbFlag{Name: flagName, Value: value, Description: description}

	stmt, err := s.Prepare(`
INSERT INTO   flag (name, value, description)
VALUES        ($dbFlag.*)
ON CONFLICT (name) DO UPDATE SET value = excluded.value,
                                 description = excluded.description;
`, flag)
	if err != nil {
		return errors.Annotate(err, "preparing set flag stmt")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, stmt, flag).Get(&outcome)
		if err != nil {
			return errors.Trace(err)
		}
		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if affected != 1 {
			return errors.Errorf("unexpected number of rows affected: %d (should be 1)", affected)
		}
		return nil
	})
	if err != nil {
		return errors.Trace(domain.CoerceError(err))
	}

	s.logger.Debugf("set flag %q to %v", flagName, value)

	return nil
}

// GetFlag returns the value of a flag.
func (s *State) GetFlag(ctx context.Context, flagName string) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	flag := dbFlag{Name: flagName}

	stmt, err := s.Prepare(`
SELECT &dbFlag.value
FROM   flag
WHERE  name = $dbFlag.name;
	`, flag)
	if err != nil {
		return false, errors.Annotate(err, "preparing select flag stmt")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, flag).Get(&flag)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.NotFoundf("flag %q", flagName)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return false, errors.Trace(domain.CoerceError(err))
	}
	return flag.Value, nil
}
