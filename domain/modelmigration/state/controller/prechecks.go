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

// CheckCloudRegion reports whether the named cloud exists and, when a region
// name is supplied, whether that region is known to the cloud.
func (s *State) CheckCloudRegion(
	ctx context.Context, cloudName, regionName string,
) (cloudExists bool, regionExists bool, err error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	arg := cloudArg{CloudName: cloudName, RegionName: regionName}
	cloudStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   cloud AS c
WHERE  c.name = $cloudArg.cloud_name
LIMIT 1
`, countResult{}, arg)
	if err != nil {
		return false, false, errors.Capture(err)
	}
	regionStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   cloud_region AS cr
JOIN   cloud AS c ON cr.cloud_uuid = c.uuid
WHERE  c.name = $cloudArg.cloud_name
AND    cr.name = $cloudArg.region_name
LIMIT 1
`, countResult{}, arg)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		cloudExists = false
		regionExists = false

		var err error
		cloudExists, err = rowExists(ctx, tx, cloudStmt, arg)
		if err != nil || !cloudExists {
			return err
		}
		if regionName == "" {
			regionExists = true
			return nil
		}
		regionExists, err = rowExists(ctx, tx, regionStmt, arg)
		return err
	})
	if err != nil {
		return false, false, errors.Errorf(
			"checking cloud %q and region %q existence: %w", cloudName, regionName, err)
	}
	return cloudExists, regionExists, nil
}

// GetDisabledUsers reports the active users from names that are disabled on
// the controller. Missing and removed users are omitted.
func (s *State) GetDisabledUsers(ctx context.Context, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`
SELECT vua.name AS &nameArg.name
FROM   v_user_auth AS vua
WHERE  vua.name IN ($nameList[:])
AND    vua.removed = FALSE
AND    COALESCE(vua.disabled, FALSE) = TRUE
ORDER BY vua.name
`, nameArg{}, nameList{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rows []nameArg
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		return getAll(ctx, tx, stmt, &rows, nameList(names))
	})
	if err != nil {
		return nil, errors.Errorf("checking disabled users: %w", err)
	}

	disabled := make([]string, 0, len(rows))
	for _, row := range rows {
		disabled = append(disabled, row.Name)
	}
	return disabled, nil
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
LIMIT 1
`, result, arg)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = credentialRevoked{}
		exists = false

		err := tx.Query(ctx, stmt, arg).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
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
	stmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   secret_backend AS sb
WHERE  sb.name = $nameArg.name
LIMIT 1
`, countResult{}, arg)
	if err != nil {
		return false, errors.Capture(err)
	}

	var exists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists = false
		var err error
		exists, err = rowExists(ctx, tx, stmt, arg)
		return err
	})
	if err != nil {
		return false, errors.Errorf("checking secret backend %q existence: %w", name, err)
	}
	return exists, nil
}

// CheckImportModelCollision reports model identity collisions that would block
// importing the model on the target controller.
func (s *State) CheckImportModelCollision(
	ctx context.Context, modelUUID, name, qualifier string,
) (modelmigration.ImportModelCollision, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Capture(err)
	}

	modelArg := modelUUIDArg{ModelUUID: modelUUID}
	nameArg := modelNameQualifierArg{Name: name, Qualifier: qualifier}
	importStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model_migration_import AS mmi
WHERE  mmi.model_uuid = $modelUUIDArg.model_uuid
LIMIT 1
`, countResult{}, modelArg)
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Capture(err)
	}
	modelStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model AS m
WHERE  m.uuid = $modelUUIDArg.model_uuid
LIMIT 1
`, countResult{}, modelArg)
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Capture(err)
	}
	namespaceStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model_namespace AS mn
WHERE  mn.model_uuid = $modelUUIDArg.model_uuid
LIMIT 1
`, countResult{}, modelArg)
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Capture(err)
	}
	nameStmt, err := s.Prepare(`
SELECT 1 AS &countResult.count
FROM   model AS m
WHERE  m.name = $modelNameQualifierArg.name
AND    m.qualifier = $modelNameQualifierArg.qualifier
LIMIT 1
`, countResult{}, nameArg)
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Capture(err)
	}

	var collision modelmigration.ImportModelCollision
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		collision = modelmigration.ImportModelCollision{}

		var err error
		collision.Importing, err = rowExists(ctx, tx, importStmt, modelArg)
		if err != nil {
			return errors.Errorf("checking import for model %q: %w", modelUUID, err)
		}
		collision.ModelExists, err = rowExists(ctx, tx, modelStmt, modelArg)
		if err != nil {
			return errors.Errorf("checking model %q: %w", modelUUID, err)
		}
		collision.ModelNamespaceExists, err = rowExists(ctx, tx, namespaceStmt, modelArg)
		if err != nil {
			return errors.Errorf("checking model namespace for model %q: %w", modelUUID, err)
		}
		collision.ModelNameExists, err = rowExists(ctx, tx, nameStmt, nameArg)
		if err != nil {
			return errors.Errorf(
				"checking model name %q (qualifier %q): %w", name, qualifier, err)
		}
		return nil
	})
	if err != nil {
		return modelmigration.ImportModelCollision{}, errors.Errorf(
			"checking model import collisions for model %q: %w", modelUUID, err)
	}
	return collision, nil
}

// GetImportClaim returns the target-side import claim for the given model
// UUID, or [modelmigrationerrors.ErrImportNotFound] when no import exists.
func (s *State) GetImportClaim(ctx context.Context, modelUUID string) (modelmigration.ImportClaim, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	var claim modelmigration.ImportClaim
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		claim = modelmigration.ImportClaim{}
		claim, err = s.getImportClaim(ctx, tx, modelUUID)
		return err
	})
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}
	return claim, nil
}

// getImportClaim reads the import claim for modelUUID within tx, returning
// [modelmigrationerrors.ErrImportNotFound] when no claim exists.
func (s *State) getImportClaim(
	ctx context.Context, tx *sqlair.TX, modelUUID string,
) (modelmigration.ImportClaim, error) {
	arg := modelUUIDArg{ModelUUID: modelUUID}
	var row importClaimRow
	stmt, err := s.Prepare(`
SELECT mmi.source_migration_uuid AS &importClaimRow.source_migration_uuid,
       mmipt.type AS &importClaimRow.phase_type,
       strftime('%Y-%m-%dT%H:%M:%fZ', mmi.updated_at) AS &importClaimRow.updated_at
FROM   model_migration_import AS mmi
JOIN   model_migration_import_phase_type AS mmipt ON mmipt.id = mmi.phase_type_id
WHERE  mmi.model_uuid = $modelUUIDArg.model_uuid
LIMIT 1
`, row, arg)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, arg).Get(&row)
	if errors.Is(err, sqlair.ErrNoRows) {
		return modelmigration.ImportClaim{}, errors.Errorf(
			"model %q: %w", modelUUID, modelmigrationerrors.ErrImportNotFound)
	} else if err != nil {
		return modelmigration.ImportClaim{}, errors.Capture(err)
	}

	updatedAt, err := time.Parse(time.RFC3339, row.UpdatedAt)
	if err != nil {
		return modelmigration.ImportClaim{}, errors.Errorf(
			"parsing import updated_at for model %q: %w", modelUUID, err)
	}

	return modelmigration.ImportClaim{
		SourceMigrationUUID: row.SourceMigrationUUID,
		Phase:               modelmigration.ImportPhase(row.PhaseType),
		UpdatedAt:           updatedAt,
	}, nil
}

func rowExists(ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, args ...any) (bool, error) {
	var result countResult
	err := tx.Query(ctx, stmt, args...).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return result.Count > 0, nil
}
