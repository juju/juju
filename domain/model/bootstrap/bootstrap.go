// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/cloud"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/model/service"
	"github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/modelagent"
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

// CreateGlobalModelRecord is responsible for making a new model with all of its
// associated metadata during the bootstrap process.
// If the GlobalModelCreationArgs does not have a credential name set then no
// cloud credential will be associated with the model.
//
// The following error types can be expected to be returned:
// - modelerrors.AlreadyExists: When the model UUID is already in use or a model
// with the same name and owner already exists.
// - errors.NotFound: When the cloud, cloud region, or credential do not exist.
// - errors.NotValid: When the model UUID is not valid.
// - [modelerrors.AgentVersionNotSupported]
// - [usererrors.NotFound] When the model owner does not exist.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
// - [modelerrors.CredentialNotValid] - when a credential has been provided that
// isn't supported by the cloud.
//
// CreateGlobalModelRecord expects the caller to generate their own model
// ID and pass it to this function. In an ideal world we want to have this
// stopped and make this function generate a new ID and return the value.
// This can only be achieved once we have the Juju client stop generating IDs
// for controller models in the bootstrap process.
func CreateGlobalModelRecord(
	modelID coremodel.UUID,
	args model.GlobalModelCreationArgs,
) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		if err := args.Validate(); err != nil {
			return errors.Errorf("cannot create model when validating args: %w", err)
		}

		if err := modelID.Validate(); err != nil {
			return errors.Errorf(
				"cannot create model %q when validating id: %w", args.Name, err,
			)
		}

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
					args.Name,
				)
			}

			if args.Credential.IsZero() {
				supports, err := state.CloudSupportsAuthType(
					ctx, preparer{}, tx, args.Cloud, cloud.EmptyAuthType,
				)
				if err != nil {
					return errors.Errorf(
						"checking if new model %q cloud %q supports empty auth type: %w",
						args.Name, args.Cloud, err,
					)
				}

				if !supports {
					return errors.Errorf(
						"new model %q cloud %q does not support empty authentication, a credential needs to be supplied",
						args.Name, args.Cloud,
					).Add(modelerrors.CredentialNotValid)
				}
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

// CreateLocalModelRecord creates a new model within the model database with all
// of its associated metadata. This variant defaults the agent stream to
// [coreagentbinary.AgentStreamReleased].
func CreateLocalModelRecord(
	id coremodel.UUID,
	controllerUUID uuid.UUID,
	agentVersion semversion.Number,
) internaldatabase.BootstrapOpt {
	return CreateLocalModelRecordWithAgentStream(
		id,
		controllerUUID,
		agentVersion,
		coreagentbinary.AgentStreamReleased,
	)
}

// CreateLocalModelRecordWithAgentStream creates a new model within the model
// database with all of its associated metadata. This variant allows the caller
// to also specify a model agent stream that the model is to use.
func CreateLocalModelRecordWithAgentStream(
	id coremodel.UUID,
	controllerUUID uuid.UUID,
	agentVersion semversion.Number,
	agentStream coreagentbinary.AgentStream,
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

		if agentStream == coreagentbinary.AgentStreamZero {
			agentStream = coreagentbinary.AgentStreamReleased
		}

		agentStreamArg, err := modelagent.AgentStreamFromCoreAgentStream(agentStream)
		if err != nil {
			return errors.Errorf(
				"converting agent stream %q to argument: %w", agentStream, err,
			)
		}

		args := model.ModelDetailArgs{
			UUID:              m.UUID,
			ControllerUUID:    controllerUUID,
			Name:              m.Name,
			OwnerName:         m.OwnerName,
			Owner:             m.Owner,
			Type:              m.ModelType,
			Cloud:             m.Cloud,
			CloudRegion:       m.CloudRegion,
			CredentialOwner:   m.Credential.Owner,
			CredentialName:    m.Credential.Name,
			IsControllerModel: true,

			// TODO (manadart 2024-01-13): Note that this comes from the arg.
			// It is not populated in the return from the controller state.
			// So that method should not return the core type.
			AgentVersion: agentVersion,
			AgentStream:  agentStreamArg,
		}

		return modelDB.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			return state.InsertModelInfo(ctx, args, preparer{}, tx)
		})
	}
}

// SetModelConstraints sets the constraints for the controller model during bootstrap.
// The following error types can be expected:
// - [networkerrors.SpaceNotFound]: when a space constraint is set but the
// space does not exist.
// - [machineerrors.InvalidContainerType]: when the container type set on the
// constraints is invalid.
// - [modelerrors.NotFound]: when no model exists to set constraints for.
func SetModelConstraints(cons coreconstraints.Value) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, modelDB database.TxnRunner) error {
		return modelDB.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			modelCons := constraints.DecodeConstraints(cons)
			return state.SetModelConstraints(ctx, preparer{}, tx, modelCons)
		})
	}
}

type preparer struct{}

func (p preparer) Prepare(query string, typeSamples ...any) (*sqlair.Statement, error) {
	return sqlair.Prepare(query, typeSamples...)
}
