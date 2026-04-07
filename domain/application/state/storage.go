// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	domainlife "github.com/juju/juju/domain/life"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/iter"
)

// GetApplicationStorageDirectivesInfo returns the storage directives set for an application,
// keyed to the storage name. If the application does not have any storage
// directives set then an empty result is returned.
//
// If the application does not exist, then a [applicationerrors.ApplicationNotFound]
// error is returned.
func (st *State) GetApplicationStorageDirectivesInfo(
	ctx context.Context,
	appUUID coreapplication.UUID,
) (map[string]application.ApplicationStorageInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	appUUIDInput := entityUUID{UUID: appUUID.String()}

	query, err := st.Prepare(`
SELECT &applicationStorageInfo.* FROM (
	SELECT  asd.count,
			asd.size_mib,
			asd.storage_name,
			sp.name as storage_pool_name
	FROM application_storage_directive asd
	JOIN storage_pool sp ON sp.uuid = asd.storage_pool_uuid
	WHERE asd.application_uuid = $entityUUID.uuid
)
		`,
		appUUIDInput, applicationStorageInfo{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	storageInfoResult := []applicationStorageInfo{}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkApplicationExists(ctx, tx, appUUID)
		if err != nil {
			return errors.Errorf(
				"checking application %q exists: %w", appUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"application %q does not exist", appUUID,
			).Add(applicationerrors.ApplicationNotFound)
		}

		err = tx.Query(ctx, query, appUUIDInput).GetAll(&storageInfoResult)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Preallocate map with expected size since each storage name
	appStorage := make(map[string]application.ApplicationStorageInfo, len(storageInfoResult))
	for _, info := range storageInfoResult {
		appStorage[info.StorageName] = application.ApplicationStorageInfo{
			StoragePoolName: info.StoragePoolName,
			SizeMiB:         info.SizeMiB,
			Count:           info.Count,
		}
	}

	return appStorage, nil
}

// GetApplicationStorageDirectives returns the storage directives that are
// set for an application . If the application does not have any storage
// directives set then an empty result is returned.
//
// The following error types can be expected:
// - [github.com/juju/juju/domain/application/errors.ApplicationNotFound]
// when the application no longer exists.
func (st *State) GetApplicationStorageDirectives(
	ctx context.Context,
	appUUID coreapplication.UUID,
) ([]internal.StorageDirective, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	appUUIDInput := entityUUID{UUID: appUUID.String()}
	query, err := st.Prepare(`
SELECT &storageDirective.* FROM (
    SELECT asd.count,
           asd.size_mib,
           asd.storage_name,
           asd.storage_pool_uuid,
           cm.name AS charm_metadata_name,
           csk.kind AS charm_storage_kind,
           cs.count_max AS count_max
    FROM   application_storage_directive asd
    JOIN   charm_storage cs ON cs.charm_uuid = asd.charm_uuid AND cs.name = asd.storage_name
    JOIN   charm_metadata cm ON cm.charm_uuid = asd.charm_uuid
    JOIN   charm_storage_kind csk ON csk.id = cs.storage_kind_id
    WHERE  application_uuid = $entityUUID.uuid
)
		`,
		appUUIDInput, storageDirective{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	dbVals := []storageDirective{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkApplicationExists(ctx, tx, appUUID)
		if err != nil {
			return errors.Errorf(
				"checking application %q exists: %w", appUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"application %q does not exist", appUUID,
			).Add(applicationerrors.ApplicationNotFound)
		}

		err = tx.Query(ctx, query, appUUIDInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]internal.StorageDirective, 0, len(dbVals))
	for _, val := range dbVals {
		rval = append(rval, internal.StorageDirective{
			CharmMetadataName: val.CharmMetadataName,
			CharmStorageType:  charm.StorageType(val.CharmStorageKind),
			Count:             val.Count,
			MaxCount:          val.CountMax,
			Name:              domainstorage.Name(val.StorageName),
			PoolUUID:          domainstorage.StoragePoolUUID(val.StoragePoolUUID),
			Size:              val.SizeMiB,
		})
	}
	return rval, nil
}

// GetStorageInstanceCompositionByUUID returns the storage compositions for
// the specified storage instance.
func (st *State) GetStorageInstanceCompositionByUUID(
	ctx context.Context,
	storageInstanceUUID domainstorage.StorageInstanceUUID,
) (
	internal.StorageInstanceComposition,
	error,
) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.StorageInstanceComposition{}, errors.Capture(err)
	}
	storageInstanceUUIDInput := entityUUID{UUID: storageInstanceUUID.String()}
	compositionQ := `
SELECT &storageInstanceComposition.*
FROM (
    SELECT    sf.uuid AS filesystem_uuid,
              sf.provision_scope_id AS filesystem_provision_scope,
              si.storage_name AS storage_name,
              si.uuid AS uuid,
              sv.uuid AS volume_uuid,
              sv.provision_scope_id AS volume_provision_scope
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    WHERE     si.uuid = $entityUUID.uuid
)
`

	stmt, err := st.Prepare(
		compositionQ,
		storageInstanceUUIDInput,
		storageInstanceComposition{},
	)
	if err != nil {
		return internal.StorageInstanceComposition{}, errors.Capture(err)
	}

	var dbVal storageInstanceComposition
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, storageInstanceUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage instance %q not found", storageInstanceUUID).
				Add(storageerrors.StorageInstanceNotFound)
		}
		return err
	})
	if err != nil {
		return internal.StorageInstanceComposition{}, errors.Capture(err)
	}

	rval := makeStorageInstanceComposition(dbVal)
	return rval, nil
}

// GetStorageInstancesForProviderIDs returns all of the storage instances
// found in the model using one of the provider ids supplied. The storage
// instance must also not be owned by a unit. If no storage instances are found
// then an empty result is returned.
func (st *State) GetStorageInstancesForProviderIDs(
	ctx context.Context,
	ids []string,
) ([]internal.StorageInstanceInfoForAttach, error) {
	// Early exit if no ids are supplied. We cannot have empty values with an
	// IN expression.
	if len(ids) == 0 {
		return []internal.StorageInstanceInfoForAttach{}, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	providerIDsInput := storageProviderIDs(ids)

	var (
		storageInstInfos       []storageInstanceInfoForAttach
		storageInstAttachments []storageInstanceUnitAttachmentByStorageUUID
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageInstInfos, err = st.getStorageInstancesInfoForAttachByProviderIDs(
			ctx, tx, providerIDsInput,
		)
		if err != nil {
			return errors.Errorf(
				"getting storage instances information for provider IDs: %w",
				err,
			)
		}

		storageInstAttachments, err = st.getStorageInstanceUnitAttachmentsForProviderIDs(
			ctx, tx, providerIDsInput,
		)
		if err != nil {
			return errors.Errorf(
				"getting storage instance unit attachments for provider IDs: %w",
				err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return makeStorageInstanceInfosForAttach(
		storageInstInfos,
		storageInstAttachments,
	), nil
}

// getStorageInstancesInfoForAttachByProviderIDs returns attach metadata for
// storage instances that match any supplied provider ID and are not owned by a
// unit.
//
// Provider IDs are matched against the storage filesystem and storage volume
// rows associated with each storage instance.
//
// Returned values are ordered by storage instance UUID.
func (st *State) getStorageInstancesInfoForAttachByProviderIDs(
	ctx context.Context,
	tx *sqlair.TX,
	providerIDsInput storageProviderIDs,
) ([]storageInstanceInfoForAttach, error) {
	q := `
WITH matched_storage_instances AS (
    SELECT DISTINCT si.uuid
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    WHERE     si.uuid NOT IN (SELECT storage_instance_uuid
                              FROM storage_unit_owner)
    AND       (sf.provider_id IN ($storageProviderIDs[:])
           OR  sv.provider_id IN ($storageProviderIDs[:]))
)
SELECT * AS &storageInstanceInfoForAttach.* FROM (
    SELECT    si.uuid,
              si.charm_name,
              si.storage_name,
              si.life_id,
              si.requested_size_mib,
              si.storage_kind_id,
              sif.storage_filesystem_uuid AS filesystem_uuid,
              sf.size_mib AS filesystem_size_mib,
              sf.provision_scope_id AS filesystem_provision_scope_id,
              mf.machine_uuid AS filesystem_owned_machine_uuid,
              siv.storage_volume_uuid AS volume_uuid,
              sv.size_mib AS volume_size_mib,
              sv.provision_scope_id AS volume_provision_scope_id,
              mv.machine_uuid AS volume_owned_machine_uuid
    FROM      matched_storage_instances msi
    JOIN      storage_instance si ON si.uuid = msi.uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN machine_filesystem mf ON sif.storage_filesystem_uuid = mf.filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    LEFT JOIN machine_volume mv ON siv.storage_volume_uuid = mv.volume_uuid
    ORDER BY  si.uuid
)
`
	queryStmt, err := st.Prepare(
		q,
		providerIDsInput,
		storageInstanceInfoForAttach{},
	)
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instances info for provider IDs query: %w", err,
		)
	}

	var siInfos []storageInstanceInfoForAttach
	err = tx.Query(ctx, queryStmt, providerIDsInput).GetAll(&siInfos)
	if errors.Is(err, sqlair.ErrNoRows) {
		// No results is not an error
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage instances info for provider IDs: %w", err,
		)
	}

	return siInfos, nil
}

// getStorageInstanceUnitAttachmentsForProviderIDs returns storage attachment
// rows for storage instances that match any supplied provider ID and are not
// owned by a unit.
//
// Returned values are ordered by storage instance UUID.
func (st *State) getStorageInstanceUnitAttachmentsForProviderIDs(
	ctx context.Context,
	tx *sqlair.TX,
	providerIDsInput storageProviderIDs,
) ([]storageInstanceUnitAttachmentByStorageUUID, error) {
	q := `
WITH matched_storage_instances AS (
    SELECT DISTINCT si.uuid
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    WHERE     si.uuid NOT IN (SELECT storage_instance_uuid
                              FROM storage_unit_owner)
    AND       (sf.provider_id IN ($storageProviderIDs[:])
           OR  sv.provider_id IN ($storageProviderIDs[:]))
)
SELECT &storageInstanceUnitAttachmentByStorageUUID.*
FROM (
    SELECT    sa.storage_instance_uuid,
              sa.unit_uuid,
              sa.uuid
    FROM      storage_attachment sa
    JOIN      matched_storage_instances msi ON sa.storage_instance_uuid = msi.uuid
    ORDER BY  sa.storage_instance_uuid, sa.unit_uuid, sa.uuid
)
`
	queryStmt, err := st.Prepare(
		q,
		providerIDsInput,
		storageInstanceUnitAttachmentByStorageUUID{},
	)
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instance unit attachments for provider IDs query: %w",
			err,
		)
	}

	var attachments []storageInstanceUnitAttachmentByStorageUUID
	err = tx.Query(ctx, queryStmt, providerIDsInput).GetAll(&attachments)
	if errors.Is(err, sqlair.ErrNoRows) {
		// No result is not a error
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage instance unit attachments for provider IDs: %w",
			err,
		)
	}

	return attachments, nil
}

func makeStorageInstanceComposition(dbVal storageInstanceComposition) internal.StorageInstanceComposition {
	v := internal.StorageInstanceComposition{
		StorageName: domainstorage.Name(dbVal.StorageName),
		UUID:        domainstorage.StorageInstanceUUID(dbVal.UUID),
	}

	if dbVal.FilesystemUUID.Valid {
		v.Filesystem = &internal.StorageInstanceCompositionFilesystem{
			ProvisionScope: domainstorageprov.ProvisionScope(dbVal.FilesystemProvisionScope.V),
			UUID:           domainstorage.FilesystemUUID(dbVal.FilesystemUUID.V),
		}
	}

	if dbVal.VolumeUUID.Valid {
		v.Volume = &internal.StorageInstanceCompositionVolume{
			ProvisionScope: domainstorageprov.ProvisionScope(dbVal.VolumeProvisionScope.V),
			UUID:           domainstorage.VolumeUUID(dbVal.VolumeUUID.V),
		}
	}
	return v
}

// GetUnitOwnedStorageInstances returns attachment metadata for all
// storage instances owned by the unit in the model. If the unit does not
// currently own any storage instances then an empty result is returned.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
func (st *State) GetUnitOwnedStorageInstances(
	ctx context.Context,
	unitUUID coreunit.UUID,
) (
	[]internal.StorageInstanceInfoForAttach,
	[]internal.StorageAttachmentComposition,
	error,
) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	uuidInput := entityUUID{UUID: unitUUID.String()}

	attachmentCompositionQ := `
SELECT &storageAttachmentComposition.*
FROM (
    SELECT    sia.storage_instance_uuid AS storage_instance_uuid,
              sia.uuid AS uuid,
              sfa.uuid AS filesystem_attachment_uuid,
              sfa.storage_filesystem_uuid AS filesystem_uuid,
              sfa.provision_scope_id AS filesystem_attachment_provision_scope,
              sva.uuid AS volume_attachment_uuid,
              sva.storage_volume_uuid AS volume_uuid,
              sva.provision_scope_id AS volume_attachment_provision_scope
    FROM      storage_attachment sia
    JOIN      unit u ON sia.unit_uuid = u.uuid
    LEFT JOIN storage_instance_filesystem sif ON sia.storage_instance_uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_attachment sfa ON sif.storage_filesystem_uuid = sfa.storage_filesystem_uuid AND u.net_node_uuid = sfa.net_node_uuid
    LEFT JOIN storage_instance_volume siv ON sia.storage_instance_uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume_attachment sva ON siv.storage_volume_uuid = sva.storage_volume_uuid AND u.net_node_uuid = sva.net_node_uuid
    WHERE     sia.unit_uuid = $entityUUID.uuid
)
`
	attachmentStmt, err := st.Prepare(
		attachmentCompositionQ,
		uuidInput,
		storageAttachmentComposition{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	var dbAttachmentVals []storageAttachmentComposition
	var storageInstInfos []storageInstanceInfoForAttach
	var storageInstAttachments []storageInstanceUnitAttachmentByStorageUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID.String())
		if err != nil {
			return errors.Errorf(
				"checking unit %q exists: %w", unitUUID, err,
			)
		}
		if !exists {
			return errors.Errorf("unit %q does not exist", unitUUID).Add(
				applicationerrors.UnitNotFound,
			)
		}

		storageInstInfos, err = st.getStorageInstancesInfoForUnitOwnedStorage(
			ctx, tx, unitUUID,
		)
		if err != nil {
			return errors.Errorf(
				"getting owned storage instances information for attachment: %w",
				err,
			)
		}

		err = tx.Query(ctx, attachmentStmt, uuidInput).GetAll(&dbAttachmentVals)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}

		storageInstAttachments, err = st.getStorageInstanceUnitAttachmentsForUnitOwnedStorage(
			ctx, tx, unitUUID,
		)
		if err != nil {
			return errors.Errorf(
				"getting owned storage instance unit attachments: %w",
				err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	retAttachmentComp := make(
		[]internal.StorageAttachmentComposition, 0, len(dbAttachmentVals),
	)
	for _, dbAttachmentVal := range dbAttachmentVals {
		v := internal.StorageAttachmentComposition{
			UUID: domainstorage.StorageAttachmentUUID(dbAttachmentVal.UUID),
			StorageInstanceUUID: domainstorage.StorageInstanceUUID(
				dbAttachmentVal.StorageInstanceUUID,
			),
		}
		if dbAttachmentVal.FilesystemAttachmentUUID.Valid &&
			dbAttachmentVal.FilesystemUUID.Valid {
			r := internal.StorageInstanceCompositionFilesystemAttachment{
				ProvisionScope: domainstorage.ProvisionScope(
					dbAttachmentVal.FilesystemAttachmentProvisionScope.V,
				),
				UUID: domainstorage.FilesystemAttachmentUUID(
					dbAttachmentVal.FilesystemAttachmentUUID.V,
				),
				FilesystemUUID: domainstorage.FilesystemUUID(
					dbAttachmentVal.FilesystemUUID.V,
				),
			}
			v.FilesystemAttachment = &r
		}
		if dbAttachmentVal.VolumeAttachmentUUID.Valid &&
			dbAttachmentVal.VolumeUUID.Valid {
			r := internal.StorageInstanceCompositionVolumeAttachment{
				ProvisionScope: domainstorage.ProvisionScope(
					dbAttachmentVal.VolumeAttachmentProvisionScope.V,
				),
				UUID: domainstorage.VolumeAttachmentUUID(
					dbAttachmentVal.VolumeAttachmentUUID.V,
				),
				VolumeUUID: domainstorage.VolumeUUID(
					dbAttachmentVal.VolumeUUID.V,
				),
			}
			v.VolumeAttachment = &r
		}
		retAttachmentComp = append(retAttachmentComp, v)
	}

	return makeStorageInstanceInfosForAttach(
		storageInstInfos,
		storageInstAttachments,
	), retAttachmentComp, nil
}

// getStorageInstancesInfoForUnitOwnedStorage returns attach metadata for all
// storage instances owned by the supplied unit, ordered by storage UUID.
func (st *State) getStorageInstancesInfoForUnitOwnedStorage(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
) ([]storageInstanceInfoForAttach, error) {
	inUUID := entityUUID{UUID: unitUUID.String()}

	q := `
SELECT * AS &storageInstanceInfoForAttach.* FROM (
    SELECT    si.uuid,
              si.charm_name,
              si.storage_name,
              si.life_id,
              si.requested_size_mib,
              si.storage_kind_id,
              sif.storage_filesystem_uuid AS filesystem_uuid,
              sf.size_mib AS filesystem_size_mib,
              sf.provision_scope_id AS filesystem_provision_scope_id,
              mf.machine_uuid AS filesystem_owned_machine_uuid,
              siv.storage_volume_uuid AS volume_uuid,
              sv.size_mib AS volume_size_mib,
              sv.provision_scope_id AS volume_provision_scope_id,
              mv.machine_uuid AS volume_owned_machine_uuid
    FROM      storage_unit_owner suo
    JOIN      storage_instance si ON suo.storage_instance_uuid = si.uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN machine_filesystem mf ON sif.storage_filesystem_uuid = mf.filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    LEFT JOIN machine_volume mv ON siv.storage_volume_uuid = mv.volume_uuid
    WHERE     suo.unit_uuid = $entityUUID.uuid
    ORDER BY  si.uuid
)
`
	queryStmt, err := st.Prepare(q, inUUID, storageInstanceInfoForAttach{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing query for getting unit-owned storage instances info for attachment: %w",
			err,
		)
	}

	var siInfos []storageInstanceInfoForAttach
	err = tx.Query(ctx, queryStmt, inUUID).GetAll(&siInfos)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting unit-owned storage instances information for "+
				"attachment: %w",
			err,
		)
	}

	return siInfos, nil
}

// getStorageInstanceUnitAttachmentsForUnitOwnedStorage returns storage
// instance attachments for all storage instances owned by the supplied unit.
//
// The attachments returned are not exclusively to the unit that is the subject
// of this function.
//
// Results are ordered by storage instance UUID.
func (st *State) getStorageInstanceUnitAttachmentsForUnitOwnedStorage(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
) ([]storageInstanceUnitAttachmentByStorageUUID, error) {
	inUUID := entityUUID{UUID: unitUUID.String()}

	q := `
SELECT &storageInstanceUnitAttachmentByStorageUUID.*
FROM (
	SELECT sa.storage_instance_uuid,
	       sa.unit_uuid,
	       sa.uuid
	FROM   storage_attachment sa
	JOIN   storage_unit_owner suo ON sa.storage_instance_uuid = suo.storage_instance_uuid
	WHERE  suo.unit_uuid = $entityUUID.uuid
	ORDER BY sa.storage_instance_uuid
)
`
	stmt, err := st.Prepare(
		q,
		inUUID,
		storageInstanceUnitAttachmentByStorageUUID{},
	)
	if err != nil {
		return nil, errors.Errorf(
			"preparing unit-owned storage instance unit attachments query: %w",
			err,
		)
	}

	var result []storageInstanceUnitAttachmentByStorageUUID
	err = tx.Query(ctx, stmt, inUUID).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

// getStorageInstanceUnitAttachments returns the units attached to a storage
// instance including the Storage Attachment UUID, Unit UUIDs and Unit Names.
// The caller must already have verified that the storage instance exists. If
// the storage has no attachments, an empty result is returned.
func (st *State) getStorageInstanceUnitAttachments(
	ctx context.Context,
	tx *sqlair.TX,
	siUUID domainstorage.StorageInstanceUUID,
) ([]storageInstanceUnitAttachment, error) {
	inUUID := entityUUID{UUID: siUUID.String()}

	attachStmt, err := st.Prepare(`
SELECT &storageInstanceUnitAttachment.*
FROM (
	SELECT u.uuid AS unit_uuid,
		   u.name AS unit_name,
		   sia.uuid AS uuid
	FROM   storage_attachment sia
	JOIN   unit u ON sia.unit_uuid = u.uuid
	WHERE  sia.storage_instance_uuid = $entityUUID.uuid
)
`,
		inUUID, storageInstanceUnitAttachment{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instance unit attachments query: %w", err,
		)
	}

	var result []storageInstanceUnitAttachment
	err = tx.Query(ctx, attachStmt, inUUID).GetAll(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

// GetStorageAttachInfoForStorageInstances returns metadata and attachment
// details for the specified storage instances.
//
// Duplicate storage instance UUIDs are de-duplicated and are not treated as
// separate requests.
//
// The following errors can be expected:
// - [storageerrors.StorageInstanceNotFound] when any storage instance does not
// exist.
func (st *State) GetStorageAttachInfoForStorageInstances(
	ctx context.Context,
	storageUUIDs []domainstorage.StorageInstanceUUID,
) ([]internal.StorageInstanceInfoForAttach, error) {
	if len(storageUUIDs) == 0 {
		return []internal.StorageInstanceInfoForAttach{}, nil
	}

	requestedUUIDs := make(storageInstanceUUIDs, 0, len(storageUUIDs))
	requestedUUIDs = slices.AppendSeq(requestedUUIDs, iter.TransformSeq(
		slices.Values(storageUUIDs),
		func(uuid domainstorage.StorageInstanceUUID) string {
			return uuid.String()
		},
	))
	slices.Sort(requestedUUIDs)
	requestedUUIDs = slices.Compact(requestedUUIDs)

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		storageInstInfos       []storageInstanceInfoForAttach
		storageInstAttachments []storageInstanceUnitAttachmentByStorageUUID
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		storageInstInfos, err = st.getStorageInstancesInfoForAttach(
			ctx, tx, requestedUUIDs,
		)
		if err != nil {
			return errors.Errorf(
				"getting storage instances information for attachment: %w", err,
			)
		}
		if len(storageInstInfos) != len(requestedUUIDs) {
			return errors.New(
				"one or more storage instances do not exist",
			).Add(storageerrors.StorageInstanceNotFound)
		}

		storageInstAttachments, err = st.getStorageInstanceUnitAttachmentsForStorageInstances(
			ctx, tx, requestedUUIDs,
		)
		if err != nil {
			return errors.Errorf(
				"getting storage instance unit attachments: %w", err,
			)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return makeStorageInstanceInfosForAttach(
		storageInstInfos,
		storageInstAttachments,
	), nil
}

// makeStorageInstanceInfosForAttach merges storage instance and attachment
// rows into values used by attach decision logic.
//
// Both arguments MUST be ordered by storage instance UID (UUID) so
// partitioning can align attachment rows with their storage instance.
func makeStorageInstanceInfosForAttach(
	storageInstInfos []storageInstanceInfoForAttach,
	storageInstAttachments []storageInstanceUnitAttachmentByStorageUUID,
) []internal.StorageInstanceInfoForAttach {
	if len(storageInstInfos) == 0 {
		return []internal.StorageInstanceInfoForAttach{}
	}

	partitioner := iter.NewPartitioner(storageInstAttachments)
	defer partitioner.Close()

	retVals := make(
		[]internal.StorageInstanceInfoForAttach,
		0,
		len(storageInstInfos),
	)
	for _, storageInstInfo := range storageInstInfos {
		retVal := internal.StorageInstanceInfoForAttach{
			StorageInstanceInfo: internal.StorageInstanceInfo{
				UUID:             domainstorage.StorageInstanceUUID(storageInstInfo.UUID),
				Life:             domainlife.Life(storageInstInfo.Life),
				Kind:             domainstorage.StorageKind(storageInstInfo.StorageKindID),
				RequestedSizeMIB: storageInstInfo.RequestedSizeMIB,
				StorageName:      storageInstInfo.StorageName,
			},
		}

		if storageInstInfo.CharmName.Valid {
			retVal.StorageInstanceInfo.CharmName = new(storageInstInfo.CharmName.V)
		}

		if storageInstInfo.FilesystemUUID.Valid {
			retVal.StorageInstanceInfo.Filesystem =
				&internal.StorageInstanceFilesystemInfo{
					UUID: domainstorage.FilesystemUUID(
						storageInstInfo.FilesystemUUID.V,
					),
					ProvisionScope: domainstorageprov.ProvisionScope(
						storageInstInfo.FilesystemProvisionScopeID.V,
					),
					Size: storageInstInfo.FilesystemSizeMIB.V,
				}
		}
		if storageInstInfo.FilesystemOwnedMachineUUID.Valid &&
			retVal.StorageInstanceInfo.Filesystem != nil {
			retVal.StorageInstanceInfo.Filesystem.OwningMachineUUID =
				new(coremachine.UUID(storageInstInfo.FilesystemOwnedMachineUUID.V))
		}

		if storageInstInfo.VolumeUUID.Valid {
			retVal.StorageInstanceInfo.Volume = &internal.StorageInstanceVolumeInfo{
				UUID: domainstorage.VolumeUUID(storageInstInfo.VolumeUUID.V),
				ProvisionScope: domainstorageprov.ProvisionScope(
					storageInstInfo.VolumeProvisionScopeID.V,
				),
				Size: storageInstInfo.VolumeSizeMIB.V,
			}
		}
		if storageInstInfo.VolumeOwnedMachineUUID.Valid &&
			retVal.StorageInstanceInfo.Volume != nil {
			retVal.StorageInstanceInfo.Volume.OwningMachineUUID =
				new(coremachine.UUID(storageInstInfo.VolumeOwnedMachineUUID.V))
		}

		for row := range partitioner.NextPart(storageInstInfo.UUID) {
			retVal.StorageInstanceAttachments = append(
				retVal.StorageInstanceAttachments,
				internal.StorageInstanceUnitAttachment{
					UnitUUID: coreunit.UUID(row.UnitUUID),
					UUID:     domainstorage.StorageAttachmentUUID(row.UUID),
				},
			)
		}

		retVals = append(retVals, retVal)
	}

	return retVals
}

// getStorageInstanceUnitAttachmentsForStorageInstances returns storage
// attachment rows for the supplied storage instances, ordered by storage
// instance UUID.
func (st *State) getStorageInstanceUnitAttachmentsForStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	storageUUIDs storageInstanceUUIDs,
) ([]storageInstanceUnitAttachmentByStorageUUID, error) {
	if len(storageUUIDs) == 0 {
		return nil, nil
	}

	q := `
SELECT &storageInstanceUnitAttachmentByStorageUUID.*
FROM (
	SELECT sa.storage_instance_uuid,
	       sa.unit_uuid,
	       sa.uuid
	FROM   storage_attachment sa
	WHERE  sa.storage_instance_uuid IN ($storageInstanceUUIDs[:])
	ORDER BY sa.storage_instance_uuid, sa.unit_uuid, sa.uuid
)
`
	stmt, err := st.Prepare(
		q,
		storageUUIDs,
		storageInstanceUnitAttachmentByStorageUUID{},
	)
	if err != nil {
		return nil, errors.Errorf(
			"preparing storage instance unit attachments bulk query: %w", err,
		)
	}

	var result []storageInstanceUnitAttachmentByStorageUUID
	err = tx.Query(ctx, stmt, storageUUIDs).GetAll(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	return result, err
}

// GetUnitStorageDirectives returns the storage directives that are set for
// a unit. If the unit does not have any storage directives set then an
// empty result is returned.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
func (st *State) GetUnitStorageDirectives(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]internal.StorageDirective, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitUUIDInput := entityUUID{UUID: unitUUID.String()}
	query, err := st.Prepare(`
SELECT &storageDirective.* FROM (
    SELECT usd.count,
           usd.size_mib,
           usd.storage_name,
           usd.storage_pool_uuid,
           cm.name AS charm_metadata_name,
           csk.kind AS charm_storage_kind,
           cs.count_max AS count_max
    FROM   unit u
    JOIN   unit_storage_directive usd ON usd.unit_uuid = u.uuid AND usd.charm_uuid = u.charm_uuid
    JOIN   charm_storage cs ON cs.charm_uuid = u.charm_uuid AND cs.name = usd.storage_name
    JOIN   charm_metadata cm ON cm.charm_uuid = u.charm_uuid
    JOIN   charm_storage_kind csk ON csk.id = cs.storage_kind_id
    WHERE  u.uuid = $entityUUID.uuid
)
		`,
		unitUUIDInput, storageDirective{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	dbVals := []storageDirective{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID.String())
		if err != nil {
			return errors.Errorf(
				"checking unit %q exists: %w", unitUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"unit %q does not exist", unitUUID,
			).Add(applicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, query, unitUUIDInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]internal.StorageDirective, 0, len(dbVals))
	for _, val := range dbVals {
		rval = append(rval, internal.StorageDirective{
			CharmMetadataName: val.CharmMetadataName,
			Count:             val.Count,
			MaxCount:          val.CountMax,
			Name:              domainstorage.Name(val.StorageName),
			CharmStorageType:  charm.StorageType(val.CharmStorageKind),
			PoolUUID:          domainstorage.StoragePoolUUID(val.StoragePoolUUID),
			Size:              val.SizeMiB,
		})
	}
	return rval, nil
}

// GetUnitStorageDirectiveByName returns the named storage directive that
// is set for a unit.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
// - [applicationerrors.StorageNameNotSupported] if the named storage directive doesn't exist.
func (st *State) GetUnitStorageDirectiveByName(
	ctx context.Context, unitUUID coreunit.UUID, storageName string,
) (internal.StorageDirective, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.StorageDirective{}, errors.Capture(err)
	}

	unitUUIDInput := entityUUID{UUID: unitUUID.String()}
	storageDirectiveInput := storageDirective{StorageName: storageName}
	query, err := st.Prepare(`
SELECT &storageDirective.* FROM (
    SELECT usd.count,
           usd.size_mib,
           usd.storage_name,
           usd.storage_pool_uuid,
           cm.name AS charm_metadata_name,
           csk.kind AS charm_storage_kind,
           cs.count_max AS count_max
    FROM   unit u
    JOIN   unit_storage_directive usd ON usd.unit_uuid = u.uuid AND usd.charm_uuid = u.charm_uuid
    JOIN   charm_storage cs ON cs.charm_uuid = u.charm_uuid AND cs.name = usd.storage_name
    JOIN   charm_metadata cm ON cm.charm_uuid = u.charm_uuid
    JOIN   charm_storage_kind csk ON csk.id = cs.storage_kind_id
    WHERE  u.uuid = $entityUUID.uuid
    AND    usd.storage_name = $storageDirective.storage_name
)
		`,
		unitUUIDInput, storageDirectiveInput,
	)
	if err != nil {
		return internal.StorageDirective{}, errors.Capture(err)
	}

	var dbVal storageDirective
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID.String())
		if err != nil {
			return errors.Errorf(
				"checking unit %q exists: %w", unitUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"unit %q does not exist", unitUUID,
			).Add(applicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, query, unitUUIDInput, storageDirectiveInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.StorageNameNotSupported
		}
		return err
	})

	if err != nil {
		return internal.StorageDirective{}, errors.Capture(err)
	}

	return internal.StorageDirective{
		CharmMetadataName: dbVal.CharmMetadataName,
		Count:             dbVal.Count,
		MaxCount:          dbVal.CountMax,
		Name:              domainstorage.Name(dbVal.StorageName),
		CharmStorageType:  charm.StorageType(dbVal.CharmStorageKind),
		PoolUUID:          domainstorage.StoragePoolUUID(dbVal.StoragePoolUUID),
		Size:              dbVal.SizeMiB,
	}, nil
}

// insertApplicationStorageDirectives inserts all of the storage directives for
// a new application. This func checks to make sure that the caller has supplied
// a directive for each of the storage definitions on the charm.
func (st *State) insertApplicationStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	uuid, charmUUID string,
	directives []domainstorage.DirectiveArg,
) error {
	if len(directives) == 0 {
		return nil
	}

	insertDirectivesInput := make([]insertApplicationStorageDirective, 0, len(directives))
	for _, d := range directives {
		insertDirectivesInput = append(
			insertDirectivesInput,
			insertApplicationStorageDirective{
				ApplicationUUID: uuid,
				CharmUUID:       charmUUID,
				Count:           d.Count,
				Size:            d.Size,
				StorageName:     d.Name.String(),
				StoragePoolUUID: d.PoolUUID.String(),
			},
		)
	}

	insertDirectivesStmt, err := st.Prepare(`
INSERT INTO application_storage_directive (*)
VALUES ($insertApplicationStorageDirective.*)
`,
		insertApplicationStorageDirective{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertDirectivesStmt, insertDirectivesInput).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// insertUnitStorageDirectivesForAllUnits inserts all of the storage directives
// for all the units in an application.
func (st *State) insertUnitStorageDirectivesForAllUnits(
	ctx context.Context,
	tx *sqlair.TX,
	uuid, charmUUID string,
	directives []domainstorage.DirectiveArg,
) error {
	if len(directives) == 0 {
		return nil
	}

	applicationUUID := applicationUUID{
		ApplicationUUID: uuid,
	}
	selectUnitUUIDsStmt, err := st.Prepare(`
SELECT &unitUUID.uuid
FROM   unit u
WHERE  u.application_uuid = $applicationUUID.application_uuid
`, applicationUUID, unitUUID{})
	if err != nil {
		return errors.Errorf("preparing all unit uuids for app query").Add(err)
	}

	var unitUUIDs []unitUUID
	err = tx.Query(ctx, selectUnitUUIDsStmt, applicationUUID).GetAll(&unitUUIDs)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil
	} else if err != nil {
		return errors.Errorf(
			"getting all unit uuids for application %q", uuid,
		).Add(err)
	}

	insertDirectivesInput := make([]insertUnitStorageDirective, 0,
		len(unitUUIDs)*len(directives))
	for _, unit := range unitUUIDs {
		for _, d := range directives {
			insertDirectivesInput = append(
				insertDirectivesInput,
				insertUnitStorageDirective{
					UnitUUID:        unit.UnitUUID,
					CharmUUID:       charmUUID,
					Count:           d.Count,
					Size:            d.Size,
					StorageName:     d.Name.String(),
					StoragePoolUUID: d.PoolUUID.String(),
				},
			)
		}
	}

	insertDirectivesStmt, err := st.Prepare(`
INSERT INTO unit_storage_directive (*)
VALUES ($insertUnitStorageDirective.*)
`, insertUnitStorageDirective{})
	if err != nil {
		return errors.Errorf(
			"preparing insert unit storage directive query",
		).Add(err)
	}

	err = tx.Query(ctx, insertDirectivesStmt, insertDirectivesInput).Run()
	if err != nil {
		return errors.Errorf(
			"inserting unit storage directives for all units in application %q",
			uuid,
		).Add(err)
	}

	return nil
}

// updateApplicationStorageDirectives updates the storage directives and charmUUID
// for an application based on the provided overrides.
// This is used during charm refresh to reconcile storage requirements.
func (st *State) updateApplicationStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.UUID,
	charmUUID string,
	updates []domainstorage.DirectiveArg,
) error {
	if len(updates) == 0 {
		return nil
	}

	updateStmt, err := st.Prepare(`
UPDATE application_storage_directive
SET    count = $updateApplicationStorageDirective.count,
       size_mib = $updateApplicationStorageDirective.size_mib,
	   storage_pool_uuid = $updateApplicationStorageDirective.storage_pool_uuid,
	   charm_uuid = $updateApplicationStorageDirective.charm_uuid
WHERE  application_uuid = $updateApplicationStorageDirective.application_uuid
AND    storage_name = $updateApplicationStorageDirective.storage_name
`, updateApplicationStorageDirective{})
	if err != nil {
		return errors.Capture(err)
	}

	for _, override := range updates {
		input := updateApplicationStorageDirective{
			ApplicationUUID: appUUID.String(),
			CharmUUID:       charmUUID,
			Count:           override.Count,
			SizeMiB:         override.Size,
			StorageName:     override.Name.String(),
			StoragePoolUUID: override.PoolUUID.String(),
		}
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, updateStmt, input).Get(&outcome); err != nil {
			return errors.Errorf("updating storage directives for application: %w", err)
		}
		result := outcome.Result()
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.Errorf("getting number of affected rows: %w", err)
		}
		// We should always have an update if the storage directive is present,
		// since charmUUID should always be updated.
		if affected == 0 {
			return errors.Errorf(
				"missing storage directive for charm storage %q",
				input.StorageName,
			).Add(applicationerrors.MissingStorageDirective)
		}
	}

	return nil
}

func (st *State) setFilesystemProviderIDs(
	ctx context.Context, tx *sqlair.TX,
	providerIDs map[domainstorage.FilesystemUUID]string,
) error {
	stmt, err := st.Prepare(`
UPDATE storage_filesystem
SET    provider_id = $setStorageFilesystemProviderID.provider_id
WHERE  uuid = $setStorageFilesystemProviderID.uuid
`, setStorageFilesystemProviderID{})
	if err != nil {
		return errors.Capture(err)
	}

	for uuid, providerID := range providerIDs {
		input := setStorageFilesystemProviderID{
			UUID:       uuid.String(),
			ProviderID: providerID,
		}
		err := tx.Query(ctx, stmt, input).Run()
		if err != nil {
			return errors.Errorf(
				"setting filesystem %s provider id: %w", uuid, err,
			)
		}
	}

	return nil
}

func (st *State) setFilesystemAttachmentProviderIDs(
	ctx context.Context, tx *sqlair.TX,
	providerIDs map[domainstorage.FilesystemAttachmentUUID]string,
) error {
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_filesystem_attachment
WHERE  uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	stmt, err := st.Prepare(`
UPDATE storage_filesystem_attachment
SET    provider_id = $setStorageFilesystemAttachmentProviderID.provider_id
WHERE  uuid = $setStorageFilesystemAttachmentProviderID.uuid
`, setStorageFilesystemAttachmentProviderID{})
	if err != nil {
		return errors.Capture(err)
	}

	for uuid, providerID := range providerIDs {
		io := entityUUID{
			UUID: uuid.String(),
		}
		err := tx.Query(ctx, existsStmt, io).Get(&io)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"filesystem attachment %s not found", uuid,
			).Add(storageprovisioningerrors.FilesystemAttachmentNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking filesystem attachment %s provider id exists: %w",
				uuid, err,
			)
		}
		input := setStorageFilesystemAttachmentProviderID{
			UUID:       uuid.String(),
			ProviderID: providerID,
		}
		err = tx.Query(ctx, stmt, input).Get()
		if err != nil {
			return errors.Errorf(
				"setting filesystem attachment %s provider id: %w", uuid, err,
			)
		}
	}

	return nil
}

// GetProviderTypeForPool returns the provider type that is in use for the
// given pool.
//
// The following error types can be expected:
// - [storageerrors.StoragePoolNotFound] when no storage pool exists for the
// provided pool uuid.
func (st *State) GetProviderTypeForPool(
	ctx context.Context, poolUUID domainstorage.StoragePoolUUID,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		poolUUIDInput = storagePoolUUID{UUID: poolUUID.String()}
		typeVal       storagePoolType
	)

	providerTypeStmt, err := st.Prepare(`
SELECT &storagePoolType.*
FROM   storage_pool
WHERE  uuid = $storagePoolUUID.uuid
`,
		poolUUIDInput, typeVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, providerTypeStmt, poolUUIDInput).Get(&typeVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage pool %q does not exist", poolUUID,
			).Add(storageerrors.StoragePoolNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return typeVal.Type, nil
}

// makeInsertUnitStorageAttachmentArgs is responsible for making the set of
// storage instance attachment arguments that correspond to the storage uuids.
func makeInsertUnitStorageAttachmentArgs(
	unitUUID string,
	storageToAttach []internal.CreateStorageInstanceAttachmentArg,
) []insertStorageInstanceAttachment {
	rval := make([]insertStorageInstanceAttachment, 0, len(storageToAttach))
	for _, sa := range storageToAttach {
		rval = append(rval, insertStorageInstanceAttachment{
			LifeID:              int(domainlife.Alive),
			StorageInstanceUUID: sa.StorageInstanceUUID.String(),
			UnitUUID:            unitUUID,
			UUID:                sa.UUID.String(),
		})
	}

	return rval
}

// makeInsertUnitStorageOwnerArgs is responsible for making the set of
// storage instance unit owner arguments that correspond to the unit and storage
// instances supplied.
func makeInsertUnitStorageOwnerArgs(
	_ context.Context,
	unitUUID string,
	storageToOwn []domainstorage.StorageInstanceUUID,
) []insertStorageUnitOwner {
	rval := make([]insertStorageUnitOwner, 0, len(storageToOwn))
	for _, instUUID := range storageToOwn {
		rval = append(rval, insertStorageUnitOwner{
			StorageInstanceUUID: instUUID.String(),
			UnitUUID:            unitUUID,
		})
	}

	return rval
}

// makeInsertMachineVolumeOwnerArgs is responsible for making the set of volume
// machine owner arguments that correspond to the machine and volumes supplied.
func makeInsertMachineVolumeOwnerArgs(
	_ context.Context,
	machineUUID coremachine.UUID,
	volumesToOwn []domainstorage.VolumeUUID,
) []insertVolumeMachineOwner {
	rval := make([]insertVolumeMachineOwner, 0, len(volumesToOwn))
	for _, uuid := range volumesToOwn {
		rval = append(rval, insertVolumeMachineOwner{
			MachineUUID: machineUUID.String(),
			VolumeUUID:  uuid.String(),
		})
	}
	return rval
}

// makeInsertMachineFilesystemOwnerArgs is responsible for making the set of
// filesystem machine owner arguments that correspond to the machine and
// filesystems supplied.
func makeInsertMachineFilesystemOwnerArgs(
	_ context.Context,
	machineUUID coremachine.UUID,
	filesystemsToOwn []domainstorage.FilesystemUUID,
) []insertFilesystemMachineOwner {
	rval := make([]insertFilesystemMachineOwner, 0, len(filesystemsToOwn))
	for _, uuid := range filesystemsToOwn {
		rval = append(rval, insertFilesystemMachineOwner{
			MachineUUID:    machineUUID.String(),
			FilesystemUUID: uuid.String(),
		})
	}
	return rval
}

// GetStorageUUIDByID returns the UUID for the specified storage, returning an error
// satisfying [storageerrors.StorageInstanceNotFound] if the storage doesn't exist.
func (st *State) GetStorageUUIDByID(ctx context.Context, storageID corestorage.ID) (domainstorage.StorageInstanceUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	inst := storageInstance{StorageID: storageID}

	query, err := st.Prepare(`
SELECT &storageInstance.*
FROM   storage_instance
WHERE  storage_id = $storageInstance.storage_id
`, inst)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, inst).Get(&inst)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage %q not found", storageID).Add(storageerrors.StorageInstanceNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying storage ID %q: %w", storageID, err)
	}

	return inst.StorageUUID, nil
}

// setStorageInstancesCharmName sets the charm name of the supplied Storage
// Instances. It is assumed that the caller has already validated that the
// Storage Instances exist.
func (st *State) setStorageInstancesCharmName(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.StorageInstanceCharmNameSetArg,
) error {
	if len(args) == 0 {
		// Early exit oppurtunity.
		return nil
	}

	stmt, err := st.Prepare(`
UPDATE storage_instance
SET    charm_name = $setStorageInstanceCharmName.charm_name
WHERE  uuid = $setStorageInstanceCharmName.uuid
`, setStorageInstanceCharmName{})
	if err != nil {
		return errors.Errorf(
			"preparing update storage instances charm name statement: %w", err,
		)
	}

	for _, arg := range args {
		input := setStorageInstanceCharmName{
			CharmName: arg.CharmMetadataName,
			UUID:      arg.UUID.String(),
		}
		err := tx.Query(ctx, stmt, input).Run()
		if err != nil {
			return errors.Errorf(
				"setting storage instance %q charm name: %w",
				arg.UUID,
				err,
			)
		}
	}

	return nil
}

// AttachStorageToUnit attaches an existing storage instance to a unit after
// validating the Storage Instance and Unit preconditions.
//
// The following errors can be expected:
// - [storageerrors.StorageInstanceNotFound] when the storage instance does
// not exist.
// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not
// alive.
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [applicationerrors.UnitNotAlive] when the unit is not alive.
// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the storage
// instance is already attached to the unit.
// - [applicationerrors.UnitAttachmentCountExceedsLimit] when the unit already
// has too many attachments for the storage name.
// - [applicationerrors.UnitCharmChanged] when the unit's charm has changed.
// - [applicationerrors.UnitMachineChanged] when the unit's machine has changed.
// - [applicationerrors.StorageInstanceUnexpectedAttachments] when the Storage
// Instance has attachments outside the expected set.
func (st *State) AttachStorageToUnit(
	ctx context.Context,
	unitUUID coreunit.UUID,
	storageArg internal.AttachStorageInstanceToUnitArg,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	var charmNameSetArgs []internal.StorageInstanceCharmNameSetArg
	if storageArg.StorageInstanceCharmNameSetArg != nil {
		charmNameSetArgs = append(
			charmNameSetArgs, *storageArg.StorageInstanceCharmNameSetArg,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkStorageInstanceExistsAndAlive(
			ctx, tx, storageArg.StorageInstanceUUID,
		)
		if err != nil {
			return err
		}

		err = st.checkUnitForStorageInstanceAttach(
			ctx,
			tx,
			unitUUID,
			storageArg.StorageInstanceUUID,
			storageArg.UnitStorageInstanceAttachmentCheckArgs,
		)
		if err != nil {
			return err
		}

		err = st.checkStorageInstancesAttachmentExpectations(
			ctx,
			tx,
			[]internal.StorageInstanceAttachmentCheckArgs{
				storageArg.StorageInstanceAttachmentCheckArgs,
			},
		)
		if err != nil {
			return err
		}

		err = st.insertUnitStorageAttachments(
			ctx,
			tx,
			unitUUID.String(),
			[]internal.CreateStorageInstanceAttachmentArg{
				storageArg.CreateStorageInstanceAttachmentArg,
			},
		)
		if err != nil {
			return err
		}

		err = st.setStorageInstancesCharmName(ctx, tx, charmNameSetArgs)
		if err != nil {
			return errors.Errorf(
				"updating storage instance %q charm name: %w",
				storageArg.StorageInstanceUUID,
				err,
			)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// checkStorageInstanceExistsAndAlive validates that the storage instance exists
// and is alive.
//
// The following errors can be expected:
// - [storageerrors.StorageInstanceNotFound] when the storage instance does not
// exist.
// - [storageerrors.StorageInstanceNotAlive] when the storage instance is not
// alive.
func (st *State) checkStorageInstanceExistsAndAlive(
	ctx context.Context,
	tx *sqlair.TX,
	uuid domainstorage.StorageInstanceUUID,
) error {
	var (
		lifeVal   entityLife
		inputUUID = entityUUID{UUID: uuid.String()}
	)

	q := "SELECT &entityLife.* FROM storage_instance WHERE uuid = $entityUUID.uuid"
	stmt, err := st.Prepare(q, inputUUID, lifeVal)
	if err != nil {
		return errors.Errorf(
			"preparing storage instance life check query: %w", err,
		)
	}

	err = tx.Query(ctx, stmt, inputUUID).Get(&lifeVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"storage instance %q not found", uuid,
		).Add(storageerrors.StorageInstanceNotFound)
	} else if err != nil {
		return errors.Errorf(
			"checking storage instance %q current life: %w", uuid, err,
		)
	}

	if domainlife.Life(lifeVal.LifeID) != domainlife.Alive {
		return errors.New(
			"storage instance is not alive",
		).Add(storageerrors.StorageInstanceNotAlive)
	}

	return nil
}

// checkStorageInstancesExistAndAlive validates that all supplied storage
// instances exist and are alive.
//
// The following errors can be expected:
//   - [storageerrors.StorageInstanceNotFound] when any storage instance does
//     not exist.
//   - [storageerrors.StorageInstanceNotAlive] when any storage instance is not
//     alive.
func (st *State) checkStorageInstancesExistAndAlive(
	ctx context.Context,
	tx *sqlair.TX,
	uuids []domainstorage.StorageInstanceUUID,
) error {
	if len(uuids) == 0 {
		// Nothing to do
		return nil
	}

	// inputUUIDs is used to feed this function further. The value becomes
	// sorted and de-duplicated.
	inputUUIDs := make(storageInstanceUUIDs, 0, len(uuids))
	inputUUIDs = slices.AppendSeq(inputUUIDs, iter.TransformSeq(
		slices.Values(uuids),
		func(uuid domainstorage.StorageInstanceUUID) string {
			return uuid.String()
		},
	))

	slices.Sort(inputUUIDs)
	inputUUIDs = slices.Compact(inputUUIDs)

	q := `
SELECT &entityUUIDLife.*
FROM storage_instance
WHERE uuid IN ($storageInstanceUUIDs[:])
ORDER BY uuid
`

	stmt, err := st.Prepare(q, inputUUIDs, entityUUIDLife{})
	if err != nil {
		return errors.Errorf("preparing storage instances life query: %w", err)
	}

	dbVals := []entityUUIDLife{}
	err = tx.Query(ctx, stmt, inputUUIDs).GetAll(&dbVals)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("all storage instances not found").Add(
			storageerrors.StorageInstanceNotFound,
		)
	} else if err != nil {
		return errors.Errorf("querying storage instances life: %w", err)
	}

	// Walk over the supplied UUIDs and verify they match the values from the
	// database.
	for i, uuid := range inputUUIDs {
		// If the UUID is not in the database result or does not match the
		// database result the Storage Instance does not exist.
		if i >= len(dbVals) || dbVals[i].UUID != uuid {
			return errors.Errorf(
				"storage instance %q not found", uuid,
			).Add(storageerrors.StorageInstanceNotFound)
		}

		// If the life of the Storage Instance is not alive, return an error.
		if domainlife.Life(dbVals[i].LifeID) != domainlife.Alive {
			return errors.Errorf(
				"storage instance %q is not alive", uuid,
			).Add(storageerrors.StorageInstanceNotAlive)
		}
	}

	return nil
}

// checkStorageInstancesAttachmentExpectations validates that a set of storage
// instances have the expected storage attachments.
//
// Instances with no expected attachments are checked for having no
// attachments. Instances with expected attachments are checked for having the
// expected attachments and no unexpected attachments.
//
// The caller must make sure that the same Storage Instance UUID is not used
// more then once in args.
//
// The following errors can be expected:
//   - [storageerrors.StorageInstanceNotFound] when a storage instance UUID
//     supplied in args cannot be found in the model.
//   - [applicationerrors.StorageInstanceUnexpectedAttachments] when a storage
//     instance has attachments outside
//     [internal.StorageInstanceAttachmentCheckArgs.ExpectedAttachments] or is
//     missing expected attachments.
func (st *State) checkStorageInstancesAttachmentExpectations(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.StorageInstanceAttachmentCheckArgs,
) error {
	if len(args) == 0 {
		// Nothing to do.
		return nil
	}

	// Sort args based on StorageInstance UUID so that the the db results match
	// the order of the args.
	slices.SortFunc(args, func(a, b internal.StorageInstanceAttachmentCheckArgs) int {
		return strings.Compare(a.UUID.String(), b.UUID.String())
	})

	var (
		expectedAttachmentUUIDs storageAttachmentUUIDs
		expectedSIUUIDs         storageInstanceUUIDs
		unexpectedSIUUIDs       storageInstanceUUIDs
	)

	// We need to pack args into [expectedAttachmentUUIDs],
	// [expectedStorageInstanceUUIDs] & [unexpectedStorageInstanceUUIDs].
	// If a check has no expected attachments it goes into the unexpected list.
	// Otherwise, it goes into the expected list.
	for _, arg := range args {
		if len(arg.ExpectedAttachments) == 0 {
			unexpectedSIUUIDs = append(unexpectedSIUUIDs, arg.UUID.String())
			continue
		}

		expectedSIUUIDs = append(expectedSIUUIDs, arg.UUID.String())
		// Pre-allocated expectedAttachmentUUIDs to avoid reallocation.
		expectedAttachmentUUIDs = slices.Grow(
			expectedAttachmentUUIDs, len(arg.ExpectedAttachments),
		)
		// Transform each StorageAttachmentUUID to a string and append to
		// [expectedAttachmentUUIDs].
		expectedAttachmentUUIDs = slices.AppendSeq(
			expectedAttachmentUUIDs,
			iter.TransformSeq(
				slices.Values(arg.ExpectedAttachments),
				func(uuid domainstorage.StorageAttachmentUUID) string {
					return uuid.String()
				},
			),
		)
	}

	queryStorageInstanceAttachmentUnexpectedCount := `
SELECT &storageInstanceAttachmentCheckCount.*
FROM (
    SELECT    si.uuid AS storage_instance_uuid,
              0 as expected_count,
              COUNT(sa.uuid) AS unexpected_count
    FROM      storage_instance si
    LEFT JOIN storage_attachment sa ON sa.storage_instance_uuid = si.uuid
    WHERE     si.uuid IN ($storageInstanceUUIDs[:])
    GROUP BY  si.uuid
)
`

	queryStorageInstanceAttachmentExpectedCount := `
SELECT &storageInstanceAttachmentCheckCount.*
FROM (
    SELECT    si.uuid AS storage_instance_uuid,
              COUNT(sae.uuid) AS expected_count,
              COUNT(sau.uuid) AS unexpected_count
    FROM      storage_instance si
    LEFT JOIN storage_attachment sa  ON sa.storage_instance_uuid = si.uuid
    LEFT JOIN storage_attachment sae ON sae.uuid = sa.uuid
                                    AND sae.uuid IN ($storageAttachmentUUIDs[:])
    LEFT JOIN storage_attachment sau ON sau.uuid = sa.uuid
                                    AND sau.uuid NOT IN ($storageAttachmentUUIDs[:])
    WHERE     si.uuid IN ($storageInstanceUUIDs[:])
    GROUP BY  si.uuid
)
`

	stmtUnexpected, err := st.Prepare(
		queryStorageInstanceAttachmentUnexpectedCount,
		storageInstanceAttachmentCheckCount{},
		unexpectedSIUUIDs,
	)
	if err != nil {
		return errors.Errorf(
			"preparing storage instance unexpected attachments check query: %w",
			err,
		)
	}

	stmtExpected, err := st.Prepare(
		queryStorageInstanceAttachmentExpectedCount,
		storageInstanceAttachmentCheckCount{},
		expectedAttachmentUUIDs,
		expectedSIUUIDs,
	)
	if err != nil {
		return errors.Errorf(
			"preparing storage instance expected attachments check query: %w",
			err,
		)
	}

	expectedVals := []storageInstanceAttachmentCheckCount{}
	unexpectedVals := []storageInstanceAttachmentCheckCount{}

	if len(unexpectedSIUUIDs) != 0 {
		err = tx.Query(
			ctx,
			stmtUnexpected,
			unexpectedSIUUIDs,
		).GetAll(&unexpectedVals)
		// We don't need to handle a no rows error. Performed below.
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"checking storage instances unexpected attachments: %w", err,
			)
		}
	}
	if len(expectedSIUUIDs) != 0 {
		err = tx.Query(
			ctx,
			stmtExpected,
			expectedAttachmentUUIDs,
			expectedSIUUIDs,
		).GetAll(&expectedVals)
		// We don't need to handle a no rows error. Performed below.
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"checking storage instances expected attachments: %w", err,
			)
		}
	}

	// Merge the two results together and sort based on storage instance uuid.
	// This has to be done this way instead of SQL due to limitations in sqlair.
	expectedVals = slices.Grow(expectedVals, len(unexpectedVals))
	groupedVals := append(expectedVals, unexpectedVals...)
	slices.SortFunc(groupedVals, func(a, b storageInstanceAttachmentCheckCount) int {
		return strings.Compare(a.StorageInstanceUUID, b.StorageInstanceUUID)
	})

	for i, arg := range args {
		// if we have less values in groupedVals than args we have hit the end
		// of the road and something is missing.
		if len(groupedVals) <= i {
			return errors.Errorf(
				"storage instance %q not found", arg.UUID,
			).Add(storageerrors.StorageInstanceNotFound)
		}

		if arg.UUID.String() != groupedVals[i].StorageInstanceUUID {
			return errors.Errorf(
				"storage instance %q not found", arg.UUID,
			).Add(storageerrors.StorageInstanceNotFound)
		}
		if groupedVals[i].ExpectedCount != len(args[i].ExpectedAttachments) {
			return errors.Errorf(
				"storage instance %q missing %d expected attachments",
				arg.UUID,
				len(args[i].ExpectedAttachments)-groupedVals[i].ExpectedCount,
			).Add(applicationerrors.StorageInstanceUnexpectedAttachments)
		}
		if groupedVals[i].UnexpectedCount > 0 {
			return errors.Errorf(
				"storage instance %q has %d unexpected attachments",
				arg.UUID,
				groupedVals[i].UnexpectedCount,
			).Add(applicationerrors.StorageInstanceUnexpectedAttachments)
		}
	}

	return nil
}

// checkUnitForStorageInstanceAttach validates that the unit is in a suitable
// state to attach the specified storage instance. It is expected that the
// caller has validated that the Storage Instance exists and is alive.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [applicationerrors.UnitNotAlive] when the unit is not alive.
// - [applicationerrors.StorageInstanceAlreadyAttachedToUnit] when the storage
// instance is already attached to the unit.
// - [applicationerrors.UnitAttachmentCountExceedsLimit] when the unit already
// has too many attachments for the storage name.
// - [applicationerrors.UnitCharmChanged] when the unit's charm has changed.
// - [applicationerrors.UnitMachineChanged] when the unit's machine has changed.
func (st *State) checkUnitForStorageInstanceAttach(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	siUUID domainstorage.StorageInstanceUUID,
	args internal.UnitStorageInstanceAttachmentCheckArgs,
) error {
	var (
		checkResult              unitAttachStorageInstanceCheckInfo
		inputUnitUUID            = entityUUID{UUID: unitUUID.String()}
		inputStorageInstanceUUID = storageInstanceUUID{
			UUID: siUUID.String(),
		}
	)

	q := `
SELECT &unitAttachStorageInstanceCheckInfo.*
FROM (
    SELECT    u.charm_uuid AS unit_charm_uuid,
              u.life_id AS unit_life_id,
              m.uuid AS unit_machine_uuid,
              
              -- Calculate how many Storage Instance attachments the unit has
              -- for the same storage name of the Storage Instance being
              -- attached.
              (SELECT COUNT(*)
               FROM storage_attachment sa
               JOIN storage_instance si ON sa.storage_instance_uuid = si.uuid
               WHERE sa.unit_uuid = $entityUUID.uuid
               AND   si.storage_name = (SELECT storage_name
                 				         FROM storage_instance
                                        WHERE uuid = $storageInstanceUUID.uuid)
              ) AS unit_attachment_count,
              
              -- Calculate if the Storage Instance is already attached to the
              -- unit.
              (SELECT 1
               FROM storage_attachment sa
               WHERE sa.unit_uuid = $entityUUID.uuid
               AND   sa.storage_instance_uuid = $storageInstanceUUID.uuid
              ) AS already_attached
    FROM      unit u
    LEFT JOIN machine m ON u.net_node_uuid = m.net_node_uuid
    WHERE     u.uuid = $entityUUID.uuid
)
`
	stmt, err := st.Prepare(
		q, inputUnitUUID, inputStorageInstanceUUID, checkResult,
	)
	if err != nil {
		return errors.Errorf(
			"preparing query to get check unit storage instance attachment info: %w",
			err,
		)
	}

	err = tx.Query(ctx, stmt, inputUnitUUID, inputStorageInstanceUUID).Get(
		&checkResult,
	)
	if errors.Is(err, sqlair.ErrNoRows) {
		// If we received no result from the query then the unit does not exist.
		return errors.Errorf(
			"unit %q does not exist", unitUUID,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return err
	}

	if domainlife.Life(checkResult.UnitLifeID) != domainlife.Alive {
		return errors.New("unit is not alive").Add(applicationerrors.UnitNotAlive)
	}

	if checkResult.AlreadyAttached {
		return errors.New("storage instance is already attached to unit").Add(
			applicationerrors.StorageInstanceAlreadyAttachedToUnit,
		)
	}

	if checkResult.UnitAttachmentCount > args.CountLessThanEqual {
		return errors.New("unit attachment count exceeds limit").Add(
			applicationerrors.UnitAttachmentCountExceedsLimit,
		)
	}

	if checkResult.UnitCharmUUID != args.CharmUUID.String() {
		return errors.New("unit's charm has changed").Add(
			applicationerrors.UnitCharmChanged,
		)
	}

	var checkMachineUUID string
	if args.MachineUUID != nil {
		checkMachineUUID = args.MachineUUID.String()
	}
	unitMachineUUID := checkResult.MachineUUID.V

	if checkMachineUUID != unitMachineUUID {
		return errors.New("unit's machine has changed").Add(
			applicationerrors.UnitMachineChanged,
		)
	}

	return nil
}

func (st *State) addStorageForUnit(
	ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID,
	storageName corestorage.Name, storageArg domainstorage.UnitAddStorageArg,
) ([]string, error) {
	// First to the basic life check for the unit.
	unitLifeID, _, err := st.getUnitLifeAndNetNode(ctx, tx, unitUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}
	if unitLifeID != domainlife.Alive {
		return nil, errors.Errorf("unit %q is not alive", unitUUID).Add(applicationerrors.UnitNotAlive)
	}

	// Ensure another update hasn't violated our preconditions.
	currentCount, err := st.getUnitStorageCount(ctx, tx, unitUUID, storageName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if currentCount > storageArg.CountLessThanEqual {
		return nil, storageerrors.MaxStorageCountPreconditionFailed
	}

	storageIDs, err := st.insertUnitStorageInstances(
		ctx, tx, storageArg.StorageInstances,
	)
	if err != nil {
		return nil, errors.Errorf(
			"inserting storage instances for unit %q: %w", unitUUID, err,
		)
	}

	err = st.insertUnitStorageAttachments(
		ctx,
		tx,
		unitUUID.String(),
		storageArg.NewStorageToAttach,
	)
	if err != nil {
		return nil, errors.Errorf(
			"creating storage attachments for unit %q: %w", unitUUID, err,
		)
	}

	err = st.insertUnitStorageOwnership(ctx, tx, unitUUID.String(), storageArg.StorageToOwn)
	if err != nil {
		return nil, errors.Errorf(
			"inserting storage ownership for unit %q: %w", unitUUID, err,
		)
	}
	return storageIDs, nil
}

// AddStorageForCAASUnit adds storage instances to given CAAS unit as specified.
func (st *State) AddStorageForCAASUnit(
	ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
	storageArg domainstorage.UnitAddStorageArg,
) ([]corestorage.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var storageIDs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageIDs, err = st.addStorageForUnit(ctx, tx, unitUUID, storageName, storageArg)
		if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	result := make([]corestorage.ID, len(storageIDs))
	for i, storageID := range storageIDs {
		result[i] = corestorage.ID(storageID)
	}

	return result, nil
}

// AddStorageForIAASUnit adds storage instances to given IAAS unit as specified.
func (st *State) AddStorageForIAASUnit(
	ctx context.Context, unitUUID coreunit.UUID, storageName corestorage.Name,
	storageArg domainstorage.IAASUnitAddStorageArg,
) ([]corestorage.ID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineUUID, err := st.GetUnitMachineUUID(ctx, unitUUID.String())
	if err != nil {
		return nil, errors.Capture(err)
	}

	var storageIDs []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		storageIDs, err = st.addStorageForUnit(ctx, tx, unitUUID, storageName, storageArg.AddStorageToUnitArg)
		if err != nil {
			return errors.Capture(err)
		}

		err = st.insertMachineVolumeOwnership(ctx, tx, coremachine.UUID(machineUUID),
			storageArg.VolumesToOwn)
		if err != nil {
			return errors.Errorf(
				"inserting volume ownership for machine %q: %w",
				machineUUID, err,
			)
		}

		err = st.insertMachineFilesystemOwnership(ctx, tx, coremachine.UUID(machineUUID),
			storageArg.FilesystemsToOwn)
		if err != nil {
			return errors.Errorf(
				"inserting volume ownership for machine %q: %w",
				machineUUID, err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]corestorage.ID, len(storageIDs))
	for i, storageID := range storageIDs {
		result[i] = corestorage.ID(storageID)
	}

	return result, nil
}

// getUnitStorageNameInfo returns charm storage definition metadata for the
// named storage on the unit, along with how many instances of that storage are
// already attached to the unit.
//
// It is expected that the caller has already verified the unit exists.
//
// The following errors can be expected:
// - [applicationerrors.StorageNameNotSupported] when the unit's charm does not
// define the named storage.
func (st *State) getUnitStorageNameInfo(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	storageDefName string,
) (unitStorageNameInfo, error) {

	var (
		inUUID             = entityUUID{UUID: unitUUID.String()}
		inCharmStorageName = charmStorageName{Name: storageDefName}
		out                unitStorageNameInfo
	)

	/*
		id	parent	notused	detail
		10	0	    46	    SEARCH u USING INDEX sqlite_autoindex_unit_1 (uuid=?)
		15	0	    47	    SEARCH cs USING INDEX sqlite_autoindex_charm_storage_1 (charm_uuid=? AND name=?)
		21	0	    46	    SEARCH csk USING INDEX sqlite_autoindex_charm_storage_kind_1 (id=?)
		27	0	    46	    SEARCH m USING INDEX idx_machine_net_node (net_node_uuid=?) LEFT-JOIN
		39	0	    0	    SCALAR SUBQUERY 1
		47	39	    62	    SEARCH sa USING INDEX idx_storage_attachment_unit (unit_uuid=?)
		52	39	    46	    SEARCH si USING INDEX sqlite_autoindex_storage_instance_1 (uuid=?)
	*/
	q := `
SELECT * AS &unitStorageNameInfo.* FROM (
    SELECT    m.uuid AS machine_uuid,
              u.life_id as unit_life_id,
              u.name AS unit_name,
              u.uuid AS unit_uuid,
              u.net_node_uuid AS unit_net_node_uuid,
              cm.name AS charm_metadata_name,
              u.charm_uuid AS unit_charm_uuid,
              (SELECT count(*)
               FROM storage_attachment sa
               JOIN storage_instance si ON sa.storage_instance_uuid = si.uuid
               WHERE si.storage_name = $charmStorageName.name
               AND   sa.unit_uuid = $entityUUID.uuid) AS already_attached_count,
               
              cs.count_max AS storage_definition_count_max,
              cs.count_min AS storage_definition_count_min,
              csk.kind AS storage_definition_kind,
              cs.name AS storage_definition_name,
              cs.minimum_size_mib AS storage_definition_minimum_size_mib,
              cs.read_only AS storage_definition_read_only,
              cs.shared AS storage_definition_shared
    FROM      unit u
    JOIN      charm_metadata cm ON u.charm_uuid = cm.charm_uuid
    JOIN      charm_storage cs ON u.charm_uuid = cs.charm_uuid
    JOIN      charm_storage_kind csk ON cs.storage_kind_id = csk.id
    LEFT JOIN machine m ON u.net_node_uuid = m.net_node_uuid
    WHERE     u.uuid = $entityUUID.uuid
    AND       cs.name = $charmStorageName.name
)
`

	stmt, err := st.Prepare(q, inUUID, inCharmStorageName, out)
	if err != nil {
		return unitStorageNameInfo{}, errors.Errorf(
			"preparing unit charm storage definition name query: %w", err,
		)
	}

	err = tx.Query(ctx, stmt, inUUID, inCharmStorageName).Get(&out)
	if errors.Is(err, sqlair.ErrNoRows) {
		return unitStorageNameInfo{}, errors.Errorf(
			"storage %q is not found for unit %q charm",
			storageDefName, unitUUID,
		).Add(applicationerrors.StorageNameNotSupported)
	}

	return out, err
}

func (st *State) getUnitStorageCount(
	ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, storageName corestorage.Name,
) (uint32, error) {
	countQuery, err := st.Prepare(`
SELECT count(*) AS &storageCount.count
FROM   storage_instance si
JOIN   storage_unit_owner suo ON si.uuid = suo.storage_instance_uuid
WHERE  suo.unit_uuid = $storageCount.unit_uuid
AND    si.storage_name = $storageCount.storage_name
`, storageCount{})
	if err != nil {
		return 0, errors.Capture(err)
	}

	storageCount := storageCount{StorageName: storageName, UnitUUID: unitUUID}
	err = tx.Query(ctx, countQuery, storageCount).Get(&storageCount)
	if err != nil {
		return 0, errors.Errorf("querying storage count for storage %q on unit %q: %w", storageName, unitUUID, err)
	}
	return storageCount.Count, nil
}

func (st *State) DetachStorageForUnit(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID) error {
	// TODO implement me
	return errors.New("not implemented")
}

func (st *State) DetachStorage(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID) error {
	// TODO implement me
	return errors.New("not implemented")
}

func (st *State) checkStorageInstanceExists(
	ctx context.Context,
	tx *sqlair.TX,
	storageUUID string,
) (bool, error) {
	uuidInput := entityUUID{UUID: storageUUID}

	checkStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_instance
WHERE  uuid = $entityUUID.uuid
	`,
		uuidInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, uuidInput).Get(&uuidInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// getStorageInstanceInfoForAttach returns metadata required to validate and
// attach an existing storage instance, including linked filesystem or volume
// details and any owning machine identifiers.
//
// The following errors can be expected:
// - [storageerrors.StorageInstanceNotFound] when the storage instance does not exist.
func (st *State) getStorageInstanceInfoForAttach(
	ctx context.Context, tx *sqlair.TX,
	storageUUID domainstorage.StorageInstanceUUID,
) (storageInstanceInfoForAttach, error) {
	exists, err := st.checkStorageInstanceExists(ctx, tx, storageUUID.String())
	if err != nil {
		return storageInstanceInfoForAttach{}, errors.Errorf(
			"checking storage instance %q exists: %w", storageUUID, err,
		)
	}
	if !exists {
		return storageInstanceInfoForAttach{}, errors.Errorf("storage instance %q does not exist", storageUUID).Add(
			storageerrors.StorageInstanceNotFound,
		)
	}

	var (
		inUUID = entityUUID{UUID: storageUUID.String()}
		siInfo storageInstanceInfoForAttach
	)

	query := `
SELECT * AS &storageInstanceInfoForAttach.* FROM (
    SELECT    si.uuid,
              si.charm_name,
              si.storage_name,
              si.life_id,
              si.requested_size_mib,
              si.storage_kind_id,
              sif.storage_filesystem_uuid AS filesystem_uuid,
              sf.size_mib AS filesystem_size_mib,
              sf.provision_scope_id AS filesystem_provision_scope_id,
              mf.machine_uuid AS filesystem_owned_machine_uuid,
              siv.storage_volume_uuid AS volume_uuid,
              sv.size_mib AS volume_size_mib,
              sv.provision_scope_id AS volume_provision_scope_id,
              mv.machine_uuid AS volume_owned_machine_uuid
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN machine_filesystem mf ON sif.storage_filesystem_uuid = mf.filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    LEFT JOIN machine_volume mv ON siv.storage_volume_uuid = mv.volume_uuid
    WHERE     si.uuid = $entityUUID.uuid
)
`
	queryStmt, err := st.Prepare(query, inUUID, siInfo)
	if err != nil {
		return storageInstanceInfoForAttach{}, errors.Errorf(
			"preparing query for getting storage instance info for attachment: %w",
			err,
		)
	}

	err = tx.Query(ctx, queryStmt, inUUID).Get(&siInfo)
	if errors.Is(err, sqlair.ErrNoRows) {
		return storageInstanceInfoForAttach{}, errors.Errorf(
			"storage instance %q does not exist", storageUUID,
		).Add(storageerrors.StorageInstanceNotFound)
	}

	return siInfo, err
}

// getStorageInstancesInfoForAttach returns attach metadata for the supplied
// storage instances, ordered by storage instance UUID.
//
// This helper performs no existence validation for the supplied storage
// instance UUIDs. Callers are responsible for verifying all requested storage
// instances are present.
func (st *State) getStorageInstancesInfoForAttach(
	ctx context.Context,
	tx *sqlair.TX,
	storageUUIDs storageInstanceUUIDs,
) ([]storageInstanceInfoForAttach, error) {
	if len(storageUUIDs) == 0 {
		return nil, nil
	}

	query := `
SELECT * AS &storageInstanceInfoForAttach.* FROM (
    SELECT    si.uuid,
              si.charm_name,
              si.storage_name,
              si.life_id,
              si.requested_size_mib,
              si.storage_kind_id,
              sif.storage_filesystem_uuid AS filesystem_uuid,
              sf.size_mib AS filesystem_size_mib,
              sf.provision_scope_id AS filesystem_provision_scope_id,
              mf.machine_uuid AS filesystem_owned_machine_uuid,
              siv.storage_volume_uuid AS volume_uuid,
              sv.size_mib AS volume_size_mib,
              sv.provision_scope_id AS volume_provision_scope_id,
              mv.machine_uuid AS volume_owned_machine_uuid
    FROM      storage_instance si
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN machine_filesystem mf ON sif.storage_filesystem_uuid = mf.filesystem_uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    LEFT JOIN machine_volume mv ON siv.storage_volume_uuid = mv.volume_uuid
    WHERE     si.uuid IN ($storageInstanceUUIDs[:])
    ORDER BY  si.uuid
)
`
	queryStmt, err := st.Prepare(query, storageUUIDs, storageInstanceInfoForAttach{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing query for getting storage instances info for attachment: %w",
			err,
		)
	}

	var siInfos []storageInstanceInfoForAttach
	err = tx.Query(ctx, queryStmt, storageUUIDs).GetAll(&siInfos)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf(
			"getting storage instances information for attachment: %w", err,
		)
	}

	return siInfos, nil
}

func (st *State) getStorageInstanceInfoForAdd(
	ctx context.Context, tx *sqlair.TX, uuid coreunit.UUID, name corestorage.Name,
) (storageInfoForAdd, error) {
	storageSpec := unitCharmStorage{
		UnitUUID:    uuid,
		StorageName: name,
	}
	var result storageInfoForAdd
	stmt, err := st.Prepare(`
SELECT cs.* AS &storageInfoForAdd.*
FROM   v_charm_storage cs
JOIN   unit ON unit.charm_uuid = cs.charm_uuid
WHERE  unit.uuid = $unitCharmStorage.uuid
AND    cs.name = $unitCharmStorage.name
`, storageSpec, result)
	if err != nil {
		return result, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, storageSpec).Get(&result); err != nil {
		return result, errors.Errorf("failed to select charm storage: %w", err)
	}

	return result, nil
}

// GetModelStoragePools returns the default storage pools
// that have been set for the model.
func (st *State) GetModelStoragePools(
	ctx context.Context,
) (internal.ModelStoragePools, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return internal.ModelStoragePools{}, errors.Capture(err)
	}

	storageModelConfigKeys := storageModelConfigKeys{
		BlockDeviceKey: application.StorageDefaultBlockSourceKey,
		FilesystemKey:  application.StorageDefaultFilesystemSourceKey,
	}

	modelConfigPools, err := st.Prepare(`
WITH 	blockdevice_pool_name AS (
            SELECT sk.id AS storage_kind_id,
                   value AS name
            FROM   model_config mc,
                   storage_kind sk
            WHERE  key=$storageModelConfigKeys.blockdevice_key
            AND    sk.kind = 'block'
		),
		filesystem_pool_name AS (
            SELECT sk.id AS storage_kind_id,
                   value AS name
            FROM   model_config mc,
                   storage_kind sk
            WHERE  key=$storageModelConfigKeys.filesystem_key
            AND    sk.kind = 'filesystem'
		),
		mc_pools AS (
            SELECT bpn.storage_kind_id,
                   sp.uuid AS storage_pool_uuid
            FROM   blockdevice_pool_name bpn
            JOIN   storage_pool sp ON bpn.name=sp.name
            UNION
            SELECT fpn.storage_kind_id,
                   sp.uuid AS storage_pool_uuid
            FROM   filesystem_pool_name fpn
            JOIN   storage_pool sp ON fpn.name=sp.name
		)
SELECT &modelStoragePools.* FROM (
    SELECT storage_kind_id,
           storage_pool_uuid
    FROM   mc_pools
    UNION
    SELECT storage_kind_id,
           storage_pool_uuid
    FROM   model_storage_pool
    WHERE  storage_kind_id NOT IN (SELECT storage_kind_id
                                   FROM   mc_pools)
)
`, storageModelConfigKeys, modelStoragePools{})
	if err != nil {
		return internal.ModelStoragePools{}, errors.Capture(err)
	}

	var dbVals []modelStoragePools
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, modelConfigPools, storageModelConfigKeys).GetAll(&dbVals)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return internal.ModelStoragePools{}, errors.Capture(err)
	}

	rval := internal.ModelStoragePools{}
	for _, v := range dbVals {
		switch v.StorageKindID {
		case int(domainstorage.StorageKindBlock):
			poolUUID := domainstorage.StoragePoolUUID(v.StoragePoolUUID)
			rval.BlockDevicePoolUUID = &poolUUID
		case int(domainstorage.StorageKindFilesystem):
			poolUUID := domainstorage.StoragePoolUUID(v.StoragePoolUUID)
			rval.FilesystemPoolUUID = &poolUUID
		}
	}
	return rval, nil
}
