// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentpassword"
	agentpassworderrors "github.com/juju/juju/domain/agentpassword/errors"
	"github.com/juju/juju/internal/errors"
)

// State defines the access mechanism for interacting with passwords in the
// context of the model database.
type State struct {
	*domain.StateBase
}

// NewState constructs a new state for interacting with the underlying passwords
// of a model.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetUnitPasswordHash sets the password hash for the given unit.
func (s *State) SetUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash agentpassword.PasswordHash) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	args := unitPasswordHash{
		UUID:         unitUUID,
		PasswordHash: passwordHash,
	}

	query := `
UPDATE unit
SET password_hash = $unitPasswordHash.password_hash,
    password_hash_algorithm_id = 0
WHERE uuid = $unitPasswordHash.uuid;
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return errors.Errorf("preparing statement to set password hash: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, args).Get(&outcome); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		result := outcome.Result()
		if err != nil {
			return errors.Errorf("getting result of setting password hash: %w", err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		} else if affected == 0 {
			return agentpassworderrors.UnitNotFound
		}
		return nil
	})
	return errors.Capture(err)
}

// MatchesUnitPasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *State) MatchesUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash agentpassword.PasswordHash) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, err
	}

	args := validateUnitPasswordHash{
		UUID:         unitUUID,
		PasswordHash: passwordHash,
	}

	query := `
SELECT COUNT(*) AS &validateUnitPasswordHash.count FROM unit
WHERE uuid = $validateUnitPasswordHash.uuid
AND password_hash = $validateUnitPasswordHash.password_hash;
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return false, errors.Errorf("preparing statement to set password hash: %w", err)
	}

	var count int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&args); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		count = args.Count
		return nil
	})
	return count > 0, errors.Capture(err)
}

// GetUnitUUID returns the UUID of the unit with the given name, returning an
// error satisfying [agentpassworderrors.UnitNotFound] if the unit does not exist.
func (st *State) GetUnitUUID(ctx context.Context, unitName unit.Name) (unit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var unitUUID unit.UUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = st.getUnitUUID(ctx, tx, unitName.String())
		return errors.Capture(err)
	})
	return unitUUID, errors.Capture(err)
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, name string) (unit.UUID, error) {
	u := unitName{Name: name}

	selectUnitUUIDStmt, err := st.Prepare(`
SELECT &unitName.uuid
FROM unit
WHERE name=$unitName.name`, u)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, selectUnitUUIDStmt, u).Get(&u)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%s %w", name, agentpassworderrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}

// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
func (st *State) GetAllUnitPasswordHashes(ctx context.Context) (agentpassword.UnitPasswordHashes, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &unitPasswordHashes.* FROM v_unit_password_hash`
	stmt, err := st.Prepare(query, unitPasswordHashes{})
	if err != nil {
		return nil, errors.Errorf("preparing statement to get all unit password hashes: %w", err)
	}

	var results []unitPasswordHashes
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("getting all unit password hashes: %w", err)
	}

	return encodePasswordHashes(results), nil
}

func encodePasswordHashes(results []unitPasswordHashes) agentpassword.UnitPasswordHashes {
	ret := make(agentpassword.UnitPasswordHashes)
	for _, r := range results {
		ret[r.UnitName] = r.PasswordHash
	}
	return ret
}
