// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// These reads back the migrationtarget v8 import prechecks against the target
// controller database. They deliberately duplicate small existence/identity
// queries owned by other domains (cloud, access, credential, secretbackend,
// model) rather than calling those domain services, so the migrationtarget
// facade stays thin and the prechecks run as a single domain concern.

// CloudExists reports whether a cloud with the given name exists on the
// controller.
func (s *State) CloudExists(ctx context.Context, name string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := cloudArg{CloudName: name}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   cloud AS c
WHERE  c.name = $cloudArg.cloud_name
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf("checking cloud %q existence: %w", name, err)
	}
	return count.Count > 0, nil
}

// CloudRegionExists reports whether the named region is known to the named
// cloud on the controller.
func (s *State) CloudRegionExists(ctx context.Context, cloudName, regionName string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := cloudArg{CloudName: cloudName, RegionName: regionName}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   cloud_region AS cr
JOIN   cloud AS c ON cr.cloud_uuid = c.uuid
WHERE  c.name = $cloudArg.cloud_name
AND    cr.name = $cloudArg.region_name
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf(
			"checking region %q of cloud %q existence: %w", regionName, cloudName, err)
	}
	return count.Count > 0, nil
}

// IsUserDisabled reports whether an active (non-removed) user with the given
// name exists on the controller and, when it does, whether it is disabled. A
// user that does not exist returns exists=false.
func (s *State) IsUserDisabled(ctx context.Context, name string) (disabled bool, exists bool, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	arg := nameArg{Name: name}
	var result userDisabled
	stmt, err := s.Prepare(`
SELECT COALESCE(vua.disabled, FALSE) AS &userDisabled.disabled
FROM   v_user_auth AS vua
WHERE  vua.name = $nameArg.name
AND    vua.removed = FALSE
`, result, arg)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			exists = false
			return nil
		} else if err != nil {
			return err
		}
		exists = true
		return nil
	})
	if err != nil {
		return false, false, errors.Errorf("checking user %q: %w", name, err)
	}
	return result.Disabled, exists, nil
}

// GetCredentialRevoked reports whether a cloud credential with the given
// natural key (cloud name, owner name, credential name) exists on the
// controller and, when it does, whether it is revoked. A credential that does
// not exist returns exists=false.
func (s *State) GetCredentialRevoked(
	ctx context.Context, cloud, owner, name string,
) (revoked bool, exists bool, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	arg := credentialKeyArg{CloudName: cloud, OwnerName: owner, Name: name}
	var result credentialRevoked
	stmt, err := s.Prepare(`
SELECT COALESCE(vcc.revoked, FALSE) AS &credentialRevoked.revoked
FROM   v_cloud_credential AS vcc
WHERE  vcc.cloud_name = $credentialKeyArg.cloud_name
AND    vcc.owner_name = $credentialKeyArg.owner_name
AND    vcc.name = $credentialKeyArg.name
`, result, arg)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			exists = false
			return nil
		} else if err != nil {
			return err
		}
		exists = true
		return nil
	})
	if err != nil {
		return false, false, errors.Errorf(
			"checking credential %q/%q/%q: %w", cloud, owner, name, err)
	}
	return result.Revoked, exists, nil
}

// SecretBackendExists reports whether a secret backend with the given name
// exists on the controller.
func (s *State) SecretBackendExists(ctx context.Context, name string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := nameArg{Name: name}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   secret_backend AS sb
WHERE  sb.name = $nameArg.name
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf("checking secret backend %q existence: %w", name, err)
	}
	return count.Count > 0, nil
}

// ModelExists reports whether a model row with the given UUID exists on the
// controller.
func (s *State) ModelExists(ctx context.Context, modelUUID string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := modelUUIDArg{ModelUUID: modelUUID}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model AS m
WHERE  m.uuid = $modelUUIDArg.model_uuid
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf("checking model %q existence: %w", modelUUID, err)
	}
	return count.Count > 0, nil
}

// ModelNameInUse reports whether a model with the given name and qualifier
// already exists on the controller.
func (s *State) ModelNameInUse(ctx context.Context, name, qualifier string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := modelNameQualifierArg{Name: name, Qualifier: qualifier}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model AS m
WHERE  m.name = $modelNameQualifierArg.name
AND    m.qualifier = $modelNameQualifierArg.qualifier
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf(
			"checking model name %q (qualifier %q) existence: %w", name, qualifier, err)
	}
	return count.Count > 0, nil
}

// ModelNamespaceExists reports whether a model_namespace row exists for the
// given model UUID.
func (s *State) ModelNamespaceExists(ctx context.Context, modelUUID string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	arg := modelUUIDArg{ModelUUID: modelUUID}
	var count countResult
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &countResult.count
FROM   model_namespace AS mn
WHERE  mn.model_uuid = $modelUUIDArg.model_uuid
`, count, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, arg).Get(&count)
	})
	if err != nil {
		return false, errors.Errorf(
			"checking model namespace existence for model %q: %w", modelUUID, err)
	}
	return count.Count > 0, nil
}

// GetImportClaim returns the target-side import claim for the given model
// UUID, or [modelmigrationerrors.ErrImportNotFound] when no claim exists.
func (s *State) GetImportClaim(ctx context.Context, modelUUID string) (modelmigration.ImportClaim, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	arg := modelUUIDArg{ModelUUID: modelUUID}
	var row importClaimRow
	stmt, err := s.Prepare(`
SELECT mmi.source_migration_uuid AS &importClaimRow.source_migration_uuid,
       mmipt.type AS &importClaimRow.phase_type,
       strftime('%Y-%m-%dT%H:%M:%fZ', mmi.updated_at) AS &importClaimRow.updated_at
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $modelUUIDArg.model_uuid
`, row, arg)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).Get(&row)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"model %q: %w", modelUUID, modelmigrationerrors.ErrImportNotFound)
		}
		return err
	})
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Errorf(
			"parsing import claim updated_at for model %q: %w", modelUUID, err)
	}

	return modelmigration.ImportClaim{
		SourceMigrationUUID: row.SourceMigrationUUID,
		Phase:               modelmigration.ImportPhase(row.PhaseType),
		UpdatedAt:           updatedAt,
	}, nil
}
