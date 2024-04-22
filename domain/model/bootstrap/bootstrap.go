// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	internaldatabase "github.com/juju/juju/internal/database"
	jujuversion "github.com/juju/juju/version"
)

type modelTypeStateFunc func(context.Context, string) (string, error)

func (m modelTypeStateFunc) CloudType(c context.Context, n string) (string, error) {
	return m(c, n)
}

// CreateModel is responsible for making a new model with all of its associated
// metadata during the bootstrap process.
// If the ModelCreationArgs does not have a credential name set then no cloud
// credential will be associated with the model.
//
// The only supported agent version during bootstrap is that of the current
// controller. This will be the default if no agent version is supplied.
//
// The following error types can be expected to be returned:
// - modelerrors.AlreadyExists: When the model uuid is already in use or a model
// with the same name and owner already exists.
// - errors.NotFound: When the cloud, cloud region, or credential do not exist.
// - errors.NotValid: When the model uuid is not valid.
// - [modelerrors.AgentVersionNotSupported]
func CreateModel(
	args model.ModelCreationArgs,
) (coremodel.UUID, internaldatabase.BootstrapOpt) {
	var uuidError error
	uuid := args.UUID
	if uuid == "" {
		uuid, uuidError = coremodel.NewUUID()
	}

	if uuidError != nil {
		return coremodel.UUID(""), func(_ context.Context, _, _ database.TxnRunner) error {
			return fmt.Errorf("generating bootstrap model %q uuid: %w", args.Name, uuidError)
		}
	}

	return uuid, func(ctx context.Context, controller, model database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return fmt.Errorf("model creation args: %w", err)
		}

		agentVersion := args.AgentVersion
		if args.AgentVersion == version.Zero {
			agentVersion = jujuversion.Current
		}

		if agentVersion.Compare(jujuversion.Current) != 0 {
			return fmt.Errorf("%w %q during bootstrap", modelerrors.AgentVersionNotSupported, agentVersion)
		}
		args.AgentVersion = agentVersion

		if err := uuid.Validate(); err != nil {
			return fmt.Errorf("invalid model uuid: %w", err)
		}

		finaliser := state.GetFinaliser()
		return controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			modelTypeState := modelTypeStateFunc(
				func(ctx context.Context, cloudName string) (string, error) {
					return state.CloudType()(ctx, tx, cloudName)
				})
			modelType, err := service.ModelTypeForCloud(ctx, modelTypeState, args.Cloud)
			if err != nil {
				return fmt.Errorf("determining cloud type for model %q: %w", args.Name, err)
			}

			if err := state.Create(ctx, tx, uuid, modelType, args); err != nil {
				return fmt.Errorf("create bootstrap model %q with uuid %q: %w", args.Name, uuid, err)
			}

			if err := finaliser(ctx, tx, uuid); err != nil {
				return fmt.Errorf("finalising bootstrap model %q with uuid %q: %w", args.Name, uuid, err)
			}
			return nil
		})
	}
}

// CreateReadOnlyModel creates a new model within the model database with all of
// its associated metadata. The data will be read-only and cannot be modified
// once created.
func CreateReadOnlyModel(
	args model.ModelCreationArgs,
	controllerUUID coremodel.UUID,
	ownerName string,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return fmt.Errorf("model creation args: %w", err)
		}

		if args.UUID == "" {
			return fmt.Errorf("missing model uuid")
		}

		var modelType coremodel.ModelType
		err := controller.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			var err error
			modelType, err = state.GetModelType(ctx, tx, args.UUID)
			return err
		})
		if err != nil {
			return fmt.Errorf("getting model type: %w", err)
		}

		return model.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			return state.CreateReadOnlyModel(ctx, args.AsReadOnly(controllerUUID, modelType, ownerName), tx)
		})
	}
}
