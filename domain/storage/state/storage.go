// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

func (st *State) GetModelDetails() (domainstorage.ModelDetails, error) {
	//TODO implement me
	return domainstorage.ModelDetails{}, errors.New("not implemented")
}

func (st *State) ImportFilesystem(ctx context.Context, name storage.Name, filesystem domainstorage.FilesystemInfo) (storage.ID, error) {
	//TODO implement me
	return "", errors.New("not implemented")
}

// ListStorageInstances returns a list of storage instances in the model.
func (st *State) ListStorageInstances(ctx context.Context) ([]StorageInstanceInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	storageInstancesStmt, err := st.Prepare(`
SELECT (si.storage_id, si.storage_kind_id, si.life_id, sv.persistent) AS (&dbStorageInstanceInfo.*),
       u.name AS &dbStorageInstanceInfo.owner_unit_name
FROM   storage_instance si
LEFT JOIN storage_instance_volume siv ON si.uuid=siv.storage_instance_uuid
LEFT JOIN storage_volume sv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN storage_unit_owner suo ON si.uuid=suo.storage_instance_uuid
LEFT JOIN unit u ON suo.unit_uuid=u.uuid`, dbStorageInstanceInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	volAttsStmt, err := st.Prepare(`
SELECT DISTINCT (si.storage_id, sva.life_id, sv.hardware_id, sv.wwn) AS (&dbVolumeAttachmentInfo.*),
                (svs.status_id, svs.message, svs.updated_at) AS (&dbVolumeAttachmentInfo.*),
                u.name AS &dbVolumeAttachmentInfo.unit_name,
                m.name AS &dbVolumeAttachmentInfo.machine_name,
                bd.name AS &dbVolumeAttachmentInfo.block_device_name,
                first_value(bdld.name) OVER bdld_first AS &dbVolumeAttachmentInfo.block_device_link
FROM            storage_volume sv
LEFT JOIN       storage_volume_attachment sva ON sv.uuid=sva.storage_volume_uuid
LEFT JOIN       storage_volume_status svs ON svs.volume_uuid=sv.uuid
LEFT JOIN       storage_instance_volume siv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN       storage_instance si ON si.uuid=siv.storage_instance_uuid
LEFT JOIN       storage_attachment sa ON sa.storage_instance_uuid=si.uuid
LEFT JOIN       unit u ON u.uuid=sa.unit_uuid
LEFT JOIN       machine m ON sva.net_node_uuid=m.net_node_uuid
LEFT JOIN       block_device bd ON bd.uuid=sva.block_device_uuid
LEFT JOIN       block_device_link_device bdld ON bdld.block_device_uuid=bd.uuid
WINDOW          bdld_first AS (PARTITION BY bdld.block_device_uuid ORDER BY bdld.name)`, dbVolumeAttachmentInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	fsAttsStmt, err := st.Prepare(`
SELECT    (si.storage_id, sfa.life_id, sfa.mount_point) AS (&dbFilesystemAttachmentInfo.*),
          (sfs.status_id, sfs.message, sfs.updated_at) AS (&dbFilesystemAttachmentInfo.*),
          u.name AS &dbFilesystemAttachmentInfo.unit_name,
          m.name AS &dbFilesystemAttachmentInfo.machine_name
FROM      storage_filesystem sf
LEFT JOIN storage_filesystem_attachment sfa ON sf.uuid=sfa.storage_filesystem_uuid
LEFT JOIN storage_filesystem_status sfs ON sfs.filesystem_uuid=sf.uuid
LEFT JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid=sf.uuid
LEFT JOIN storage_instance si ON si.uuid=sif.storage_instance_uuid
LEFT JOIN storage_attachment sa ON sa.storage_instance_uuid=si.uuid
LEFT JOIN unit u ON u.uuid=sa.unit_uuid
LEFT JOIN machine m ON sfa.net_node_uuid=m.net_node_uuid`, dbFilesystemAttachmentInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		dbInstances                 []dbStorageInstanceInfo
		dbVolumeAttachmentInfos     []dbVolumeAttachmentInfo
		dbFilesystemAttachmentInfos []dbFilesystemAttachmentInfo
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, storageInstancesStmt).GetAll(&dbInstances)
		if errors.Is(err, sql.ErrNoRows) {
			// No storage instances, exit early.
			return nil
		}
		if err != nil {
			return errors.Errorf("listing storage instances: %w", err)
		}

		err = tx.Query(ctx, volAttsStmt).GetAll(&dbVolumeAttachmentInfos)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("listing volume attachments: %w", err)
		}

		err = tx.Query(ctx, fsAttsStmt).GetAll(&dbFilesystemAttachmentInfos)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("listing filesystem attachments: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	mSI := make(map[string]*StorageInstanceInfo, len(dbInstances))
	for _, dbInstance := range dbInstances {
		si := &StorageInstanceInfo{
			ID:         dbInstance.ID,
			Kind:       domainstorage.StorageKind(dbInstance.KindID),
			Life:       life.Life(dbInstance.LifeID),
			Persistent: dbInstance.Persistent,
		}
		if dbInstance.OwnerUnitName.Valid {
			u := unit.Name(dbInstance.OwnerUnitName.String)
			si.Owner = &u
		}
		mSI[dbInstance.ID] = si
	}
	for _, att := range dbVolumeAttachmentInfos {
		info, ok := mSI[att.StorageID]
		if !ok {
			// This shouldn't happen, but log and skip if it does.
			st.logger.Errorf(ctx, "found volume attachment for unknown storage instance %q", att.StorageID)
			continue
		}
		if info.VolumeInfo == nil {
			statusValue, err := status.DecodeStorageVolumeStatus(att.StatusID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			info.VolumeInfo = &VolumeInfo{
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status:  statusValue,
					Message: att.Message,
					Since:   att.UpdatedAt,
				},
			}
		}
		va := VolumeAttachmentInfo{
			AttachmentInfo: AttachmentInfo{
				Life: life.Life(att.LifeID),
			},
			HardwareID:      att.HardwareID,
			WWN:             att.WWN,
			BlockDeviceName: att.BlockDeviceName,
			BlockDeviceLink: att.BlockDeviceLink,
		}
		if att.UnitName.Valid {
			va.Unit = unit.Name(att.UnitName.String)
		} else {
			// This shouldn't happen, but log and skip if it does.
			st.logger.Errorf(ctx, "no unit name for volume attachment on storage instance %q", att.StorageID)
			continue
		}
		if att.MachineName.Valid {
			m := machine.Name(att.MachineName.String)
			va.Machine = &m
		}
		info.VolumeInfo.Attachments = append(info.VolumeInfo.Attachments, va)
	}
	for _, att := range dbFilesystemAttachmentInfos {
		info, ok := mSI[att.StorageID]
		if !ok {
			// This shouldn't happen, but log and skip if it does.
			st.logger.Errorf(ctx, "found filesystem attachment for unknown storage instance %q", att.StorageID)
			continue
		}
		if info.FilesystemInfo == nil {
			statusValue, err := status.DecodeStorageFilesystemStatus(att.StatusID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			info.FilesystemInfo = &FilesystemInfo{
				Status: status.StatusInfo[status.StorageFilesystemStatusType]{
					Status:  statusValue,
					Message: att.Message,
					Since:   att.UpdatedAt,
				},
			}
		}
		fa := FilesystemAttachmentInfo{
			AttachmentInfo: AttachmentInfo{
				Life: life.Life(att.LifeID),
			},
			MountPoint: att.MountPoint,
		}
		if att.UnitName.Valid {
			fa.Unit = unit.Name(att.UnitName.String)
		} else {
			// This shouldn't happen, but log and skip if it does.
			st.logger.Errorf(ctx, "no unit name for filesystem attachment on storage instance %q", att.StorageID)
			continue
		}
		if att.MachineName.Valid {
			m := machine.Name(att.MachineName.String)
			fa.Machine = &m
		}
		info.FilesystemInfo.Attachments = append(info.FilesystemInfo.Attachments, fa)
	}

	var result = make([]StorageInstanceInfo, 0, len(mSI))
	for _, si := range mSI {
		result = append(result, *si)
	}
	return result, nil
}
