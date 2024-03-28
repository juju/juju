// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internaldatabase "github.com/juju/juju/internal/database"
)

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

// Create creates a new model with all of its associated metadata.
func (s *ModelState) Create(ctx context.Context, args model.ReadOnlyModelCreationArgs) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return errors.Trace(CreateReadOnlyModel(ctx, args, tx))
	})
}

// Model returns a read-only model for the given uuid.
func (s *ModelState) Model(ctx context.Context) (coremodel.ReadOnlyModel, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.ReadOnlyModel{}, errors.Trace(err)
	}

	stmt := `
SELECT uuid,
       controller_uuid,
       name, 
       type, 
       cloud, 
       cloud_region, 
       credential_owner, 
       credential_name
FROM model;
`

	var model coremodel.ReadOnlyModel
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, stmt)
		if err := row.Scan(
			&model.UUID,
			&model.ControllerUUID,
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
	return model, nil
}

// CreateReadOnlyModel is responsible for creating a new model within the model
// database.
func CreateReadOnlyModel(ctx context.Context, args model.ReadOnlyModelCreationArgs, tx *sql.Tx) error {
	stmt := `
INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_region, credential_owner, credential_name)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (uuid) DO NOTHING;
`
	result, err := tx.ExecContext(ctx, stmt,
		args.UUID,
		args.ControllerUUID,
		args.Name,
		args.Type,
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
