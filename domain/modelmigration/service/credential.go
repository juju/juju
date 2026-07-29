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
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	domaincloud "github.com/juju/juju/domain/cloud"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/errors"
)

// checkModelCredential validates the credential assigned to the model against
// the target controller. It runs for both IAAS and CAAS models so an imported
// model with unusable credentials fails VALIDATION before activation.
func (s *Service) checkModelCredential(ctx context.Context, info modelmigration.CredentialValidationInfo) error {
	credential, err := s.controllerState.GetModelCloudCredential(ctx, s.modelUUID)
	if err != nil {
		return errors.Errorf("getting model cloud credential: %w", err)
	}
	// A model can legitimately have no credential (cloud_credential_uuid is
	// nullable: clouds with an "empty" auth type, such as local LXD, need
	// none), in which case there is nothing to validate.
	if credential == nil {
		return nil
	}
	if credential.Revoked {
		return errors.Errorf(
			"model cloud credential %q on cloud %q for owner %q is revoked",
			credential.Name, credential.Cloud, credential.Owner,
		)
	}
	// A credential Juju has already marked invalid is rejected here rather than
	// left to the provider: some providers still open successfully with one, so
	// relying on the open alone would let an unusable credential through. 3.6
	// refused the same way, in its validator.
	if credential.Invalid {
		return errors.Errorf(
			"model cloud credential %q on cloud %q for owner %q is not valid: %s",
			credential.Name, credential.Cloud, credential.Owner, credential.InvalidReason,
		)
	}
	return s.credentialValidator.Validate(ctx, info, *credential)
}

// credentialValidator adapts the credential domain's validator to the model
// migration VALIDATION phase. It gathers the validation artefacts (model cloud
// identity, configuration and machine instances) through the migration state,
// so no other domain service is involved.
type credentialValidator struct {
	controllerState ControllerState
	modelState      ModelState
	validator       credentialservice.CredentialValidator
}

// NewCredentialValidator returns a credential validator for [Service.CheckMachines]
// that defers the provider-level check to the supplied credential domain
// validator.
func NewCredentialValidator(
	controllerState ControllerState,
	modelState ModelState,
	validator credentialservice.CredentialValidator,
) CredentialValidator {
	return credentialValidator{
		controllerState: controllerState,
		modelState:      modelState,
		validator:       validator,
	}
}

// Validate checks whether the model's credential can access its cloud on this
// controller.
func (v credentialValidator) Validate(
	ctx context.Context,
	info modelmigration.CredentialValidationInfo,
	credential modelmigration.ModelCloudCredential,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := credentialKey(credential)
	if err != nil {
		return errors.Capture(err)
	}

	validationContext, err := v.validationContext(ctx, info)
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

	machineErrors, err := v.validator.Validate(
		ctx,
		validationContext,
		key,
		&cloudCredential,
		checkCloudInstances(info.CloudType),
	)
	if err != nil {
		return errors.Errorf("validating model credential: %w", err)
	}
	if len(machineErrors) > 0 {
		return errors.Errorf("model credential validation failed: %v", machineErrors)
	}
	return nil
}

// credentialKey converts the model's credential into the natural key the
// credential domain identifies it by, rejecting a credential the import left
// incomplete before any provider is opened with it.
func credentialKey(credential modelmigration.ModelCloudCredential) (corecredential.Key, error) {
	owner, err := user.NewName(credential.Owner)
	if err != nil {
		return corecredential.Key{}, errors.Errorf("parsing credential owner %q: %w", credential.Owner, err)
	}
	key := corecredential.Key{
		Cloud: credential.Cloud,
		Owner: owner,
		Name:  credential.Name,
	}
	if err := key.Validate(); err != nil {
		return corecredential.Key{}, errors.Errorf("model cloud credential %w", err)
	}
	if credential.AuthType == "" {
		return corecredential.Key{}, errors.Errorf("model cloud credential %q has no auth type", credential.Name)
	}
	return key, nil
}

// checkCloudInstances reports whether the credential check should also require
// every instance the cloud reports to be backed by a model machine.
//
// It must not for an unmanaged (3.6's "manual") cloud: its machines are
// provisioned outside Juju, so the provider's instances are not Juju's to
// account for and a 1:1 mapping would never hold. 3.6 made the same exception,
// passing cloud.Type != "manual" from its CheckMachines facade.
func checkCloudInstances(cloudType string) bool {
	return cloudType != domaincloud.CloudTypeUnmanaged.String()
}

// validationContext assembles the artefacts the credential domain validator
// needs for the imported model, from the model information already read for the
// VALIDATION phase plus the target controller's definition of the model's cloud.
func (v credentialValidator) validationContext(
	ctx context.Context, info modelmigration.CredentialValidationInfo,
) (credentialservice.CredentialValidationContext, error) {
	cld, err := v.controllerState.GetCloud(ctx, info.CloudName)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model cloud: %w", err)
	}
	modelConfig, err := modelConfig(info.Config)
	if err != nil {
		return credentialservice.CredentialValidationContext{}, errors.Errorf("getting model config: %w", err)
	}

	return credentialservice.CredentialValidationContext{
		ControllerUUID: info.ControllerUUID,
		Config:         modelConfig,
		MachineService: machineService{modelState: v.modelState},
		ModelType:      coremodel.ModelType(info.ModelType),
		Cloud:          cld,
		Region:         info.CloudRegion,
	}, nil
}

// modelConfig turns the model's stored configuration into the form needed to
// open the provider with. NoDefaults is correct here: v_model_config holds the
// model's fully resolved configuration, with defaults already materialised when
// the model was created, so re-applying them would only overwrite stored values.
// This matches how the rest of the codebase reconstitutes stored model config.
func modelConfig(attrs map[string]string) (*config.Config, error) {
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
