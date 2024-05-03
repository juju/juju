// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// agentVersion represents the target agent version from the model table.
type agentVersion struct {
	TargetAgentVersion string `db:"target_agent_version"`
}

// ModelState represents a type for interacting with the underlying model
// database state.
type ModelState struct {
	*domain.StateBase
}

// NewModelState returns a new State for interacting with the underlying model
// database state.
func NewModelState(
	factory database.TxnRunnerFactory,
) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
	}
}

// AgentVersion reports the currently set target agent version for the model.
// For the unlikely case that the models agent version is not set an error
// satisfying errors.NotFound will be returned. Should the agent version be
// invalid an error satisfying [errors.NotValid] will be returned.
func (s *ModelState) AgentVersion(ctx context.Context) (version.Number, error) {
	db, err := s.DB()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}

	q := `SELECT &agentVersion.target_agent_version FROM model`

	rval := agentVersion{}

	stmt, err := s.Prepare(q, rval)
	if err != nil {
		return version.Zero, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&rval)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return version.Zero, fmt.Errorf("agent version %w", errors.NotFound)
	} else if err != nil {
		return version.Zero, fmt.Errorf("retrieving current agent version: %w", domain.CoerceError(err))
	}

	v, err := version.Parse(rval.TargetAgentVersion)
	if err != nil {
		return version.Zero, fmt.Errorf(
			"parsing model agent version %q: %w%w",
			rval.TargetAgentVersion,
			err,
			errors.Hide(errors.NotValid),
		)
	}

	return v, nil
}

// Create creates a new read-only model.
func (s *ModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return errors.Trace(CreateReadOnlyModel(ctx, args, tx))
	})
}

// Delete deletes a model.
func (s *ModelState) Delete(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	modelStmt := `DELETE FROM model WHERE uuid = ?;`

	// Once we get to this point, the model is hosed. We don't expect the
	// model to be in use. The model migration will reinforce the schema once
	// the migration is tried again. Failure to do that will result in the
	// model being deleted unexpected scenarios.
	modelTriggerStmt := `DROP TRIGGER IF EXISTS trg_model_immutable_delete;`

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, modelTriggerStmt)
		if err != nil && !internaldatabase.IsErrError(err) {
			return fmt.Errorf("deleting model trigger %q: %w", uuid, err)
		}

		result, err := tx.ExecContext(ctx, modelStmt, uuid)
		if err != nil {
			return fmt.Errorf("deleting model %q: %w", uuid, err)
		}
		if affected, err := result.RowsAffected(); err != nil {
			return fmt.Errorf("deleting model %q: %w", uuid, err)
		} else if affected == 0 {
			return modelerrors.NotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.NotFound
		}
		return errors.Trace(err)
	}

	return nil
}

// Model returns a read-only model for the given uuid.
func (s *ModelState) Model(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.ReadOnlyModel{}, errors.Trace(err)
	}

	stmt := `
SELECT uuid,
       target_agent_version,
       controller_uuid,
       name, 
       type, 
       cloud, 
       cloud_region, 
       credential_owner, 
       credential_name
FROM model;
`

	var (
		rawControllerUUID string
		model             coremodel.ReadOnlyModel
		agentVersion      string
	)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, stmt)
		if err := row.Scan(
			&model.UUID,
			&agentVersion,
			&rawControllerUUID,
			&model.Name,
			&model.Type,
			&model.Cloud,
			&model.CloudRegion,
			&model.CredentialOwner,
			&model.CredentialName,
		); err != nil {
			return fmt.Errorf("scanning model: %w", err)
		}
		return row.Err()
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return coremodel.ReadOnlyModel{}, fmt.Errorf("model %w", modelerrors.NotFound)
		}
		return coremodel.ReadOnlyModel{}, errors.Trace(err)
	}

	model.AgentVersion, err = version.Parse(agentVersion)
	if err != nil {
		return coremodel.ReadOnlyModel{}, fmt.Errorf("parsing model agent version %q: %w", agentVersion, err)
	}

	model.ControllerUUID, err = uuid.UUIDFromString(rawControllerUUID)
	if err != nil {
		return coremodel.ReadOnlyModel{}, fmt.Errorf("parsing controller uuid %q: %w", rawControllerUUID, err)
	}
	return model, nil
}

// CreateReadOnlyModel is responsible for creating a new model within the model
// database.
func CreateReadOnlyModel(ctx context.Context, args model.ReadOnlyModelCreationArgs, tx *sql.Tx) error {
	stmt := `
INSERT INTO model (uuid, controller_uuid, name, type, target_agent_version, cloud, cloud_region, credential_owner, credential_name)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (uuid) DO NOTHING;
`

	// This is some defensive programming. The zero value of agent version is
	// still valid but should really be considered null for the purposes of
	// allowing the DDL to assert constraints.
	var agentVersion sql.NullString
	if args.AgentVersion != version.Zero {
		agentVersion.String = args.AgentVersion.String()
		agentVersion.Valid = true
	}

	result, err := tx.ExecContext(ctx, stmt,
		args.UUID,
		args.ControllerUUID.String(),
		args.Name,
		args.Type,
		agentVersion,
		args.Cloud,
		args.CloudRegion,
		args.CredentialOwner,
		args.CredentialName,
	)
	if err != nil {
		// If the model already exists, return an error that the model already
		// exists.
		if internaldatabase.IsErrConstraintUnique(err) {
			return fmt.Errorf("model %q already exists: %w%w", args.UUID, modelerrors.AlreadyExists, errors.Hide(err))
		}
		// If the model already exists and we try and update it, the trigger
		// should catch it and return an error.
		if internaldatabase.IsErrConstraintTrigger(err) {
			return fmt.Errorf("can not update model: %w%w", modelerrors.AlreadyExists, errors.Hide(err))
		}
		return fmt.Errorf("creating model %q: %w", args.UUID, err)
	}

	// Double check that it was actually created.
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("creating model %q: %w", args.UUID, err)
	}
	if affected != 1 {
		return modelerrors.AlreadyExists
	}
	return nil
}
