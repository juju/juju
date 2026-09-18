// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see licence file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// GetModelStorageStatuses returns the filesystems and volumes of the model
// with the minimal information required by the model status payload. Each
// non-dead filesystem and volume is reported once, whether or not it is
// linked to a storage instance: the removal cascade's persistent-storage
// guard counts those rows directly from storage_filesystem/storage_volume,
// so the client view must be able to see exactly what the guard sees.
// A filesystem backed by a volume is detachable when the backing volume is
// model-scoped; a volume is detachable when it is model-scoped (provision
// scope "model"). Dead storage entities are excluded so that the model
// status view stays aligned with the guard, which counts only non-dead
// storage (life_id < 2).
func (st *ModelState) GetModelStorageStatuses(
	ctx context.Context,
) (status.ModelStorageStatus, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return status.ModelStorageStatus{}, errors.Capture(err)
	}

	// Both branches anchor directly on storage_filesystem/storage_volume so
	// that storage with no storage-instance link (an orphan) is still
	// reported, mirroring what the removal guard counts. The filesystem
	// branch only reaches through the instance link tables to discover the
	// backing volume's provision scope for the volume-first detachability
	// precedence; its volume_id stays NULL so that each volume is emitted
	// exactly once, by the volume branch.
	stmt, err := st.Prepare(`
SELECT fs_id AS &modelStorageStatusRow.filesystem_id,
       fs_provider_id AS &modelStorageStatusRow.filesystem_provider_id,
       fs_status AS &modelStorageStatusRow.filesystem_status,
       fs_scope AS &modelStorageStatusRow.filesystem_provision_scope_id,
       v_id AS &modelStorageStatusRow.volume_id,
       v_provider_id AS &modelStorageStatusRow.volume_provider_id,
       v_scope AS &modelStorageStatusRow.volume_provision_scope_id,
       v_status AS &modelStorageStatusRow.volume_status
FROM (
    SELECT sf.filesystem_id AS fs_id,
           sf.provider_id AS fs_provider_id,
           sfsv.status AS fs_status,
           sf.provision_scope_id AS fs_scope,
           CAST(NULL AS TEXT) AS v_id,
           CAST(NULL AS TEXT) AS v_provider_id,
           bsv.provision_scope_id AS v_scope,
           CAST(NULL AS TEXT) AS v_status
    FROM       storage_filesystem AS sf
    LEFT JOIN  storage_filesystem_status AS sfs ON sfs.filesystem_uuid = sf.uuid
    LEFT JOIN  storage_filesystem_status_value AS sfsv ON sfsv.id = sfs.status_id
    LEFT JOIN  storage_instance_filesystem AS sif ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN  storage_instance_volume AS siv ON siv.storage_instance_uuid = sif.storage_instance_uuid
    LEFT JOIN  storage_volume AS bsv ON bsv.uuid = siv.storage_volume_uuid
    AND        bsv.life_id < 2
    WHERE      sf.life_id < 2

    UNION ALL

    SELECT CAST(NULL AS TEXT) AS fs_id,
           CAST(NULL AS TEXT) AS fs_provider_id,
           CAST(NULL AS TEXT) AS fs_status,
           CAST(NULL AS INT) AS fs_scope,
           sv.volume_id AS v_id,
           sv.provider_id AS v_provider_id,
           sv.provision_scope_id AS v_scope,
           svsv.status AS v_status
    FROM       storage_volume AS sv
    LEFT JOIN  storage_volume_status AS svs ON svs.volume_uuid = sv.uuid
    LEFT JOIN  storage_volume_status_value AS svsv ON svsv.id = svs.status_id
    WHERE      sv.life_id < 2
)`, modelStorageStatusRow{})
	if err != nil {
		return status.ModelStorageStatus{}, errors.Capture(err)
	}

	var rows []modelStorageStatusRow
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		rows = nil
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return status.ModelStorageStatus{}, errors.Capture(err)
	}

	var ret status.ModelStorageStatus
	for _, row := range rows {
		if row.FilesystemID.Valid {
			ret.Filesystems = append(ret.Filesystems, status.ModelStorageFilesystemStatus{
				ID:         row.FilesystemID.V,
				ProviderID: row.FilesystemProviderID.V,
				Status:     row.FilesystemStatus.V,
				// Volume-first precedence: a volume-backed filesystem
				// follows the backing volume's scope.
				Detachable: detachable(row.VolumeProvisionScope, row.FilesystemProvisionScope),
			})
		}
		if row.VolumeID.Valid {
			ret.Volumes = append(ret.Volumes, status.ModelStorageVolumeStatus{
				ID:         row.VolumeID.V,
				ProviderID: row.VolumeProviderID.V,
				Status:     row.VolumeStatus.V,
				Detachable: row.VolumeProvisionScope.Valid &&
					row.VolumeProvisionScope.V == int(storage.ProvisionScopeModel),
			})
		}
	}
	return ret, nil
}

// detachable reports whether the storage entity is model-scoped. When a
// volume scope is present it takes precedence over the filesystem scope.
func detachable(volumeScope, filesystemScope sql.Null[int]) bool {
	if volumeScope.Valid {
		return volumeScope.V == int(storage.ProvisionScopeModel)
	}
	return filesystemScope.Valid && filesystemScope.V == int(storage.ProvisionScopeModel)
}
