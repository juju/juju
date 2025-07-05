// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentpassword"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelState defines the access mechanism for interacting with passwords in the
// context of the model database.
type ModelState struct {
	*domain.StateBase
}

// NewModelState constructs a new state for interacting with the underlying passwords
// of a model.
func NewModelState(factory database.TxnRunnerFactory) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetUnitPasswordHash sets the password hash for the given unit.
func (s *ModelState) SetUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash agentpassword.PasswordHash) error {
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
func (s *ModelState) MatchesUnitPasswordHash(ctx context.Context, unitUUID unit.UUID, passwordHash agentpassword.PasswordHash) (bool, error) {
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
			return errors.Errorf("checking password hash: %w", err)
		}
		count = args.Count
		return nil
	})
	return count > 0, errors.Capture(err)
}

// GetAllUnitPasswordHashes returns a map of unit names to password hashes.
func (s *ModelState) GetAllUnitPasswordHashes(ctx context.Context) (agentpassword.UnitPasswordHashes, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &unitPasswordHashes.* FROM v_unit_password_hash`
	stmt, err := s.Prepare(query, unitPasswordHashes{})
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
func (s *ModelState) GetUnitUUID(ctx context.Context, unitName unit.Name) (unit.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var unitUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitUUID, err = s.getUnitUUID(ctx, tx, unitName.String())
		return errors.Capture(err)
	})
	return unit.UUID(unitUUID), errors.Capture(err)
}

func (s *ModelState) getUnitUUID(ctx context.Context, tx *sqlair.TX, name string) (string, error) {
	u := entityName{Name: name}

	stmt, err := s.Prepare(`
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
func (s *ModelState) SetMachinePasswordHash(ctx context.Context, machineUUID machine.UUID, passwordHash agentpassword.PasswordHash) error {
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
		if affected, err := result.RowsAffected(); err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		} else if affected == 0 {
			return machineerrors.MachineNotFound
		}
		return nil
	})
	return errors.Capture(err)
}

// MatchesMachinePasswordHashWithNonce checks if the password with a nonce is
// valid or not against the password hash stored in the database. The machine
// must be provisioned for the password to match. It returns an error if the
// machine is not provisioned.
func (s *ModelState) MatchesMachinePasswordHashWithNonce(
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

	var result machinePassword
	passwordQuery := `
SELECT
    COUNT(m.uuid) AS &machinePassword.machine_count,
    mci.instance_id AS &machinePassword.instance_id
FROM      machine m
LEFT JOIN machine_cloud_instance mci ON mci.machine_uuid = m.uuid
WHERE     m.uuid = $validatePasswordHashWithNonce.uuid
AND       m.password_hash = $validatePasswordHashWithNonce.password_hash
AND       m.nonce = $validatePasswordHashWithNonce.nonce;
`
	passwordStmt, err := s.Prepare(passwordQuery, args, result)
	if err != nil {
		return false, errors.Errorf("preparing statement to set password hash: %w", err)
	}

	var valid bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, passwordStmt, args).Get(&result); err != nil {
			return errors.Errorf("checking password hash: %w", err)
		}

		// We've not found any rows, so the password does not match.
		if result.MachineCount == 0 {
			return nil
		}

		// If the machine count is greater than 0, then we can assume the
		// password matches, but we also need to check the instance count.
		// The machine can only login if it has been provisioned.
		if !result.InstanceID.Valid || result.InstanceID.V == "" {
			return machineerrors.NotProvisioned
		}

		valid = true

		return nil
	})
	return valid, errors.Capture(err)
}

// GetAllMachinePasswordHashes returns a map of machine names to password hashes.
func (s *ModelState) GetAllMachinePasswordHashes(ctx context.Context) (agentpassword.MachinePasswordHashes, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT &entityNamePasswordHashes.* FROM machine`
	stmt, err := s.Prepare(query, entityNamePasswordHashes{})
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
func (s *ModelState) GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var machineUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		machineUUID, err = s.getMachineUUID(ctx, tx, machineName.String())
		return errors.Capture(err)
	})
	return machine.UUID(machineUUID), errors.Capture(err)
}

func (s *ModelState) getMachineUUID(ctx context.Context, tx *sqlair.TX, name string) (string, error) {
	u := entityName{Name: name}

	stmt, err := s.Prepare(`
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

// MatchesModelPasswordHash checks if the password is valid or not against the
// password hash stored for the model's agent.
func (s *ModelState) MatchesModelPasswordHash(
	ctx context.Context, passwordHash agentpassword.PasswordHash,
) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	args := validateModelPasswordHash{PasswordHash: passwordHash}
	query := `
SELECT COUNT(*) AS &validateModelPasswordHash.count
FROM   model_agent
WHERE  password_hash = $validateModelPasswordHash.password_hash;
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return false, errors.Capture(err)
	}

	var count int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&args); err != nil {
			return err
		}
		count = args.Count
		return nil
	})
	if err != nil {
		return false, errors.Errorf("checking model password hash: %w", err)
	}

	return count > 0, nil
}

// SetModelPasswordHash sets the password hash for the model overriding any
// previously set value.
func (s *ModelState) SetModelPasswordHash(ctx context.Context, passwordHash agentpassword.PasswordHash) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	passwordHashInput := modelPasswordHash{PasswordHash: passwordHash}
	updateModelPassword, err := s.Prepare(`
UPDATE model_agent
SET    password_hash = $modelPasswordHash.password_hash,
	   password_hash_algorithm_id = 0
`,
		passwordHashInput)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		err := tx.Query(ctx, updateModelPassword, passwordHashInput).Get(
			&outcome,
		)
		if err != nil {
			return errors.Errorf("getting result of setting model password hash: %w", err)
		}

		result := outcome.Result()
		if affected, err := result.RowsAffected(); err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		} else if affected == 0 {
			// Should never happen.
			return errors.New("no model agent information has been set")
		}

		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *ModelState) IsMachineController(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT 1 AS &count.count
FROM   v_machine_is_controller
WHERE  machine_uuid = $machineUUID.uuid
`
	queryStmt, err := s.Prepare(query, machineUUID{}, result)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mUUID, err := s.getMachineUUIDFromName(ctx, tx, mName)
		if err != nil {
			return err
		}

		if err := tx.Query(ctx, queryStmt, mUUID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			// If no rows are returned, the machine is not a controller.
			return nil
		} else if err != nil {
			return errors.Errorf("querying if machine %q is a controller: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", mName, err)
	}

	return result.Count == 1, nil
}

func (s *ModelState) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, mName machine.Name) (machineUUID, error) {
	machineNameParam := machineName{Name: mName}
	machineUUIDoutput := machineUUID{}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	queryStmt, err := s.Prepare(query, machineNameParam, machineUUIDoutput)
	if err != nil {
		return machineUUID{}, errors.Capture(err)
	}

	if err := tx.Query(ctx, queryStmt, machineNameParam).Get(&machineUUIDoutput); errors.Is(err, sqlair.ErrNoRows) {
		return machineUUID{}, errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return machineUUID{}, errors.Errorf("querying UUID for machine %q: %w", mName, err)
	}
	return machineUUIDoutput, nil
}

// SetApplicationPasswordHash sets the password hash for the given application.
func (s *ModelState) SetApplicationPasswordHash(
	ctx context.Context, appID application.ID, passwordHash agentpassword.PasswordHash,
) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	app := applicationID{
		ID: appID,
	}
	appStmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM application
WHERE uuid = $applicationID.uuid
`, app, count{})
	if err != nil {
		return errors.Capture(err)
	}

	args := entityPasswordHash{
		UUID:         appID.String(),
		PasswordHash: passwordHash,
	}

	query := `
INSERT INTO application_agent (application_uuid, password_hash, password_hash_algorithm_id)
VALUES      ($entityPasswordHash.uuid, $entityPasswordHash.password_hash, 0)
ON CONFLICT (application_uuid) DO
UPDATE SET  password_hash = $entityPasswordHash.password_hash,
            password_hash_algorithm_id = 0
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return errors.Errorf("preparing statement to set password hash: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		c := count{}
		if err := tx.Query(ctx, appStmt, app).Get(&c); err != nil {
			return errors.Errorf("querying for application %q: %w", appID, err)
		} else if c.Count == 0 {
			return errors.Errorf("application %q: %w", appID, applicationerrors.ApplicationNotFound)
		}

		if err := tx.Query(ctx, stmt, args).Run(); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		return nil
	})
	return errors.Capture(err)
}

// MatchesApplicationPasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *ModelState) MatchesApplicationPasswordHash(
	ctx context.Context, appID application.ID, passwordHash agentpassword.PasswordHash,
) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, err
	}

	args := validatePasswordHash{
		UUID:         appID.String(),
		PasswordHash: passwordHash,
	}

	query := `
SELECT COUNT(*) AS &validatePasswordHash.count FROM application_agent
WHERE  application_uuid = $validatePasswordHash.uuid
AND    password_hash = $validatePasswordHash.password_hash;
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return false, errors.Errorf("preparing statement to match password hash: %w", err)
	}

	var count int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&args); err != nil {
			return errors.Errorf("matching password hash: %w", err)
		}
		count = args.Count
		return nil
	})
	return count > 0, errors.Capture(err)
}

// GetApplicationIDByName returns the application ID for the named application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *ModelState) GetApplicationIDByName(ctx context.Context, name string) (application.ID, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	app := applicationIDAndName{Name: name}

	queryApplicationStmt, err := s.Prepare(`
SELECT uuid AS &applicationIDAndName.uuid
FROM application
WHERE name = $applicationIDAndName.name
`, app)
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w: %s", applicationerrors.ApplicationNotFound, name)
		} else if err != nil {
			return errors.Errorf("looking up UUID for application %q: %w", name, err)
		}
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}

	return app.ID, nil
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
