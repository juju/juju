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
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// ModelState represents a type for interacting with the underlying model
// database state.
type ModelState struct {
	*domain.StateBase
	logger logger.Logger
}

// NewModelState returns a new State for interacting with the underlying model
// database state.
func NewModelState(
	factory database.TxnRunnerFactory,
	logger logger.Logger,
) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// Create creates a new read-only model.
func (s *ModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(CreateReadOnlyModel(ctx, args, s, tx))
	})
}

// Delete deletes a model.
func (s *ModelState) Delete(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	mUUID := dbUUID{UUID: uuid.String()}

	modelStmt, err := s.Prepare(`DELETE FROM model WHERE uuid = $dbUUID.uuid;`, mUUID)
	if err != nil {
		return errors.Annotatef(err, "preparing delete model statement")
	}

	// Once we get to this point, the model is hosed. We don't expect the
	// model to be in use. The model migration will reinforce the schema once
	// the migration is tried again. Failure to do that will result in the
	// model being deleted unexpected scenarios.
	modelTriggerStmt, err := s.Prepare(`DROP TRIGGER IF EXISTS trg_model_immutable_delete;`)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, modelTriggerStmt).Run(); errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil && !internaldatabase.IsErrError(err) {
			return fmt.Errorf("deleting model trigger %q: %w", uuid, err)
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, modelStmt, mUUID).Get(&outcome); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return modelerrors.NotFound
			}
			return fmt.Errorf("deleting model %q: %w", uuid, err)
		}
		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return fmt.Errorf("deleting model %q: %w", uuid, err)
		} else if affected == 0 {
			return modelerrors.NotFound
		}
		return nil
	})
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Model returns a read-only model information that has been set in the database.
// If no model has been set then an error satisfying [modelerrors.NotFound] is
// returned.
func (s *ModelState) Model(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.ReadOnlyModel{}, errors.Trace(err)
	}

	m := dbReadOnlyModel{}
	stmt, err := s.Prepare(`SELECT &dbReadOnlyModel.* FROM model`, m)
	if err != nil {
		return coremodel.ReadOnlyModel{}, errors.Annotatef(err, "preparing select read only model statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt).Get(&m); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("getting model read only information %w", modelerrors.NotFound)
		} else if err != nil {
			return fmt.Errorf("getting model read only information: %w", err)
		}
		return nil
	})
	if err != nil {
		return coremodel.ReadOnlyModel{}, errors.Trace(err)
	}

	model := coremodel.ReadOnlyModel{
		UUID:           coremodel.UUID(m.UUID),
		Name:           m.Name,
		Type:           coremodel.ModelType(m.Type),
		Cloud:          m.Cloud,
		CloudType:      m.CloudType,
		CloudRegion:    m.CloudRegion,
		CredentialName: m.CredentialName,
	}

	if owner := m.CredentialOwner; owner != "" {
		model.CredentialOwner, err = user.NewName(owner)
		if err != nil {
			return coremodel.ReadOnlyModel{}, errors.Trace(err)
		}
	} else {
		s.logger.Infof("model %s: cloud credential owner name is empty", model.Name)
	}

	var agentVersion string
	if m.TargetAgentVersion.Valid {
		agentVersion = m.TargetAgentVersion.String
	}

	model.AgentVersion, err = version.Parse(agentVersion)
	if err != nil {
		return coremodel.ReadOnlyModel{}, fmt.Errorf("parsing model agent version %q: %w", agentVersion, err)
	}

	model.ControllerUUID, err = uuid.UUIDFromString(m.ControllerUUID)
	if err != nil {
		return coremodel.ReadOnlyModel{}, fmt.Errorf("parsing controller uuid %q: %w", m.ControllerUUID, err)
	}
	return model, nil
}

// CreateReadOnlyModel is responsible for creating a new model within the model
// database.
func CreateReadOnlyModel(ctx context.Context, args model.ReadOnlyModelCreationArgs, preparer domain.Preparer, tx *sqlair.TX) error {
	// This is some defensive programming. The zero value of agent version is
	// still valid but should really be considered null for the purposes of
	// allowing the DDL to assert constraints.
	var agentVersion sql.NullString
	if args.AgentVersion != version.Zero {
		agentVersion.String = args.AgentVersion.String()
		agentVersion.Valid = true
	}

	m := dbReadOnlyModel{
		UUID:               args.UUID.String(),
		ControllerUUID:     args.ControllerUUID.String(),
		Name:               args.Name,
		Type:               args.Type.String(),
		TargetAgentVersion: agentVersion,
		Cloud:              args.Cloud,
		CloudType:          args.CloudType,
		CloudRegion:        args.CloudRegion,
		CredentialOwner:    args.CredentialOwner.Name(),
		CredentialName:     args.CredentialName,
	}

	stmt, err := preparer.Prepare(`
INSERT INTO model (*) VALUES ($dbReadOnlyModel.*)
ON CONFLICT (uuid) DO NOTHING;
`, dbReadOnlyModel{})
	if err != nil {
		return errors.Trace(err)
	}

	var outcome sqlair.Outcome
	if err := tx.Query(ctx, stmt, m).Get(&outcome); err != nil {
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
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return fmt.Errorf("creating model %q: %w", args.UUID, err)
	}
	if affected != 1 {
		return modelerrors.AlreadyExists
	}
	return nil
}
