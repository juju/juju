// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"

	"github.com/canonical/sqlair"

	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storage/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/iter"
)

// GetStorageInstanceInfo returns the information about a single Storage
// Instance in the model.
//
// The following errors may be returned:
// - [domainstorageerrors.StorageInstanceNotFound] when no Storage Instance
// exists for the supplied uuid.
func (s *State) GetStorageInstanceInfo(
	ctx context.Context, uuid domainstorage.StorageInstanceUUID,
) (internal.StorageInstanceInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return internal.StorageInstanceInfo{}, errors.Capture(err)
	}

	var (
		inputUUID = entityUUID{UUID: uuid.String()}
		siInfo    storageInstanceInfo
	)

	// storageInstQ gets all of the basic information about a Storage Instance.
	/*
		id	parent	notused	detail
		18	0		46		SEARCH si USING INDEX sqlite_autoindex_storage_instance_1 (uuid=?)
		23	0		47		SEARCH sif USING INDEX sqlite_autoindex_storage_instance_filesystem_1 (storage_instance_uuid=?) LEFT-JOIN
		30	0		44		SEARCH sf USING COVERING INDEX sqlite_autoindex_storage_filesystem_1 (uuid=?) LEFT-JOIN
		37	0		47		SEARCH sfs USING INDEX sqlite_autoindex_storage_filesystem_status_1 (filesystem_uuid=?) LEFT-JOIN
		45	0		47		SEARCH siv USING INDEX sqlite_autoindex_storage_instance_volume_1 (storage_instance_uuid=?) LEFT-JOIN
		52	0		46		SEARCH sv USING INDEX sqlite_autoindex_storage_volume_1 (uuid=?) LEFT-JOIN
		60	0		47		SEARCH svs USING INDEX sqlite_autoindex_storage_volume_status_1 (volume_uuid=?) LEFT-JOIN
		68	0		47		SEARCH suo USING INDEX sqlite_autoindex_storage_unit_owner_1 (storage_instance_uuid=?) LEFT-JOIN
		75	0		46		SEARCH u USING INDEX sqlite_autoindex_unit_1 (uuid=?) LEFT-JOIN
	*/
	const storageInstQ = `
SELECT &storageInstanceInfo.*
FROM (
    SELECT    si.uuid,
              si.life_id,
              si.storage_id,
              si.storage_kind_id,
              suo.unit_uuid AS unit_owner_uuid,
              u.name AS unit_owner_name,
              sf.uuid AS storage_filesystem_uuid,
              sfs.status_id AS storage_filesystem_status_id,
              sfs.message AS storage_filesystem_status_message,
              sfs.updated_at storage_filesystem_status_updated_at,
              sv.persistent AS storage_volume_persistent,
              sv.uuid AS storage_volume_uuid,
              svs.status_id AS storage_volume_status_id,
              svs.message AS storage_volume_status_message,
              svs.updated_at AS storage_volume_status_updated_at
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN storage_filesystem_status sfs ON sf.uuid = sfs.filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    LEFT JOIN storage_volume_status svs ON sv.uuid = svs.volume_uuid
    LEFT JOIN storage_unit_owner suo ON si.uuid = suo.storage_instance_uuid
    LEFT JOIN unit u ON suo.unit_uuid = u.uuid
    WHERE     si.uuid = $entityUUID.uuid
)
`

	// storageInstAttachmentsQ gets all of the unit attachments of the Storage
	// Instance. It is deliberate for processing that the returned attachments
	// are ordered on Storage Attachment UUID.
	/*
		id	parent	notused	detail
		20	0		62		SEARCH sa USING INDEX idx_storage_attachment_unit_uuid_storage_instance_uuid (storage_instance_uuid=?)
		25	0		46		SEARCH u USING INDEX sqlite_autoindex_unit_1 (uuid=?)
		30	0		46		SEARCH m USING INDEX idx_machine_net_node (net_node_uuid=?) LEFT-JOIN
		37	0		47		SEARCH sif USING INDEX sqlite_autoindex_storage_instance_filesystem_1 (storage_instance_uuid=?) LEFT-JOIN
		44	0		62		SEARCH sfa USING INDEX idx_storage_filesystem_attachment_net_node_uuid (net_node_uuid=?) LEFT-JOIN
		54	0		47		SEARCH siv USING INDEX sqlite_autoindex_storage_instance_volume_1 (storage_instance_uuid=?) LEFT-JOIN
		61	0		46		SEARCH sva USING INDEX idx_storage_volume_attachment_volume_uuid (storage_volume_uuid=?) LEFT-JOIN
		72	0		0		USE TEMP B-TREE FOR GROUP BY
	*/
	const storageInstAttachmentsQ = `
SELECT &storageInstanceInfoAttachment.*
FROM (
    SELECT    sa.uuid,
              sa.life_id,
              sa.unit_uuid,
              sfa.mount_point AS storage_filesystem_attachment_mount_point,
              sfa.uuid AS storage_filesystem_attachment_uuid,
              sva.uuid AS storage_volume_attachment_uuid,
              u.name AS unit_name,
              m.name AS machine_name,
              m.uuid AS machine_uuid
    FROM      storage_attachment sa
    JOIN      unit u ON sa.unit_uuid = u.uuid
    LEFT JOIN machine m ON u.net_node_uuid = m.net_node_uuid
    LEFT JOIN storage_instance_filesystem sif ON sif.storage_instance_uuid = sa.storage_instance_uuid
    LEFT JOIN storage_filesystem_attachment sfa ON u.net_node_uuid = sfa.net_node_uuid
          AND sfa.storage_filesystem_uuid = sif.storage_filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON siv.storage_instance_uuid = sa.storage_instance_uuid
    LEFT JOIN storage_volume_attachment sva ON m.net_node_uuid = sva.net_node_uuid
          AND sva.storage_volume_uuid = siv.storage_volume_uuid
    WHERE     sa.storage_instance_uuid = $entityUUID.uuid
    GROUP BY  sa.uuid
    ORDER BY  sa.uuid
)
`

	// storageInstAttachmentBlockDeviceLinkQ is responsible for getting all of
	// the block device link device names for each Storage Attachment of a
	// Storage Instance. It is deliberate for processing that the returned
	// attachments are ordered on Storage Attachment UUID.
	/*
		id	parent	notused	detail
		15	0		47		SEARCH siv USING INDEX sqlite_autoindex_storage_instance_volume_1 (storage_instance_uuid=?)
		21	0		46		SEARCH sva USING INDEX idx_storage_volume_attachment_volume_uuid (storage_volume_uuid=?)
		26	0		44		SEARCH m USING COVERING INDEX idx_machine_net_node (net_node_uuid=?)
		30	0		44		SEARCH bd USING COVERING INDEX sqlite_autoindex_block_device_1 (uuid=?)
		35	0		61		SEARCH u USING INDEX idx_unit_net_node (net_node_uuid=?)
		43	0		47		SEARCH sa USING INDEX idx_storage_attachment_unit_uuid_storage_instance_uuid (storage_instance_uuid=? AND unit_uuid=?)
		49	0		55		SEARCH bdld USING COVERING INDEX idx_block_device_link_device (block_device_uuid=?)
		62	0		0		USE TEMP B-TREE FOR ORDER BY
	*/
	const storageInstAttachmentBlockDeviceLinkQ = `
SELECT &storageInstanceInfoAttachmentBlockDeviceLink.*
FROM (
    SELECT    sa.uuid AS storage_attachment_uuid,
              bdld.name AS block_device_link_device_name
    FROM      storage_attachment sa
    JOIN      unit u ON sa.unit_uuid = u.uuid
    JOIN      machine m ON u.net_node_uuid = m.net_node_uuid
    JOIN      storage_instance_volume siv ON siv.storage_instance_uuid = sa.storage_instance_uuid
    JOIN      storage_volume_attachment sva ON m.net_node_uuid = sva.net_node_uuid
          AND sva.storage_volume_uuid = siv.storage_volume_uuid
    JOIN      block_device bd ON sva.block_device_uuid = bd.uuid
    JOIN      block_device_link_device bdld ON bd.uuid = bdld.block_device_uuid
    WHERE     sa.storage_instance_uuid = $entityUUID.uuid
    ORDER BY  sa.uuid
)
`

	storageInstStmt, err := s.Prepare(storageInstQ, inputUUID, siInfo)
	if err != nil {
		return internal.StorageInstanceInfo{}, errors.Errorf(
			"preparing storage instance info statement: %w", err,
		)
	}

	storageInstAttachmentStmt, err := s.Prepare(
		storageInstAttachmentsQ, inputUUID, storageInstanceInfoAttachment{})
	if err != nil {
		return internal.StorageInstanceInfo{}, errors.Errorf(
			"preparing storage instance info attachment statement: %w", err,
		)
	}

	storageInstAttachmentBlockDeviceLinkStmt, err := s.Prepare(
		storageInstAttachmentBlockDeviceLinkQ,
		inputUUID,
		storageInstanceInfoAttachmentBlockDeviceLink{},
	)
	if err != nil {
		return internal.StorageInstanceInfo{}, errors.Errorf(
			"preparing storage instance info attachment block device link statement: %w",
			err,
		)
	}

	var (
		attachments                []storageInstanceInfoAttachment
		attachmentBlockDeviceLinks []storageInstanceInfoAttachmentBlockDeviceLink
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// siInfo MUST be zeroed off at the start of the tx due to retries.
		siInfo = storageInstanceInfo{}

		err := tx.Query(ctx, storageInstStmt, inputUUID).Get(&siInfo)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("storage instance not found").Add(
				domainstorageerrors.StorageInstanceNotFound,
			)
		} else if err != nil {
			return errors.Errorf(
				"getting storage instance information: %w", err,
			)
		}

		err = tx.Query(ctx, storageInstAttachmentStmt, inputUUID).GetAll(
			&attachments)
		if errors.Is(err, sqlair.ErrNoRows) {
			// We can safely bail out of the transaction at this stage. If no
			// attachments exist then there will be no block device links to get.
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting storage instance attachments information: %w", err,
			)
		}

		// Regardless of the type of storage instance we always attempt to get
		// block device links. There exists cases where filesystems are backed
		// by Volumes and this information may be relevant to the caller.
		err = tx.Query(
			ctx, storageInstAttachmentBlockDeviceLinkStmt, inputUUID,
		).GetAll(&attachmentBlockDeviceLinks)
		if errors.Is(err, sqlair.ErrNoRows) {
			// Not an error if no block device links exist
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting storage instance attachments block device information: %w",
				err,
			)
		}

		return nil
	})
	if err != nil {
		return internal.StorageInstanceInfo{}, err
	}

	retVal := internal.StorageInstanceInfo{
		Attachments: make([]internal.StorageInstanceInfoAttachment, 0, len(attachments)),
		Life:        domainlife.Life(siInfo.LifeID),
		Kind:        domainstorage.StorageKind(siInfo.StorageKindID),
		StorageID:   siInfo.StorageID,
		UUID:        domainstorage.StorageInstanceUUID(siInfo.UUID),
	}

	// Fill out Filesystem information for Storage Instance if available.
	if siInfo.FilesystemUUID.Valid {
		retVal.Filesystem = &internal.StorageInstanceInfoFilesystem{
			UUID: domainstorage.FilesystemUUID(siInfo.FilesystemUUID.V),
		}
	}

	// Fill out Filesystem status information for Storage Instance if available.
	if siInfo.FilesystemStatusID.Valid {
		retVal.Filesystem.Status = &internal.StorageInstanceInfoFilesystemStatus{
			Message: siInfo.FilesystemStatusMessage.V,
			Status:  domainstatus.StorageFilesystemStatusType(siInfo.FilesystemStatusID.V),
		}
		if siInfo.FilesystemStatusUpdatedAt.Valid {
			retVal.Filesystem.Status.UpdatedAt = &siInfo.FilesystemStatusUpdatedAt.V
		}
	}

	// Fill out Volume information for Storage instance if available.
	if siInfo.VolumeUUID.Valid {
		retVal.Volume = &internal.StorageInstanceInfoVolume{
			Persistent: siInfo.VolumePersistent.V,
			UUID:       domainstorage.VolumeUUID(siInfo.VolumeUUID.V),
		}
	}

	// Fill out Volume status information for Storage Instance if available.
	if siInfo.VolumeStatusID.Valid {
		retVal.Volume.Status = &internal.StorageInstanceInfoVolumeStatus{
			Message: siInfo.VolumeStatusMessage.V,
			Status:  domainstatus.StorageVolumeStatusType(siInfo.VolumeStatusID.V),
		}
		if siInfo.VolumeStatusUpdatedAt.Valid {
			retVal.Volume.Status.UpdatedAt = &siInfo.VolumeStatusUpdatedAt.V
		}
	}

	// Fill out Storage Instance Unit owner information if available.
	if siInfo.UnitOwnerUUID.Valid {
		retVal.UnitOwner = &internal.StorageInstanceInfoUnitOwner{
			Name: siInfo.UnitOwnerName.V,
			UUID: coreunit.UUID(siInfo.UnitOwnerUUID.V),
		}
	}

	// Partitioner based on Storage Attachment UUID for block device links.
	bdLinksPartitioner := iter.NewPartitioner(attachmentBlockDeviceLinks)

	// Process each attachment for the Storage Instance.
	for _, attachment := range attachments {
		val := internal.StorageInstanceInfoAttachment{
			Life:     domainlife.Life(attachment.LifeID),
			UnitName: attachment.UnitName,
			UnitUUID: coreunit.UUID(attachment.UnitUUID),
			UUID:     domainstorage.StorageAttachmentUUID(attachment.UUID),
		}

		if attachment.StorageFilesystemAttachmentUUID.Valid {
			val.Filesystem = &internal.StorageInstanceInfoAttachmentFilesystem{
				MountPoint: attachment.StorageFilesystemAttachmentMountPoint.V,
			}
		}

		if attachment.MachineUUID.Valid {
			val.Machine = &internal.StorageInstanceInfoAttachmentMachine{
				Name: attachment.MachineName.V,
				UUID: coremachine.UUID(attachment.MachineUUID.V),
			}
		}

		// Get all of the block device links for the Storage Attachment.
		blockDevicesSeq := bdLinksPartitioner.NextPart(attachment.UUID)
		deviceNameLinks := slices.Collect(iter.TransformSeq(
			blockDevicesSeq,
			func(v storageInstanceInfoAttachmentBlockDeviceLink) string {
				return v.BlockDeviceLinkDeviceName
			},
		))

		if attachment.StorageVolumeAttachmentUUID.Valid {
			val.Volume = &internal.StorageInstanceInfoAttachmentVolume{
				DeviceNameLinks: deviceNameLinks,
			}
		}

		retVal.Attachments = append(retVal.Attachments, val)
	}

	return retVal, nil
}

// GetStorageInstanceUUIDByID retrieves the UUID of a storage instance by
// its ID.
//
// The following errors may be returned:
// - [domainstorageerrors.StorageInstanceNotFound] when no storage
// instance exists for the provided ID.
func (s *State) GetStorageInstanceUUIDByID(
	ctx context.Context, storageID string,
) (domainstorage.StorageInstanceUUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		input = storageInstanceID{ID: storageID}
		dbVal entityUUID
	)

	stmt, err := s.Prepare(`
SELECT &entityUUID.*
FROM   storage_instance
WHERE  storage_id = $storageInstanceID.storage_id`,
		input, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage instance with ID %q does not exist", storageID,
			).Add(domainstorageerrors.StorageInstanceNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return domainstorage.StorageInstanceUUID(dbVal.UUID), nil
}

// GetStorageInstanceUUIDsByIDs retrieves the UUIDs of storage instances by
// their IDs.
func (s *State) GetStorageInstanceUUIDsByIDs(
	ctx context.Context,
	storageIDs []string,
) (map[string]domainstorage.StorageInstanceUUID, error) {
	if len(storageIDs) == 0 {
		return map[string]domainstorage.StorageInstanceUUID{}, nil
	}

	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// De-dupe input.
	storageInstanceIDs := storageInstanceIDs(
		slices.Compact(slices.Sorted(slices.Values(storageIDs))),
	)

	stmt, err := s.Prepare(`
SELECT &storageInstanceUUIDAndID.*
FROM   storage_instance
WHERE  storage_id IN ($storageInstanceIDs[:])`,
		storageInstanceUUIDAndID{}, storageInstanceIDs,
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageInstanceUUIDAndID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, storageInstanceIDs).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]domainstorage.StorageInstanceUUID, len(dbVals))
	for _, val := range dbVals {
		result[val.ID] = domainstorage.StorageInstanceUUID(val.UUID)
	}

	return result, nil
}
