// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/controller"
	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// These methods are the controller- and model-scoped pass-throughs the v8
// activation driver (internal/migration.activateModel) calls to finalise an
// imported model. They mirror the import-driver methods in import.go: input
// validation and tracing live here, the SQL lives in state.

// GetImportClaim returns the target-side import claim for modelUUID, or
// [modelmigrationerrors.ErrImportNotFound] when no claim exists.
func (s *Service) GetImportClaim(ctx context.Context, modelUUID coremodel.UUID) (modelmigration.ImportClaim, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return modelmigration.ImportClaim{}, errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.GetImportClaim(ctx, modelUUID.String())
}

// SetImportPhaseActivating transitions the model's import claim from the
// importing phase to the activating phase. It is idempotent when the claim is
// already activating.
func (s *Service) SetImportPhaseActivating(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.SetImportPhaseActivating(ctx, modelUUID.String())
}

// DeleteActivatedImport removes the model's import claim and its FK-dependent
// companion rows, asserting the claim is in the activating phase. It is
// idempotent when no claim exists.
func (s *Service) DeleteActivatedImport(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.DeleteActivatedImport(ctx, modelUUID.String())
}

// EnsureSourceControllerExists compares-or-inserts the migration source
// controller's connection details and records the models it offers. It
// generates the per-address row UUIDs before handing the details to state, and
// fails with [modelmigrationerrors.ErrExternalControllerMismatch] on a mismatch
// rather than overwriting live CMR connection data.
func (s *Service) EnsureSourceControllerExists(
	ctx context.Context,
	controllerUUID controller.UUID,
	alias, caCert string,
	addrs, consumedModels []string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := controllerUUID.Validate(); err != nil {
		return errors.Errorf("validating source controller uuid: %w", err)
	}

	addrUUIDs := make([]string, len(addrs))
	for i := range addrs {
		u, err := uuid.NewUUID()
		if err != nil {
			return errors.Errorf("generating source controller address uuid: %w", err)
		}
		addrUUIDs[i] = u.String()
	}

	return s.controllerState.EnsureSourceControllerExists(
		ctx, controllerUUID.String(), alias, caCert, addrs, addrUUIDs, consumedModels,
	)
}

// ExternalControllerModelsForImport returns the third-party offerer-model to
// controller mappings recorded for the model's import claim. Returns an empty
// slice when no mappings exist or the model has no claim.
func (s *Service) ExternalControllerModelsForImport(
	ctx context.Context, modelUUID coremodel.UUID,
) ([]coremodelmigration.OffererModel, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.ExternalControllerModelsForImport(ctx, modelUUID.String())
}

// GetControllerTargetVersion returns the controller's target agent version.
func (s *Service) GetControllerTargetVersion(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.GetControllerTargetVersion(ctx)
}

// DeleteModelImportingStatus clears the model-database import gate, making the
// model visible once activation completes.
func (s *Service) DeleteModelImportingStatus(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.DeleteModelImportingStatus(ctx)
}

// GetModelTargetAgentVersion returns the target agent version currently set for
// the model.
func (s *Service) GetModelTargetAgentVersion(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.GetModelTargetAgentVersion(ctx)
}

// SetModelTargetAgentVersion sets the model's target agent version, asserting
// that the current version matches preCondition.
func (s *Service) SetModelTargetAgentVersion(ctx context.Context, preCondition, toVersion string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.SetModelTargetAgentVersion(ctx, preCondition, toVersion)
}
