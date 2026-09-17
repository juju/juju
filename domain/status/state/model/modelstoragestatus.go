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
// non-dead filesystem and volume is reported once. A filesystem backed by a
// volume is detachable when the backing volume is model-scoped; a volume is
// detachable when it is model-scoped (provision scope "model"). Dead storage
// entities are excluded so that the model status view stays aligned with the
// removal cascade's persistent-storage guard, which counts only non-dead
// storage (life_id < 2).
func (st *ModelState) GetModelStorageStatuses(
	ctx context.Context,
) (status.ModelStorageStatus, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return status.ModelStorageStatus{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT sf.filesystem_id AS &modelStorageStatusRow.filesystem_id,
       sf.provider_id AS &modelStorageStatusRow.filesystem_provider_id,
       sfsv.status AS &modelStorageStatusRow.filesystem_status,
       sf.provision_scope_id AS &modelStorageStatusRow.filesystem_provision_scope_id,
       sv.volume_id AS &modelStorageStatusRow.volume_id,
       sv.provider_id AS &modelStorageStatusRow.volume_provider_id,
       sv.provision_scope_id AS &modelStorageStatusRow.volume_provision_scope_id,
       svsv.status AS &modelStorageStatusRow.volume_status
FROM       storage_instance AS si
LEFT JOIN  storage_instance_filesystem AS sif ON si.uuid = sif.storage_instance_uuid
LEFT JOIN  storage_filesystem AS sf ON sif.storage_filesystem_uuid = sf.uuid
AND        sf.life_id < 2
LEFT JOIN  storage_filesystem_status AS sfs ON sfs.filesystem_uuid = sf.uuid
LEFT JOIN  storage_filesystem_status_value AS sfsv ON sfsv.id = sfs.status_id
LEFT JOIN  storage_instance_volume AS siv ON si.uuid = siv.storage_instance_uuid
LEFT JOIN  storage_volume AS sv ON siv.storage_volume_uuid = sv.uuid
AND        sv.life_id < 2
LEFT JOIN  storage_volume_status AS svs ON svs.volume_uuid = sv.uuid
LEFT JOIN  storage_volume_status_value AS svsv ON svsv.id = svs.status_id
`, modelStorageStatusRow{})
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
