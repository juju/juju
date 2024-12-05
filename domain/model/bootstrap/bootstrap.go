// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/uuid"
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
// - [usererrors.NotFound] When the model owner does not exist.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
//
// CreateModel expects the caller to generate their own model id and pass it to
// this function. In an ideal world we want to have this stopped and make this
// function generate a new id and return the value. This can only be achieved
// once we have the Juju client stop generating id's for controller models in
// the bootstrap process.
func CreateModel(
	modelID coremodel.UUID,
	args model.ModelCreationArgs,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return errors.Errorf("cannot create model when validating args: %w", err)
		}

		if err := modelID.Validate(); err != nil {
			return errors.Errorf(
				"cannot create model %q when validating id: %w", args.Name, err)

		}

		agentVersion := args.AgentVersion
		if args.AgentVersion == version.Zero {
			agentVersion = jujuversion.Current
		}

		if agentVersion.Major != jujuversion.Current.Major || agentVersion.Minor != jujuversion.Current.Minor {
			return errors.Errorf("%w %q during bootstrap", modelerrors.AgentVersionNotSupported, agentVersion)
		}
		args.AgentVersion = agentVersion

		activator := state.GetActivator()
		return controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			modelTypeState := modelTypeStateFunc(
				func(ctx context.Context, cloudName string) (string, error) {
					return state.CloudType()(ctx, preparer{}, tx, cloudName)
				})
			modelType, err := service.ModelTypeForCloud(ctx, modelTypeState, args.Cloud)
			if err != nil {
				return errors.Errorf("determining cloud type for model %q: %w", args.Name, err)
			}

			if args.SecretBackend == "" && modelType == coremodel.CAAS {
				args.SecretBackend = kubernetessecrets.BackendName
			} else if args.SecretBackend == "" && modelType == coremodel.IAAS {
				args.SecretBackend = jujusecrets.BackendName
			} else if args.SecretBackend == "" {
				return errors.Errorf(
					"%w for model type %q when creating model with name %q",
					secretbackenderrors.NotFound,
					modelType,
					args.Name)

			}

			if err := state.Create(ctx, preparer{}, tx, modelID, modelType, args); err != nil {
				return errors.Errorf("create bootstrap model %q with uuid %q: %w", args.Name, modelID, err)
			}

			if err := activator(ctx, preparer{}, tx, modelID); err != nil {
				return errors.Errorf("activating bootstrap model %q with uuid %q: %w", args.Name, modelID, err)
			}
			return nil
		})
	}
}

// CreateReadOnlyModel creates a new model within the model database with all of
// its associated metadata. The data will be read-only and cannot be modified
// once created.
func CreateReadOnlyModel(
	id coremodel.UUID,
	controllerUUID uuid.UUID,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controllerDB, modelDB database.TxnRunner) error {
		if err := id.Validate(); err != nil {
			return errors.Errorf("creating read only model, id %q: %w", id, err)
		}

		var m coremodel.Model
		err := controllerDB.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			m, err = state.GetModel(ctx, tx, id)
			return err
		})
		if err != nil {
			return errors.Errorf("getting model for id %q: %w", id, err)
		}

		args := model.ReadOnlyModelCreationArgs{
			UUID:              m.UUID,
			AgentVersion:      m.AgentVersion,
			ControllerUUID:    controllerUUID,
			Name:              m.Name,
			Type:              m.ModelType,
			Cloud:             m.Cloud,
			CloudRegion:       m.CloudRegion,
			CredentialOwner:   m.Credential.Owner,
			CredentialName:    m.Credential.Name,
			IsControllerModel: true,
		}

		return modelDB.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return state.CreateReadOnlyModel(ctx, args, preparer{}, tx)
		})
	}
}

type preparer struct{}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
