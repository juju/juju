// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/passwords"
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
func (s *State) SetUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash passwords.PasswordHash) error {
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
SET password_hash = $unitPasswordHash.password_hash
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
			return applicationerrors.UnitNotFound
		}
		return nil
	})
	return errors.Capture(err)
}

// GetUnitUUID returns the UUID of the unit with the given name, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit does not exist.
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
		return "", errors.Errorf("%s %w", name, applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}

// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
func (st *State) GetAllUnitPasswordHashes(ctx context.Context) (map[string]map[unit.Name]passwords.PasswordHash, error) {
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

	ret := make(map[string]map[unit.Name]passwords.PasswordHash)
	for _, r := range results {
		if _, ok := ret[r.ApplicationName]; !ok {
			ret[r.ApplicationName] = make(map[unit.Name]passwords.PasswordHash)
		}
		ret[r.ApplicationName][r.UnitName] = r.PasswordHash
	}
	return ret, nil

}
