// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// credentialValidator adapts the credential domain's validator to the model
// migration VALIDATION phase. It gathers the validation artefacts (model cloud
// identity, configuration and machine instances) through the migration state,
// so no other domain service is involved.
type credentialValidator struct {
	controllerState ControllerState
	modelState      ModelState
	validator       credentialservice.CredentialValidator
}

// NewCredentialValidator returns the production credential validator used by
// [Service.CheckMachines].
func NewCredentialValidator(
	controllerState ControllerState,
	modelState ModelState,
) CredentialValidator {
	return credentialValidator{
		controllerState: controllerState,
		modelState:      modelState,
		validator:       credentialservice.NewCredentialValidator(),
	}
}

// Validate checks whether the model's credential can access its cloud on this
// controller.
func (v credentialValidator) Validate(ctx context.Context, credential modelmigration.ModelCloudCredential) error {
	validationContext, err := v.validationContext(ctx)
	if err != nil {
		return errors.Errorf("getting credential validation context: %w", err)
	}

	cloudCredential := cloud.NewNamedCredential(
		credential.Name,
		cloud.AuthType(credential.AuthType),
		credential.Attributes,
		credential.Revoked,
	)
	cloudCredential.Invalid = credential.Invalid
	cloudCredential.InvalidReason = credential.InvalidReason

	owner, err := user.NewName(credential.Owner)
	if err != nil {
		return errors.Errorf("parsing credential owner %q: %w", credential.Owner, err)
	}
	key := corecredential.Key{
		Cloud: credential.Cloud,
		Owner: owner,
		Name:  credential.Name,
	}
	machineErrors, err := v.validator.Validate(
		ctx,
		validationContext,
		key,
		&cloudCredential,
		true,
	)
	if err != nil {
		return errors.Errorf("validating model credential: %w", err)
	}
	if len(machineErrors) > 0 {
		return errors.Errorf("model credential validation failed: %v", machineErrors)
	}
	return nil
}

// validationContext assembles the artefacts the credential domain validator
// needs for the imported model from the migration state.
func (v credentialValidator) validationContext(ctx context.Context) (credentialservice.CredentialValidationContext, error) {
	cloudName, region, err := v.modelState.GetModelCloudInfo(ctx)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model cloud info: %w", err)
	}
	cld, err := v.controllerState.GetCloud(ctx, cloudName)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model cloud: %w", err)
	}
	modelType, err := v.modelState.GetModelType(ctx)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model type: %w", err)
	}
	controllerUUID, err := v.modelState.GetControllerUUID(ctx)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting controller uuid: %w", err)
	}
	modelConfig, err := v.modelConfig(ctx)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model config: %w", err)
	}

	return credentialservice.CredentialValidationContext{
		ControllerUUID: controllerUUID,
		Config:         modelConfig,
		MachineService: machineService{modelState: v.modelState},
		ModelType:      coremodel.ModelType(modelType),
		Cloud:          cld,
		Region:         region,
	}, nil
}

// modelConfig returns the model's configuration, ready to open the provider
// with.
func (v credentialValidator) modelConfig(ctx context.Context) (*config.Config, error) {
	attrs, err := v.modelState.GetModelConfig(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	anyAttrs := make(map[string]any, len(attrs))
	for k, val := range attrs {
		anyAttrs[k] = val
	}
	return config.New(config.NoDefaults, anyAttrs)
}

// machineService adapts the migration model state to the narrow machine
// interface expected by the credential domain validator.
type machineService struct {
	modelState ModelState
}

// GetAllProvisionedMachineInstanceID returns all provisioned machine instance
// IDs in the model, keyed by machine name.
func (s machineService) GetAllProvisionedMachineInstanceID(ctx context.Context) (map[machine.Name]instance.Id, error) {
	instanceToMachine, err := s.modelState.GetMachineInstanceIDs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[machine.Name]instance.Id, len(instanceToMachine))
	for instanceID, machineName := range instanceToMachine {
		result[machine.Name(machineName)] = instance.Id(instanceID)
	}
	return result, nil
}

// InstanceID returns the cloud specific instance id for the machine with the
// given UUID.
func (s machineService) InstanceID(ctx context.Context, mUUID machine.UUID) (string, error) {
	return s.modelState.GetMachineInstanceID(ctx, mUUID.String())
}
