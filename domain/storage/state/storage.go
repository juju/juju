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
func (st *State) ListStorageInstances(ctx context.Context) ([]domainstorage.StorageInstanceDetails, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT (si.storage_id, si.storage_kind_id, si.life_id, sv.persistent) AS (&dbStorageInstanceDetails.*),
       u.name AS &dbStorageInstanceDetails.owner_unit_name
FROM   storage_instance si
LEFT JOIN storage_instance_volume siv ON si.uuid=siv.storage_instance_uuid
LEFT JOIN storage_volume sv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN storage_unit_owner suo ON si.uuid=suo.storage_instance_uuid
LEFT JOIN unit u ON suo.unit_uuid=u.uuid`, dbStorageInstanceDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbInstances []dbStorageInstanceDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&dbInstances)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("listing storage instances: %w", err)
	}

	result := make([]domainstorage.StorageInstanceDetails, 0, len(dbInstances))
	for _, dbInstance := range dbInstances {
		si := domainstorage.StorageInstanceDetails{
			ID:         dbInstance.ID,
			Kind:       domainstorage.StorageKind(dbInstance.KindID),
			Life:       life.Life(dbInstance.LifeID),
			Persistent: dbInstance.Persistent,
		}
		if dbInstance.OwnerUnitName.Valid {
			u := unit.Name(dbInstance.OwnerUnitName.String)
			si.Owner = &u
		}
		result = append(result, si)
	}
	return result, nil
}

type storageIDs []string

// ListVolumeWithAttachments returns a map of volume storage IDs to their
// information including attachments.
func (st *State) ListVolumeWithAttachments(
	ctx context.Context,
	storageInstanceIDs ...string,
) (map[string]VolumeDetails, error) {
	if len(storageInstanceIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT DISTINCT (si.storage_id, sva.life_id, sva.block_device_uuid) AS (&dbVolumeAttachmentDetails.*),
                (svs.status_id, svs.message, svs.updated_at) AS (&dbVolumeAttachmentDetails.*),
                u.name AS &dbVolumeAttachmentDetails.unit_name,
                m.name AS &dbVolumeAttachmentDetails.machine_name
FROM            storage_volume sv
LEFT JOIN       storage_volume_attachment sva ON sv.uuid=sva.storage_volume_uuid
LEFT JOIN       storage_volume_status svs ON svs.volume_uuid=sv.uuid
LEFT JOIN       storage_instance_volume siv ON siv.storage_volume_uuid=sv.uuid
LEFT JOIN       storage_instance si ON si.uuid=siv.storage_instance_uuid
LEFT JOIN       storage_attachment sa ON sa.storage_instance_uuid=si.uuid
LEFT JOIN       unit u ON u.uuid=sa.unit_uuid
LEFT JOIN       machine m ON sva.net_node_uuid=m.net_node_uuid
WHERE si.storage_id IN ($storageIDs[:])`, dbVolumeAttachmentDetails{}, storageIDs{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVolumeAttachmentData []dbVolumeAttachmentDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, storageIDs(storageInstanceIDs)).GetAll(&dbVolumeAttachmentData)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("listing volume attachments: %w", err)
	}

	result := make(map[string]VolumeDetails)
	for _, att := range dbVolumeAttachmentData {
		info, ok := result[att.StorageID]
		if !ok {
			statusValue, err := status.DecodeStorageVolumeStatus(att.StatusID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			info = VolumeDetails{
				StorageID: att.StorageID,
				Status: status.StatusInfo[status.StorageVolumeStatusType]{
					Status:  statusValue,
					Message: att.Message,
					Since:   att.UpdatedAt,
				},
			}
		}
		va := VolumeAttachmentDetails{
			AttachmentDetails: AttachmentDetails{
				Life: life.Life(att.LifeID),
			},
			BlockDeviceUUID: att.BlockDeviceUUID,
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
		info.Attachments = append(info.Attachments, va)
		result[att.StorageID] = info
	}
	return result, nil
}

// ListFilesystemWithAttachments returns a map of filesystem storage IDs to their
// information including attachments.
func (st *State) ListFilesystemWithAttachments(
	ctx context.Context,
	storageInstanceIDs ...string,
) (map[string]FilesystemDetails, error) {
	if len(storageInstanceIDs) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    (si.storage_id, sfa.life_id, sfa.mount_point) AS (&dbFilesystemAttachmentDetails.*),
          (sfs.status_id, sfs.message, sfs.updated_at) AS (&dbFilesystemAttachmentDetails.*),
          u.name AS &dbFilesystemAttachmentDetails.unit_name,
          m.name AS &dbFilesystemAttachmentDetails.machine_name
FROM      storage_filesystem sf
LEFT JOIN storage_filesystem_attachment sfa ON sf.uuid=sfa.storage_filesystem_uuid
LEFT JOIN storage_filesystem_status sfs ON sfs.filesystem_uuid=sf.uuid
LEFT JOIN storage_instance_filesystem sif ON sif.storage_filesystem_uuid=sf.uuid
LEFT JOIN storage_instance si ON si.uuid=sif.storage_instance_uuid
LEFT JOIN storage_attachment sa ON sa.storage_instance_uuid=si.uuid
LEFT JOIN unit u ON u.uuid=sa.unit_uuid
LEFT JOIN machine m ON sfa.net_node_uuid=m.net_node_uuid
WHERE si.storage_id IN ($storageIDs[:])`, dbFilesystemAttachmentDetails{}, storageIDs{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbFilesystemAttachmentData []dbFilesystemAttachmentDetails
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, storageIDs(storageInstanceIDs)).GetAll(&dbFilesystemAttachmentData)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	if err != nil {
		return nil, errors.Errorf("listing filesystem attachments: %w", err)
	}

	result := make(map[string]FilesystemDetails)
	for _, att := range dbFilesystemAttachmentData {
		info, ok := result[att.StorageID]
		if !ok {
			statusValue, err := status.DecodeStorageFilesystemStatus(att.StatusID)
			if err != nil {
				return nil, errors.Capture(err)
			}
			info = FilesystemDetails{
				StorageID: att.StorageID,
				Status: status.StatusInfo[status.StorageFilesystemStatusType]{
					Status:  statusValue,
					Message: att.Message,
					Since:   att.UpdatedAt,
				},
			}
		}
		fa := FilesystemAttachmentDetails{
			AttachmentDetails: AttachmentDetails{
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
		info.Attachments = append(info.Attachments, fa)
		result[att.StorageID] = info
	}
	return result, nil
}
