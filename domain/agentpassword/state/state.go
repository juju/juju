// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentpassword"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
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

	args := entityPasswordHash{
		UUID:         unitUUID.String(),
		PasswordHash: passwordHash,
	}

	query := `
UPDATE unit
SET    password_hash = $entityPasswordHash.password_hash,
       password_hash_algorithm_id = 0
WHERE  uuid = $entityPasswordHash.uuid;
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

// MatchesUnitPasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *State) MatchesUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash agentpassword.PasswordHash) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, err
	}

	args := validatePasswordHash{
		UUID:         unitUUID.String(),
		PasswordHash: passwordHash,
	}

	query := `
SELECT COUNT(*) AS &validatePasswordHash.count FROM unit
WHERE  uuid = $validatePasswordHash.uuid
AND    password_hash = $validatePasswordHash.password_hash;
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

	return encodeUnitPasswordHashes(results), nil
}

// GetUnitUUID returns the UUID of the unit with the given name, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit does not exist.
func (st *State) GetUnitUUID(ctx context.Context, unitName unit.Name) (unit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var unitUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = st.getUnitUUID(ctx, tx, unitName.String())
		return errors.Capture(err)
	})
	return unit.UUID(unitUUID), errors.Capture(err)
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, name string) (string, error) {
	u := entityName{Name: name}

	stmt, err := st.Prepare(`
SELECT &entityName.uuid
FROM   unit
WHERE  name=$entityName.name`, u)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, u).Get(&u)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%s %w", name, applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Errorf("looking up unit UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}

// SetMachinePasswordHash sets the password hash for the given machine.
func (s *State) SetMachinePasswordHash(ctx context.Context, machineUUID machine.UUID, passwordHash agentpassword.PasswordHash) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	args := entityPasswordHash{
		UUID:         machineUUID.String(),
		PasswordHash: passwordHash,
	}

	query := `
UPDATE machine
SET    password_hash = $entityPasswordHash.password_hash,
       password_hash_algorithm_id = 0
WHERE  uuid = $entityPasswordHash.uuid;
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
		} else if affected, err := result.RowsAffected(); err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		} else if affected == 0 {
			return machineerrors.MachineNotFound
		}
		return nil
	})
	return errors.Capture(err)
}

// MatchesMachinePasswordHashWithNonce checks if the password with a nonce is
// valid or not against the password hash stored in the database.
func (s *State) MatchesMachinePasswordHashWithNonce(
	ctx context.Context,
	machineUUID machine.UUID,
	passwordHash agentpassword.PasswordHash,
	nonce string,
) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, err
	}

	args := validatePasswordHashWithNonce{
		UUID:         machineUUID.String(),
		PasswordHash: passwordHash,
		Nonce:        nonce,
	}

	query := `
SELECT COUNT(*) AS &validatePasswordHashWithNonce.count FROM machine
WHERE  uuid = $validatePasswordHashWithNonce.uuid
AND    password_hash = $validatePasswordHashWithNonce.password_hash
AND    nonce = $validatePasswordHashWithNonce.nonce;
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

// GetAllMachinePasswordHashes returns a map of machine names to password hashes.
func (st *State) GetAllMachinePasswordHashes(ctx context.Context) (agentpassword.MachinePasswordHashes, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &entityNamePasswordHashes.* FROM machine`
	stmt, err := st.Prepare(query, entityNamePasswordHashes{})
	if err != nil {
		return nil, errors.Errorf("preparing statement to get all machine password hashes: %w", err)
	}

	var results []entityNamePasswordHashes
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

	return encodeMachinePasswordHashes(results), nil
}

// GetMachineUUID returns the UUID of the machine with the given name, returning
// an error satisfying [machineerrors.MachineNotFound] if the machine does not
// exist.
func (st *State) GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var machineUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineUUID, err = st.getMachineUUID(ctx, tx, machineName.String())
		return errors.Capture(err)
	})
	return machine.UUID(machineUUID), errors.Capture(err)
}

func (st *State) getMachineUUID(ctx context.Context, tx *sqlair.TX, name string) (string, error) {
	u := entityName{Name: name}

	stmt, err := st.Prepare(`
SELECT &entityName.uuid
FROM   machine
WHERE  name=$entityName.name`, u)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, u).Get(&u)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%s %w", name, applicationerrors.MachineNotFound)
	} else if err != nil {
		return "", errors.Errorf("looking up machine UUID for %q: %w", name, err)
	}
	return u.UUID, errors.Capture(err)
}

func encodeUnitPasswordHashes(results []unitPasswordHashes) agentpassword.UnitPasswordHashes {
	ret := make(agentpassword.UnitPasswordHashes)
	for _, r := range results {
		ret[r.UnitName] = r.PasswordHash
	}
	return ret
}

func encodeMachinePasswordHashes(results []entityNamePasswordHashes) agentpassword.MachinePasswordHashes {
	ret := make(agentpassword.MachinePasswordHashes)
	for _, r := range results {
		ret[machine.Name(r.Name)] = r.PasswordHash
	}
	return ret
}
