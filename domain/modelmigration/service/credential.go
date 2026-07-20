// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	credentialservice "github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
)

// CredentialValidationContextGetter returns the artefacts required to validate
// a model's credential for the given model.
type CredentialValidationContextGetter func(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (credentialservice.CredentialValidationContext, error)

// credentialValidator adapts the credential domain's validator to the model
// migration VALIDATION phase.
type credentialValidator struct {
	getter    CredentialValidationContextGetter
	validator credentialservice.CredentialValidator
}

// NewCredentialValidator returns the production credential validator used by
// [Service.CheckMachines].
func NewCredentialValidator(
	getter CredentialValidationContextGetter,
) CredentialValidator {
	return credentialValidator{
		getter:    getter,
		validator: credentialservice.NewCredentialValidator(),
	}
}

// Validate checks whether the model's credential can access its cloud on this
// controller.
func (v credentialValidator) Validate(ctx context.Context, modelUUID string, credential modelmigration.ModelCloudCredential) error {
	typedUUID := coremodel.UUID(modelUUID)
	if err := typedUUID.Validate(); err != nil {
		return errors.Errorf("invalid model uuid: %w", err)
	}

	validationContext, err := v.getter(ctx, typedUUID)
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
