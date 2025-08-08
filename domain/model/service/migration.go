// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelDeleter is an interface for deleting models.
type ModelDeleter interface {
	// DeleteDB is responsible for removing a model from Juju and all of it's
	// associated metadata.
	DeleteDB(string) error
}

// MigrationService defines a service for interacting with the underlying state based
// information of a model.
type MigrationService struct {
	st           State
	modelDeleter ModelDeleter
	logger       logger.Logger
}

// NewMigrationService returns a new MigrationService for interacting with a models state.
func NewMigrationService(
	st State,
	modelDeleter ModelDeleter,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:           st,
		modelDeleter: modelDeleter,
		logger:       logger,
	}
}

// ImportModel is responsible for importing an existing model into this Juju
// controller. The caller must explicitly specify the agent version that is in
// use for the imported model.
//
// Models created by this function must be activated using the returned
// ModelActivator.
//
// The following error types can be expected to be returned:
// - [modelerrors.AlreadyExists]: When the model uuid is already in use or a
// model with the same name and owner already exists.
// - [errors.NotFound]: When the cloud, cloud region, or credential do not
// exist.
// - [github.com/juju/juju/domain/access/errors.NotFound]: When the owner of the
// model can not be found.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
func (s *MigrationService) ImportModel(
	ctx context.Context,
	args model.ModelImportArgs,
) (func(context.Context) error, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := args.Validate(); err != nil {
		return nil, errors.Errorf(
			"cannot validate model import args: %w", err,
		)
	}

	return createModel(ctx, s.st, args.UUID, args.GlobalModelCreationArgs)
}

// DeleteModel is responsible for removing a model from Juju and all of it's
// associated metadata.
// - errors.NotValid: When the model uuid is not valid.
// - modelerrors.NotFound: When the model does not exist.
func (s *MigrationService) DeleteModel(
	ctx context.Context,
	uuid coremodel.UUID,
	opts ...model.DeleteModelOption,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	options := model.DefaultDeleteModelOptions()
	for _, fn := range opts {
		fn(options)
	}

	if err := uuid.Validate(); err != nil {
		return errors.Errorf("delete model, uuid: %w", err)
	}

	// Delete common items from the model. This helps to ensure that the
	// model is cleaned up correctly.
	if err := s.st.Delete(ctx, uuid); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("delete model: %w", err)
	}

	// If the db should not be deleted then we can return early.
	if !options.DeleteDB() {
		s.logger.Infof(ctx, "skipping model deletion, model database will still be present")
		return nil
	}

	// Delete the db completely from the system. Currently, this will remove
	// the db from the dbaccessor, but it will not drop the db (currently not
	// supported in dqlite). For now we do a best effort to remove all items
	// with in the db.
	if err := s.modelDeleter.DeleteDB(uuid.String()); err != nil {
		return errors.Errorf("delete model: %w", err)
	}

	return nil
}
