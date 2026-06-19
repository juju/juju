// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// These methods implement the target-side v8 import claim, the
// importing-phase assertion used to gate controller-data write groups, and
// the migration-specific companion tables (model_migration_import_offer and
// model_migration_import_external_controller_model). Application of the
// controller-data writes themselves (permissions, users, credential,
// authorized keys, secret backend, leadership, image metadata) is owned by
// the per-domain services the v8 import driver calls directly; this package
// only owns the migration bookkeeping tables and the external-controller
// compare-or-insert that has no other domain owner.

// BeginImport inserts a new durable model_migration_import claim
// (phase=importing) for modelUUID as the first target-side write of a v8
// import, and returns the claim's UUID. sourceMigrationUUID is recorded for
// diagnostics only and must be non-empty.
//
// If a claim already exists for modelUUID,
// [modelmigrationerrors.ErrImportClaimExists] is returned. The caller should
// then call [State.GetImportClaim] to learn the existing claim's phase and
// report the correct AlreadyExists wording (a duplicate importing claim,
// cleanup in progress, or activation in progress).
func (s *State) BeginImport(ctx context.Context, modelUUID, sourceMigrationUUID string) (string, error) {
	if sourceMigrationUUID == "" {
		return "", errors.Errorf("empty source migration uuid for model %q", modelUUID)
	}

	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	claimUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating import claim uuid: %w", err)
	}

	claim := importClaimArg{
		UUID:                claimUUID.String(),
		ModelUUID:           modelUUID,
		SourceMigrationUUID: sourceMigrationUUID,
	}
	stmt, err := s.Prepare(`
INSERT INTO model_migration_import (uuid, model_uuid, source_migration_uuid)
VALUES ($importClaimArg.uuid, $importClaimArg.model_uuid, $importClaimArg.source_migration_uuid)
`, claim)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, claim).Run(); err != nil {
			if database.IsErrConstraintUnique(err) {
				return errors.Errorf(
					"model %q: %w", modelUUID, modelmigrationerrors.ErrImportClaimExists)
			}
			return errors.Errorf("inserting import claim for model %q: %w", modelUUID, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return claim.UUID, nil
}

// AssertImporting returns nil if a model_migration_import claim exists for
// modelUUID and its phase is 'importing'. It returns
// [modelmigrationerrors.ErrImportNotFound] if no claim exists, or
// [modelmigrationerrors.ErrImportNotImporting] if the claim has moved past
// the importing phase (activating or aborting). The v8 import driver calls
// this immediately before each controller-data write group that is not
// already covered by an atomic write-group method in this package.
func (s *State) AssertImporting(ctx context.Context, modelUUID string) error {
	claim, err := s.GetImportClaim(ctx, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if claim.Phase != modelmigration.ImportPhaseImporting {
		return errors.Errorf(
			"model %q import claim is %q: %w", modelUUID, claim.Phase,
			modelmigrationerrors.ErrImportNotImporting)
	}
	return nil
}

// assertImportingTx is the importing-phase assertion used inside a
// write-group transaction, so the phase check and the write it gates commit
// atomically.
func (s *State) assertImportingTx(ctx context.Context, tx *sqlair.TX, modelUUID string) error {
	arg := modelUUIDArg{ModelUUID: modelUUID}
	var row importPhaseRow
	stmt, err := s.Prepare(`
SELECT mmipt.type AS &importPhaseRow.phase_type
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $modelUUIDArg.model_uuid
`, row, arg)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("model %q: %w", modelUUID, modelmigrationerrors.ErrImportNotFound)
	} else if err != nil {
		return errors.Errorf("checking import claim phase for model %q: %w", modelUUID, err)
	}
	if modelmigration.ImportPhase(row.PhaseType) != modelmigration.ImportPhaseImporting {
		return errors.Errorf(
			"model %q import claim is %q: %w", modelUUID, row.PhaseType,
			modelmigrationerrors.ErrImportNotImporting)
	}
	return nil
}

// ImportOfferPermissions records the offer UUIDs granted permission during
// this import claim into model_migration_import_offer, atomically with an
// importing-phase assertion for modelUUID. AbortImport reads this table to
// delete the corresponding offer-permission rows without a cross-DB query to
// the model DB, since the offers themselves live there. The caller is
// expected to have already written the offer permission rows themselves
// (owned by the access domain).
func (s *State) ImportOfferPermissions(
	ctx context.Context, modelUUID, claimUUID string, offerUUIDs []string,
) error {
	if len(offerUUIDs) == 0 {
		return nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := s.Prepare(`
INSERT INTO model_migration_import_offer (migration_uuid, offer_uuid)
VALUES ($importOfferArg.migration_uuid, $importOfferArg.offer_uuid)
`, importOfferArg{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.assertImportingTx(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		for _, offerUUID := range offerUUIDs {
			arg := importOfferArg{MigrationUUID: claimUUID, OfferUUID: offerUUID}
			if err := tx.Query(ctx, insertStmt, arg).Run(); err != nil {
				return errors.Errorf("recording import offer %q: %w", offerUUID, err)
			}
		}
		return nil
	})
}

// EnsureExternalControllerMatchesOrInsert compares the given third-party
// controller's connection details (alias, CA cert, addresses) against any
// existing external_controller row with the same UUID. It inserts the
// controller and its addresses if absent, no-ops if the existing row is
// identical, and fails with
// [modelmigrationerrors.ErrExternalControllerMismatch] if they differ.
// Unlike the legacy importer's UPSERT, a v8 import must never silently
// overwrite live CMR connection data that other models may share.
func (s *State) EnsureExternalControllerMatchesOrInsert(
	ctx context.Context, ref coremodelmigration.ExternalController,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.ensureExternalControllerMatchesOrInsertTx(ctx, tx, ref)
	})
}

func (s *State) ensureExternalControllerMatchesOrInsertTx(
	ctx context.Context, tx *sqlair.TX, ref coremodelmigration.ExternalController,
) error {
	ctrlUUID := entityUUID{UUID: ref.UUID}

	selectCtrlStmt, err := s.Prepare(`
SELECT &externalControllerInfo.*
FROM   external_controller
WHERE  uuid = $entityUUID.uuid
`, ctrlUUID, externalControllerInfo{})
	if err != nil {
		return errors.Capture(err)
	}
	selectAddrsStmt, err := s.Prepare(`
SELECT &addressValue.address
FROM   external_controller_address
WHERE  controller_uuid = $entityUUID.uuid
`, ctrlUUID, addressValue{})
	if err != nil {
		return errors.Capture(err)
	}

	info := externalControllerInfo{UUID: ref.UUID, Alias: ref.Alias, CACert: ref.CACert}

	var existing externalControllerInfo
	err = tx.Query(ctx, selectCtrlStmt, ctrlUUID).Get(&existing)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q: %w", ref.UUID, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		addrs, err := newExternalControllerAddresses(ref.Addresses)
		if err != nil {
			return errors.Capture(err)
		}
		return s.insertExternalController(ctx, tx, info, addrs)
	}

	var existingAddrs []addressValue
	if err := tx.Query(ctx, selectAddrsStmt, ctrlUUID).GetAll(&existingAddrs); err != nil &&
		!errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external controller %q addresses: %w", ref.UUID, err)
	}
	if existing.Alias == info.Alias &&
		existing.CACert == info.CACert &&
		addressesMatch(existingAddrs, compareAddresses(ref.Addresses)) {
		return nil
	}
	return errors.Errorf(
		"external controller %q already registered with different address/CA: %w",
		ref.UUID, modelmigrationerrors.ErrExternalControllerMismatch)
}

// ensureExternalModelMatchesOrInsertTx compares-or-inserts an external_model
// row, the controller-DB record of which external controller hosts a given
// (third-party) model UUID. It fails with
// [modelmigrationerrors.ErrExternalControllerMismatch] if the model is
// already mapped to a different controller.
func (s *State) ensureExternalModelMatchesOrInsertTx(
	ctx context.Context, tx *sqlair.TX, modelUUID, controllerUUID string,
) error {
	arg := externalModelArg{ModelUUID: modelUUID}
	selectStmt, err := s.Prepare(`
SELECT &externalModelArg.controller_uuid
FROM   external_model
WHERE  uuid = $externalModelArg.uuid
`, arg)
	if err != nil {
		return errors.Capture(err)
	}

	var existing externalModelArg
	err = tx.Query(ctx, selectStmt, arg).Get(&existing)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("looking up external model %q: %w", modelUUID, err)
	}
	if errors.Is(err, sqlair.ErrNoRows) {
		insertStmt, err := s.Prepare(`
INSERT INTO external_model (*) VALUES ($externalModelArg.*)
`, externalModelArg{})
		if err != nil {
			return errors.Capture(err)
		}
		toInsert := externalModelArg{ModelUUID: modelUUID, ControllerUUID: controllerUUID}
		if err := tx.Query(ctx, insertStmt, toInsert).Run(); err != nil {
			return errors.Errorf("inserting external model %q: %w", modelUUID, err)
		}
		return nil
	}
	if existing.ControllerUUID == controllerUUID {
		return nil
	}
	return errors.Errorf(
		"external model %q already registered to a different controller: %w",
		modelUUID, modelmigrationerrors.ErrExternalControllerMismatch)
}

// ImportExternalControllers applies the third-party external controller
// references from a v8 import envelope to the target controller, atomically
// with an importing-phase assertion for modelUUID.
//
// For each reference it compares-or-inserts external_controller and
// external_controller_address via EnsureExternalControllerMatchesOrInsert,
// compares-or-inserts the consumed external_model rows the same way, and
// records each (offerer_model_uuid, controller_uuid) pair into
// model_migration_import_external_controller_model -- the durable handoff
// Activate reads to reconcile offerer-controller mappings even after a
// controller restart (WS9/WS4.1).
func (s *State) ImportExternalControllers(
	ctx context.Context, modelUUID, claimUUID string, refs []coremodelmigration.ExternalController,
) error {
	if len(refs) == 0 {
		return nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	insertMappingStmt, err := s.Prepare(`
INSERT INTO model_migration_import_external_controller_model
       (migration_uuid, offerer_model_uuid, controller_uuid)
VALUES ($importExternalControllerModelArg.migration_uuid,
        $importExternalControllerModelArg.offerer_model_uuid,
        $importExternalControllerModelArg.controller_uuid)
`, importExternalControllerModelArg{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := s.assertImportingTx(ctx, tx, modelUUID); err != nil {
			return errors.Capture(err)
		}
		for _, ref := range refs {
			if err := s.ensureExternalControllerMatchesOrInsertTx(ctx, tx, ref); err != nil {
				return errors.Capture(err)
			}
			for _, consumedModelUUID := range ref.ConsumedModels {
				if err := s.ensureExternalModelMatchesOrInsertTx(
					ctx, tx, consumedModelUUID, ref.UUID,
				); err != nil {
					return errors.Capture(err)
				}
				mapping := importExternalControllerModelArg{
					MigrationUUID:    claimUUID,
					OffererModelUUID: consumedModelUUID,
					ControllerUUID:   ref.UUID,
				}
				if err := tx.Query(ctx, insertMappingStmt, mapping).Run(); err != nil {
					return errors.Errorf(
						"recording import external controller model %q: %w", consumedModelUUID, err)
				}
			}
		}
		return nil
	})
}

// newExternalControllerAddresses generates a fresh UUID for each address so
// the rows can be inserted via insertExternalControllerAddresses.
func newExternalControllerAddresses(
	addrs []string,
) ([]modelmigrationinternal.ExternalControllerAddress, error) {
	out := make([]modelmigrationinternal.ExternalControllerAddress, 0, len(addrs))
	for _, addr := range addrs {
		id, err := uuid.NewUUID()
		if err != nil {
			return nil, errors.Errorf("generating external controller address uuid: %w", err)
		}
		out = append(out, modelmigrationinternal.ExternalControllerAddress{
			UUID:    id.String(),
			Address: addr,
		})
	}
	return out, nil
}

// compareAddresses adapts a plain address list to the shape addressesMatch
// expects, for comparison only (no UUID needed).
func compareAddresses(addrs []string) []modelmigrationinternal.ExternalControllerAddress {
	out := make([]modelmigrationinternal.ExternalControllerAddress, len(addrs))
	for i, addr := range addrs {
		out[i] = modelmigrationinternal.ExternalControllerAddress{Address: addr}
	}
	return out
}
