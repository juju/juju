// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// These methods are thin controller-scoped pass-throughs onto the v8 import
// claim and migration-specific companion tables. The v8 import driver
// (internal/migration.ModelImporter.ImportModelV2) calls them directly,
// alongside the per-domain services that own the actual controller-data
// writes (permissions, users, credential, authorized keys, secret backend,
// leadership, image metadata).

// BeginImport claims modelUUID for a new v8 import by inserting the durable
// model_migration_import row (phase=importing) as the first target-side
// write, and returns the new claim's UUID. sourceMigrationUUID is recorded
// for diagnostics only and must be non-empty.
//
// If a claim already exists, it returns an error wrapping
// [coreerrors.AlreadyExists] whose message reflects the existing claim's
// phase: cleanup-in-progress wording when phase=aborting, activation-in-
// progress wording when phase=activating, or a plain occupied-model message
// otherwise. The v8 facade maps any [coreerrors.AlreadyExists] error to
// params.CodeAlreadyExists.
func (s *Service) BeginImport(ctx context.Context, modelUUID coremodel.UUID, sourceMigrationUUID string) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return "", errors.Errorf("validating model uuid: %w", err)
	}

	claimUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating import claim uuid: %w", err)
	}

	claim, err := s.controllerState.BeginImport(ctx, modelUUID.String(), claimUUID.String(), sourceMigrationUUID)
	if err == nil {
		return claimUUID.String(), nil
	}
	if !errors.Is(err, modelmigrationerrors.ErrImportClaimExists) {
		return "", errors.Capture(err)
	}

	// The existing claim is returned alongside the conflict sentinel, so the
	// phase-specific wording is built without a second read.
	return "", modelmigration.ImportClaimConflictError(modelUUID.String(), claim.Phase)
}

// AssertImporting returns nil if a model_migration_import claim exists for
// modelUUID and its phase is 'importing'. The v8 import driver calls this
// once, after all controller-data write groups have completed, so a non-nil
// result means Abort or Activate raced ahead of the import at some point
// during it. ImportOfferPermissions and ImportExternalControllers each fold
// their own importing-phase assertion atomically into their write, so those
// two write groups specifically are guarded as they happen; the writes in
// between are not individually guarded. Closing that gap with a per-group
// atomic assertion is deferred to the Task 11 reconciler.
func (s *Service) AssertImporting(ctx context.Context, modelUUID coremodel.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.AssertImporting(ctx, modelUUID.String())
}

// ImportOfferPermissions records the offer UUIDs granted permission during
// this import claim into model_migration_import_offer, atomically with an
// importing-phase assertion for modelUUID. AbortImport reads this table to
// delete the corresponding offer-permission rows without a cross-DB query to
// the model DB. The caller is responsible for writing the offer permission
// rows themselves via the access domain.
func (s *Service) ImportOfferPermissions(
	ctx context.Context, modelUUID coremodel.UUID, claimUUID string, offerUUIDs []string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	return s.controllerState.ImportOfferPermissions(ctx, modelUUID.String(), claimUUID, offerUUIDs)
}

// EnsureExternalControllerExists compares-or-inserts a single third-party
// controller's connection details (alias, CA cert, addresses). It fails with
// [modelmigrationerrors.ErrExternalControllerMismatch] rather than
// overwriting live CMR connection data that other models may share.
func (s *Service) EnsureExternalControllerExists(
	ctx context.Context, ref coremodelmigration.ExternalController,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	stateRef, err := externalControllerForState(ref)
	if err != nil {
		return errors.Capture(err)
	}
	return s.controllerState.EnsureExternalControllerExists(ctx, stateRef)
}

// ImportExternalControllers applies the third-party external controller
// references from a v8 import envelope to the target controller, atomically
// with an importing-phase assertion for modelUUID, and records the durable
// (offerer_model_uuid, controller_uuid) handoff that Activate reads to
// reconcile offerer-controller mappings even after a controller restart.
func (s *Service) ImportExternalControllers(
	ctx context.Context, modelUUID coremodel.UUID, claimUUID string, refs []coremodelmigration.ExternalController,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("validating model uuid: %w", err)
	}

	stateRefs := make([]modelmigrationinternal.ExternalController, 0, len(refs))
	for _, ref := range refs {
		stateRef, err := externalControllerForState(ref)
		if err != nil {
			return errors.Capture(err)
		}
		stateRefs = append(stateRefs, stateRef)
	}
	return s.controllerState.ImportExternalControllers(ctx, modelUUID.String(), claimUUID, stateRefs)
}

// externalControllerForState translates a v8 envelope external controller
// reference into the state-layer representation, generating a fresh UUID for
// each address row so the state layer only persists supplied identifiers.
func externalControllerForState(
	ref coremodelmigration.ExternalController,
) (modelmigrationinternal.ExternalController, error) {
	addrs := make([]modelmigrationinternal.ExternalControllerAddress, 0, len(ref.Addresses))
	for _, addr := range ref.Addresses {
		addrUUID, err := uuid.NewUUID()
		if err != nil {
			return modelmigrationinternal.ExternalController{}, errors.Errorf(
				"generating external controller address uuid: %w", err)
		}
		addrs = append(addrs, modelmigrationinternal.ExternalControllerAddress{
			UUID:    addrUUID.String(),
			Address: addr,
		})
	}
	return modelmigrationinternal.ExternalController{
		UUID:           ref.UUID,
		Alias:          ref.Alias,
		CACert:         ref.CACert,
		Addresses:      addrs,
		ConsumedModels: ref.ConsumedModels,
	}, nil
}
