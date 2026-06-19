// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// These methods are thin controller-scoped pass-throughs onto the v8 import
// claim and migration-specific companion tables. The v8 import driver
// (internal/migration.ModelImporter.ImportModelV2) calls them directly,
// alongside the per-domain services that own the actual controller-data
// writes (permissions, users, credential, authorized keys, secret backend,
// leadership, image metadata).

// BeginImport claims modelUUID for a new v8 import by inserting the durable
// model_migration_import row (phase=importing) as the first target-side
// write. sourceMigrationUUID is recorded for diagnostics only and must be
// non-empty.
//
// If a claim already exists, it returns an error wrapping
// [coreerrors.AlreadyExists] whose message reflects the existing claim's
// phase: cleanup-in-progress wording when phase=aborting, activation-in-
// progress wording when phase=activating, or a plain occupied-model message
// otherwise. The v8 facade maps any [coreerrors.AlreadyExists] error to
// params.CodeAlreadyExists.
func (s *Service) BeginImport(ctx context.Context, modelUUID, sourceMigrationUUID string) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	claimUUID, err := s.controllerState.BeginImport(ctx, modelUUID, sourceMigrationUUID)
	if err == nil {
		return claimUUID, nil
	}
	if !errors.Is(err, modelmigrationerrors.ErrImportClaimExists) {
		return "", errors.Capture(err)
	}

	claim, claimErr := s.controllerState.GetImportClaim(ctx, modelUUID)
	if claimErr != nil {
		// The claim that caused the conflict is gone again (e.g. a racing
		// Activate finalised between our failed insert and this read). The
		// caller still needs a coded AlreadyExists to retry against.
		return "", errors.Errorf("model import for %s: %w", modelUUID, coreerrors.AlreadyExists)
	}

	return "", modelmigration.ImportClaimConflictError(modelUUID, claim.Phase)
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
func (s *Service) AssertImporting(ctx context.Context, modelUUID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.AssertImporting(ctx, modelUUID)
}

// ImportOfferPermissions records the offer UUIDs granted permission during
// this import claim into model_migration_import_offer, atomically with an
// importing-phase assertion for modelUUID. AbortImport reads this table to
// delete the corresponding offer-permission rows without a cross-DB query to
// the model DB. The caller is responsible for writing the offer permission
// rows themselves via the access domain.
func (s *Service) ImportOfferPermissions(
	ctx context.Context, modelUUID, claimUUID string, offerUUIDs []string,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.ImportOfferPermissions(ctx, modelUUID, claimUUID, offerUUIDs)
}

// EnsureExternalControllerMatchesOrInsert compares-or-inserts a single
// third-party controller's connection details (alias, CA cert, addresses).
// It fails with [modelmigrationerrors.ErrExternalControllerMismatch] rather
// than overwriting live CMR connection data that other models may share.
func (s *Service) EnsureExternalControllerMatchesOrInsert(
	ctx context.Context, ref coremodelmigration.ExternalController,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.EnsureExternalControllerMatchesOrInsert(ctx, ref)
}

// ImportExternalControllers applies the third-party external controller
// references from a v8 import envelope to the target controller, atomically
// with an importing-phase assertion for modelUUID, and records the durable
// (offerer_model_uuid, controller_uuid) handoff that Activate reads to
// reconcile offerer-controller mappings even after a controller restart.
func (s *Service) ImportExternalControllers(
	ctx context.Context, modelUUID, claimUUID string, refs []coremodelmigration.ExternalController,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.controllerState.ImportExternalControllers(ctx, modelUUID, claimUUID, refs)
}
