// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/service"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/errors"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// State is the combined state required by the migration service.
type State interface {
	service.CreateModelState

	// Delete removes the model row and all model-scoped controller rows
	// (model_namespace, model_secret_backend, secret_backend_reference,
	// model_authorized_keys, permission, model_last_login) for the given model
	// UUID. Returns [modelerrors.NotFound] when the model does not exist.
	Delete(ctx context.Context, uuid coremodel.UUID) error
}

// MigrationService defines a service for interacting with the underlying state based
// information of a model.
type MigrationService struct {
	st     State
	logger logger.Logger
}

// NewMigrationService returns a new MigrationService for interacting with a models state.
func NewMigrationService(
	st State,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		st:     st,
		logger: logger,
	}
}

// ImportModelLegacy is responsible for importing an existing model into this
// Juju controller by creating the model record in the controller database and
// marking it as importing through the legacy import path.
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
func (s *MigrationService) ImportModelLegacy(
	ctx context.Context,
	args model.ModelImportArgs,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	args, modelType, err := s.prepareModelImport(ctx, args)
	if err != nil {
		return err
	}

	return s.st.ImportModel(ctx, args.UUID, modelType, args.GlobalModelCreationArgs)
}

// ImportModel bootstraps the target-local model identity for a v8 migration
// import: the controller-database model row, model_namespace, admin
// permissions and secret backend. Unlike ImportModelLegacy, it does not insert
// a model_migration_import row -- the v8 import claim is the durable lock of
// record and is owned by the modelmigration domain (Service.BeginImport),
// which must already hold the claim for args.UUID by the time this is called.
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
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	args, modelType, err := s.prepareModelImport(ctx, args)
	if err != nil {
		return err
	}

	return s.st.Create(ctx, args.UUID, modelType, args.GlobalModelCreationArgs)
}

// prepareModelImport validates the import args, resolves the model type from
// the target cloud, and infers a default secret backend when none was
// supplied. It is shared by ImportModelLegacy and ImportModel, which differ
// only in whether the bootstrap also claims a model_migration_import row.
func (s *MigrationService) prepareModelImport(
	ctx context.Context, args model.ModelImportArgs,
) (model.ModelImportArgs, coremodel.ModelType, error) {
	if err := args.Validate(); err != nil {
		return args, "", errors.Errorf(
			"cannot validate model import args: %w", err,
		)
	}

	modelType, err := service.ModelTypeForCloud(ctx, s.st, args.Cloud)
	if err != nil {
		return args, "", errors.Errorf(
			"determining model type when importing model %q: %w",
			args.Name, err,
		)
	}

	if args.SecretBackend == "" {
		switch modelType {
		case coremodel.CAAS:
			args.SecretBackend = kubernetessecrets.BackendName
		case coremodel.IAAS:
			args.SecretBackend = jujusecrets.BackendName
		default:
			return args, "", errors.Errorf(
				"%w for model type %q when creating model with name %q",
				secretbackenderrors.NotFound,
				modelType,
				args.Name,
			)
		}
	}

	return args, modelType, nil
}

// DeleteImportedModel deletes the controller-database model row and all
// model-scoped controller rows for an import that is being aborted. It is
// idempotent: if the model does not exist, it returns nil.
func (s *MigrationService) DeleteImportedModel(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("invalid model UUID %q: %w", modelUUID, err)
	}
	err := s.st.Delete(ctx, modelUUID)
	if errors.Is(err, modelerrors.NotFound) {
		return nil
	}
	return errors.Capture(err)
}

// ActivateModel marks the model as active after a successful import or
// migration.
func (s *MigrationService) ActivateModel(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("invalid model UUID %q: %w", modelUUID, err)
	}
	return s.st.Activate(ctx, modelUUID)
}
