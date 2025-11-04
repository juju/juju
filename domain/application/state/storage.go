// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/life"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// These consts are the sequence namespaces used to generate
// monotonically increasing ints to use for storage entity IDs.
const (
	filesystemNamespace = domainsequence.StaticNamespace("filesystem")
	volumeNamespace     = domainsequence.StaticNamespace("volume")
	storageNamespace    = domainsequence.StaticNamespace("storage")
)

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
) ([]application.StorageDirective, error) {
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

	rval := make([]application.StorageDirective, 0, len(dbVals))
	for _, val := range dbVals {
		rval = append(rval, application.StorageDirective{
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

// GetStorageInstancesForProviderIDs returns all of the storage instances
// found in the model using one of the provider ids supplied. The storage
// instance must also not be owned by a unit. If no storage instances are found
// then an empty result is returned.
func (st *State) GetStorageInstancesForProviderIDs(
	ctx context.Context,
	ids []string,
) ([]internal.StorageInstanceComposition, error) {
	// Early exit if no ids are supplied. We cannot have empty values with an
	// IN expression.
	if len(ids) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	providerIDsInput := storageProviderIDs(ids)

	/*
		id	parent	notused	detail
		15	0     	0      	SCAN si USING INDEX sqlite_autoindex_storage_instance_1
		19	0     	0      	USING INDEX sqlite_autoindex_storage_unit_owner_1 FOR IN-OPERATOR
		29	0     	0      	SEARCH sif USING INDEX sqlite_autoindex_storage_instance_filesystem_1 (storage_instance_uuid=?) LEFT-JOIN
		36	0     	0      	SEARCH sf USING INDEX sqlite_autoindex_storage_filesystem_1 (uuid=?) LEFT-JOIN
		44	0     	0      	SEARCH siv USING INDEX sqlite_autoindex_storage_instance_volume_1 (storage_instance_uuid=?) LEFT-JOIN
		51	0     	0      	SEARCH sv USING INDEX sqlite_autoindex_storage_volume_1 (uuid=?) LEFT-JOIN
	*/
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
    WHERE     si.uuid NOT IN (SELECT storage_instance_uuid
                              FROM storage_unit_owner)
    AND	     (sf.provider_id IN ($storageProviderIDs[:])
           OR sv.provider_id IN ($storageProviderIDs[:]))
)
`

	stmt, err := st.Prepare(
		compositionQ,
		providerIDsInput,
		storageInstanceComposition{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageInstanceComposition
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, providerIDsInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]internal.StorageInstanceComposition, 0, len(dbVals))
	for _, dbVal := range dbVals {
		v := internal.StorageInstanceComposition{
			StorageName: domainstorage.Name(dbVal.StorageName),
			UUID:        domainstorage.StorageInstanceUUID(dbVal.UUID),
		}

		if dbVal.FilesystemUUID.Valid {
			v.Filesystem = &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorageprov.ProvisionScope(dbVal.FilesystemProvisionScope.V),
				UUID:           domainstorageprov.FilesystemUUID(dbVal.FilesystemUUID.V),
			}
		}

		if dbVal.VolumeUUID.Valid {
			v.Volume = &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorageprov.ProvisionScope(dbVal.VolumeProvisionScope.V),
				UUID:           domainstorageprov.VolumeUUID(dbVal.VolumeUUID.V),
			}
		}

		rval = append(rval, v)
	}

	return rval, nil
}

// GetUnitOwnedStorageInstances returns the storage instance compositions
// for all storage instances owned by the unit in the model. If the unit
// does not currently own any storage instances then an empty result is
// returned.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit no longer exists.
func (st *State) GetUnitOwnedStorageInstances(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]internal.StorageInstanceComposition, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuidInput := entityUUID{UUID: unitUUID.String()}

	compositionQ := `
SELECT &storageInstanceComposition.*
FROM (
    SELECT    sf.uuid AS filesystem_uuid,
              sf.provision_scope_id AS filesystem_provision_scope,
              si.storage_name AS storage_name,
              si.uuid AS uuid,
              sv.uuid AS volume_uuid,
              sv.provision_scope_id AS volume_provision_scope
    FROM      storage_unit_owner suo
    JOIN      storage_instance si ON suo.storage_instance_uuid = si.uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid = sif.storage_instance_uuid
    LEFT JOIN storage_filesystem sf ON sif.storage_filesystem_uuid = sf.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    LEFT JOIN storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    WHERE     suo.unit_uuid = $entityUUID.uuid
)
`

	stmt, err := st.Prepare(compositionQ, uuidInput, storageInstanceComposition{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageInstanceComposition
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID)
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

		err = tx.Query(ctx, stmt, uuidInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make([]internal.StorageInstanceComposition, 0, len(dbVals))
	for _, dbVal := range dbVals {
		v := internal.StorageInstanceComposition{
			StorageName: domainstorage.Name(dbVal.StorageName),
			UUID:        domainstorage.StorageInstanceUUID(dbVal.UUID),
		}

		if dbVal.FilesystemUUID.Valid {
			v.Filesystem = &internal.StorageInstanceCompositionFilesystem{
				ProvisionScope: domainstorageprov.ProvisionScope(dbVal.FilesystemProvisionScope.V),
				UUID:           domainstorageprov.FilesystemUUID(dbVal.FilesystemUUID.V),
			}
		}

		if dbVal.VolumeUUID.Valid {
			v.Volume = &internal.StorageInstanceCompositionVolume{
				ProvisionScope: domainstorageprov.ProvisionScope(dbVal.VolumeProvisionScope.V),
				UUID:           domainstorageprov.VolumeUUID(dbVal.VolumeUUID.V),
			}
		}

		rval = append(rval, v)
	}
	return rval, nil
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
) ([]application.StorageDirective, error) {
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
    FROM   unit_storage_directive usd
    JOIN   charm_storage cs ON cs.charm_uuid = usd.charm_uuid AND cs.name = usd.storage_name
    JOIN   charm_metadata cm ON cm.charm_uuid = usd.charm_uuid
    JOIN   charm_storage_kind csk ON csk.id = cs.storage_kind_id
    WHERE  unit_uuid = $entityUUID.uuid
)
		`,
		unitUUIDInput, storageDirective{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	dbVals := []storageDirective{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID)
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

	rval := make([]application.StorageDirective, 0, len(dbVals))
	for _, val := range dbVals {
		rval = append(rval, application.StorageDirective{
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

// insertApplicationStorageDirectives inserts all of the storage directives for
// a new application. This func checks to make sure that the caller has supplied
// a directive for each of the storage definitions on the charm.
func (st *State) insertApplicationStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coreapplication.UUID,
	charmUUID corecharm.ID,
	directives []internal.CreateApplicationStorageDirectiveArg,
) error {
	if len(directives) == 0 {
		return nil
	}

	insertDirectivesInput := make([]insertApplicationStorageDirective, 0, len(directives))
	for _, d := range directives {
		insertDirectivesInput = append(
			insertDirectivesInput,
			insertApplicationStorageDirective{
				ApplicationUUID: uuid.String(),
				CharmUUID:       charmUUID.String(),
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

// insertUnitStorageAttachments is responsible for creating all of the unit
// storage attachments for the supplied storage instance uuids. This func will
// also create storage attachments for each filesystem and volume
func (st *State) insertUnitStorageAttachments(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	storageToAttach []internal.CreateUnitStorageAttachmentArg,
) error {
	storageAttachmentArgs := makeInsertUnitStorageAttachmentArgs(
		ctx, unitUUID, storageToAttach,
	)

	fsAttachmentArgs := st.makeInsertUnitFilesystemAttachmentArgs(
		storageToAttach,
	)

	volAttachmentArgs := st.makeInsertUnitVolumeAttachmentArgs(
		storageToAttach,
	)

	insertStorageAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($insertStorageInstanceAttachment.*)
`,
		insertStorageInstanceAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertFSAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*)
VALUES ($insertStorageFilesystemAttachment.*)
`,
		insertStorageFilesystemAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertVolAttachmentStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*)
VALUES ($insertStorageVolumeAttachment.*)
`,
		insertStorageVolumeAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(storageAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertStorageAttachmentStmt, storageAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create storage attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	if len(fsAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertFSAttachmentStmt, fsAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create filesystem attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	if len(volAttachmentArgs) != 0 {
		err := tx.Query(ctx, insertVolAttachmentStmt, volAttachmentArgs).Run()
		if err != nil {
			return errors.Errorf(
				"create volume attachments for unit %q: %w", unitUUID, err,
			)
		}
	}

	return nil
}

// insertUnitStorageDirectives is responsible for creating the storage
// directives for a unit. This func assumes that no storage directives exist
// already for the unit.
//
// The storage directives supply must match the storage defined by the charm.
// It is expected that the caller is satisfied this check has been performed.
func (st *State) insertUnitStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	charmUUID corecharm.ID,
	args []internal.CreateUnitStorageDirectiveArg,
) error {
	if len(args) == 0 {
		return nil
	}

	insertStorageDirectiveStmt, err := st.Prepare(`
INSERT INTO unit_storage_directive (*) VALUES ($insertUnitStorageDirective.*)
`,
		insertUnitStorageDirective{})
	if err != nil {
		return errors.Capture(err)
	}

	insertArgs := make([]insertUnitStorageDirective, 0, len(args))
	for _, arg := range args {
		insertArgs = append(insertArgs, insertUnitStorageDirective{
			CharmUUID:       charmUUID.String(),
			Count:           arg.Count,
			Size:            arg.Size,
			StorageName:     arg.Name.String(),
			StoragePoolUUID: arg.PoolUUID.String(),
			UnitUUID:        unitUUID.String(),
		})
	}

	err = tx.Query(ctx, insertStorageDirectiveStmt, insertArgs).Run()
	if err != nil {
		return errors.Errorf(
			"creating unit %q storage directives: %w", unitUUID, err,
		)
	}

	return nil
}

// insertUnitStorageInstances is responsible for creating all of the needed
// storage instances to satisfy the storage instance arguments supplied.
func (st *State) insertUnitStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	stArgs []internal.CreateUnitStorageInstanceArg,
) error {
	storageInstArgs, err := st.makeInsertUnitStorageInstanceArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return errors.Errorf(
			"creating database input for makeing unit storage instances: %w",
			err,
		)
	}

	fsArgs, fsInstanceArgs, fsStatusArgs, err := st.makeInsertUnitFilesystemArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return errors.Errorf(
			"creating database input for makeing unit storage filesystems: %w",
			err,
		)
	}

	vArgs, vInstanceArgs, vStatusArgs, err := st.makeInsertUnitVolumeArgs(
		ctx, tx, stArgs,
	)
	if err != nil {
		return errors.Errorf(
			"creating database input for makeing unit storage volumes: %w",
			err,
		)
	}

	insertStorageInstStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($insertStorageInstance.*)
`,
		insertStorageInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (*) VALUES ($insertStorageFilesystem.*)
`,
		insertStorageFilesystem{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageFilesystemInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*) VALUES ($insertStorageFilesystemInstance.*)
`,
		insertStorageFilesystemInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageFilesystemStatusStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_status (*) VALUES ($insertStorageFilesystemStatus.*)
`,
		insertStorageFilesystemStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageVolumeStmt, err := st.Prepare(`
INSERT INTO storage_volume (*) VALUES ($insertStorageVolume.*)
`,
		insertStorageVolume{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageVolumeInstStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($insertStorageVolumeInstance.*)
`,
		insertStorageVolumeInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageVolumeStatusStmt, err := st.Prepare(`
INSERT INTO storage_volume_status (*) VALUES ($insertStorageVolumeStatus.*)
`,
		insertStorageVolumeStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	// We guard against zero length insert args below. This is because there is
	// no correlation between input args and the number of inserts that happen.
	// Empty inserts will result in an error that we don't need to consider.
	if len(storageInstArgs) != 0 {
		err := tx.Query(ctx, insertStorageInstStmt, storageInstArgs).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage instance(s): %w",
				len(storageInstArgs), err,
			)
		}
	}

	if len(fsArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemStmt, fsArgs).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage filesystems: %w",
				len(fsArgs), err,
			)
		}
	}

	if len(fsInstanceArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemInstStmt, fsInstanceArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting storage filesystem to instance relationship for new filesystems: %w",
				err,
			)
		}
	}

	if len(fsStatusArgs) != 0 {
		err := tx.Query(ctx, insertStorageFilesystemStatusStmt, fsStatusArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting newly create storage filesystem(s) status: %w",
				err,
			)
		}
	}

	if len(vArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeStmt, vArgs).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage volumes: %w",
				len(fsArgs), err,
			)
		}
	}

	if len(vInstanceArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeInstStmt, vInstanceArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting storage volume to instance relationship for new volumes: %w",
				err,
			)
		}
	}

	if len(vStatusArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeStatusStmt, vStatusArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting newly create storage volume(s) status: %w",
				err,
			)
		}
	}

	return nil
}

// insertUnitStorageOwnership is responsible setting unit ownership records for
// the supplied storage instance uuids.
func (st *State) insertUnitStorageOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	storageToOwn []domainstorage.StorageInstanceUUID,
) error {
	args := makeInsertUnitStorageOwnerArgs(ctx, unitUUID, storageToOwn)
	if len(args) == 0 {
		return nil
	}

	insertStorageOwnerStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($insertStorageUnitOwner.*)
`,
		insertStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStorageOwnerStmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting storage instance unit owner: %w", err,
		)
	}

	return nil
}

// insertMachineVolumeOwnership is responsible setting machine ownership records
// for the supplied volume uuids.
func (st *State) insertMachineVolumeOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID coremachine.UUID,
	volumesToOwn []domainstorageprov.VolumeUUID,
) error {
	args := makeInsertMachineVolumeOwnerArgs(ctx, machineUUID, volumesToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_volume (*) VALUES ($insertVolumeMachineOwner.*)
`, insertVolumeMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting volume machine owner: %w", err,
		)
	}

	return nil
}

// insertMachineFilesystemOwnership is responsible setting machine ownership
// records for the supplied filesystem uuids.
func (st *State) insertMachineFilesystemOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID coremachine.UUID,
	filesystemsToOwn []domainstorageprov.FilesystemUUID,
) error {
	args := makeInsertMachineFilesystemOwnerArgs(ctx, machineUUID,
		filesystemsToOwn)
	if len(args) == 0 {
		return nil
	}

	stmt, err := st.Prepare(`
INSERT INTO machine_filesystem (*) VALUES ($insertFilesystemMachineOwner.*)
`, insertFilesystemMachineOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting filesystem machine owner: %w", err,
		)
	}

	return nil
}

// GetProviderTypeForPool returns the provider type that is in use for the
// given pool.
//
// The following error types can be expected:
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
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
			).Add(storageerrors.PoolNotFoundError)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return typeVal.Type, nil
}

// makeInsertUnitFilesystemArgs is responsible for making the insert args to
// establish new filesystems linked to a storage instance in the model.
func (st *State) makeInsertUnitFilesystemArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) (
	[]insertStorageFilesystem,
	[]insertStorageFilesystemInstance,
	[]insertStorageFilesystemStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		// If the caller does not provide a filesystem uuid then they don't
		// expect one to be created.
		if arg.Filesystem == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}

	// early exit
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	// now that we have the set of filesystems we can generate the ids
	fsIDS, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)), filesystemNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"generating %d new filesystem ids: %w", len(argIndexes), err,
		)
	}

	fsStatus, err := status.EncodeStorageFilesystemStatus(
		status.StorageFilesystemStatusTypePending,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"encoding filesystem status pending for new filesystem args: err",
		)
	}

	fsRval := make([]insertStorageFilesystem, 0, len(argIndexes))
	fsInstanceRval := make([]insertStorageFilesystemInstance, 0, len(argIndexes))
	fsStatusRval := make([]insertStorageFilesystemStatus, 0, len(argIndexes))
	statusTime := time.Now()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		fsRval = append(fsRval, insertStorageFilesystem{
			FilesystemID:     fmt.Sprintf("%d", fsIDS[i]),
			LifeID:           int(life.Alive),
			UUID:             instArg.Filesystem.UUID.String(),
			ProvisionScopeID: int(instArg.Filesystem.ProvisionScope),
		})
		fsInstanceRval = append(fsInstanceRval, insertStorageFilesystemInstance{
			StorageInstanceUUID:    instArg.UUID.String(),
			StorageFilesystemUUUID: instArg.Filesystem.UUID.String(),
		})
		fsStatusRval = append(fsStatusRval, insertStorageFilesystemStatus{
			FilesystemUUID: instArg.Filesystem.UUID.String(),
			StatusID:       fsStatus,
			UpdateAt:       statusTime,
		})
	}

	return fsRval, fsInstanceRval, fsStatusRval, nil
}

// makeInsertUnitFilesystemAttachmentArgs will make a slice of
// [insertStorageFilesystemAttachment] for each filesystem attachment defined in
// args.
func (st *State) makeInsertUnitFilesystemAttachmentArgs(
	args []internal.CreateUnitStorageAttachmentArg,
) []insertStorageFilesystemAttachment {
	rval := []insertStorageFilesystemAttachment{}
	for _, arg := range args {
		if arg.FilesystemAttachment == nil {
			continue
		}

		insertArg := insertStorageFilesystemAttachment{
			LifeID:                int(life.Alive),
			NetNodeUUID:           arg.FilesystemAttachment.NetNodeUUID.String(),
			ProvisionScopeID:      int(arg.FilesystemAttachment.ProvisionScope),
			StorageFilesystemUUID: arg.FilesystemAttachment.FilesystemUUID.String(),
			UUID:                  arg.FilesystemAttachment.UUID.String(),
		}
		if arg.FilesystemAttachment.ProviderID != nil {
			insertArg.ProviderID.V = *arg.FilesystemAttachment.ProviderID
			insertArg.ProviderID.Valid = true
		}
		rval = append(rval, insertArg)
	}

	return rval
}

// makeInsertUnitStorageInstanceArgs is responsible for making the insert args
// required for instantiating new storage instances that match a unit's storage
// directive Included in the return is the set of insert values required for
// making the unit the owner of the new storage instance(s). Attachment records
// are also returned for each of the storage instances.
func (st *State) makeInsertUnitStorageInstanceArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) ([]insertStorageInstance, error) {
	storageInstancesRval := make([]insertStorageInstance, 0, len(args))

	for _, arg := range args {
		id, err := sequencestate.NextValue(ctx, st, tx, storageNamespace)
		if err != nil {
			return nil, errors.Errorf(
				"creating unique storage instance id: %w", err,
			)
		}
		storageID := corestorage.MakeID(
			corestorage.Name(arg.Name), id,
		).String()

		storageInstancesRval = append(storageInstancesRval, insertStorageInstance{
			CharmName:       arg.CharmName,
			LifeID:          int(life.Alive),
			RequestSizeMiB:  arg.RequestSizeMiB,
			StorageID:       storageID,
			StorageKindID:   int(arg.Kind),
			StorageName:     arg.Name.String(),
			StoragePoolUUID: arg.StoragePoolUUID.String(),
			UUID:            arg.UUID.String(),
		})
	}

	return storageInstancesRval, nil
}

// makeInsertUnitStorageAttachmentArgs is responsible for making the set of
// storage instance attachment arguments that correspond to the storage uuids.
func makeInsertUnitStorageAttachmentArgs(
	_ context.Context,
	unitUUID coreunit.UUID,
	storageToAttach []internal.CreateUnitStorageAttachmentArg,
) []insertStorageInstanceAttachment {
	rval := make([]insertStorageInstanceAttachment, 0, len(storageToAttach))
	for _, sa := range storageToAttach {
		rval = append(rval, insertStorageInstanceAttachment{
			LifeID:              int(life.Alive),
			StorageInstanceUUID: sa.StorageInstanceUUID.String(),
			UnitUUID:            unitUUID.String(),
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
	unitUUID coreunit.UUID,
	storageToOwn []domainstorage.StorageInstanceUUID,
) []insertStorageUnitOwner {
	rval := make([]insertStorageUnitOwner, 0, len(storageToOwn))
	for _, instUUID := range storageToOwn {
		rval = append(rval, insertStorageUnitOwner{
			StorageInstanceUUID: instUUID.String(),
			UnitUUID:            unitUUID.String(),
		})
	}

	return rval
}

// makeInsertUnitVolumeArgs is responsible for making the insert args to
// establish new volumes linked to a storage instance in the model.
func (st *State) makeInsertUnitVolumeArgs(
	ctx context.Context,
	tx *sqlair.TX,
	args []internal.CreateUnitStorageInstanceArg,
) (
	[]insertStorageVolume,
	[]insertStorageVolumeInstance,
	[]insertStorageVolumeStatus,
	error,
) {
	argIndexes := make([]int, 0, len(args))
	for i, arg := range args {
		// If the caller does not provide a volume uuid then they don't
		// expect one to be created.
		if arg.Volume == nil {
			continue
		}
		argIndexes = append(argIndexes, i)
	}

	// early exit
	if len(argIndexes) == 0 {
		return nil, nil, nil, nil
	}

	// now that we have the set of volumes we can generate the ids
	fsIDS, err := sequencestate.NextNValues(
		ctx, st, tx, uint64(len(argIndexes)), volumeNamespace,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"generating %d new volume ids: %w", len(argIndexes), err,
		)
	}

	vStatus, err := status.EncodeStorageVolumeStatus(
		status.StorageVolumeStatusTypePending,
	)
	if err != nil {
		return nil, nil, nil, errors.Errorf(
			"encoding volume status pending for new volume args: err",
		)
	}

	vRval := make([]insertStorageVolume, 0, len(argIndexes))
	vInstanceRval := make([]insertStorageVolumeInstance, 0, len(argIndexes))
	vStatusRval := make([]insertStorageVolumeStatus, 0, len(argIndexes))
	statusTime := time.Now()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		vRval = append(vRval, insertStorageVolume{
			VolumeID:         fmt.Sprintf("%d", fsIDS[i]),
			LifeID:           int(life.Alive),
			UUID:             instArg.Volume.UUID.String(),
			ProvisionScopeID: int(instArg.Volume.ProvisionScope),
		})
		vInstanceRval = append(vInstanceRval, insertStorageVolumeInstance{
			StorageInstanceUUID: instArg.UUID.String(),
			StorageVolumeUUID:   instArg.Volume.UUID.String(),
		})
		vStatusRval = append(vStatusRval, insertStorageVolumeStatus{
			VolumeUUID: instArg.Volume.UUID.String(),
			StatusID:   vStatus,
			UpdateAt:   statusTime,
		})
	}

	return vRval, vInstanceRval, vStatusRval, nil
}

// makeInsertUnitVolumeAttachmentArgs will make a slice of
// [insertStorageVolumeAttachment] values for each volume attachment argument
// supplied.
func (st *State) makeInsertUnitVolumeAttachmentArgs(
	args []internal.CreateUnitStorageAttachmentArg,
) []insertStorageVolumeAttachment {
	rval := []insertStorageVolumeAttachment{}
	for _, arg := range args {
		if arg.VolumeAttachment == nil {
			continue
		}

		insertArg := insertStorageVolumeAttachment{
			LifeID:            int(life.Alive),
			NetNodeUUID:       arg.VolumeAttachment.NetNodeUUID.String(),
			ProvisionScopeID:  int(arg.VolumeAttachment.ProvisionScope),
			StorageVolumeUUID: arg.VolumeAttachment.VolumeUUID.String(),
			UUID:              arg.VolumeAttachment.UUID.String(),
		}
		if arg.VolumeAttachment.ProviderID != nil {
			insertArg.ProviderID.V = *arg.VolumeAttachment.ProviderID
			insertArg.ProviderID.Valid = true
		}
		rval = append(rval, insertArg)
	}

	return rval
}

// makeInsertMachineVolumeOwnerArgs is responsible for making the set of volume
// machine owner arguments that correspond to the machine and volumes supplied.
func makeInsertMachineVolumeOwnerArgs(
	_ context.Context,
	machineUUID coremachine.UUID,
	volumesToOwn []domainstorageprov.VolumeUUID,
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
	filesystemsToOwn []domainstorageprov.FilesystemUUID,
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

// AttachStorage attaches the specified storage to the specified unit.
// The following error types can be expected:
// - [storageerrors.StorageInstanceNotFound] when the storage doesn't exist.
// - [applicationerrors.UnitNotFound]: when the unit does not exist.
// - [applicationerrors.StorageAlreadyAttached]: when the attachment already exists.
// - [applicationerrors.FilesystemAlreadyAttached]: when the filesystem is already attached.
// - [applicationerrors.VolumeAlreadyAttached]: when the volume is already attached.
// - [applicationerrors.UnitNotAlive]: when the unit is not alive.
// - [applicationerrors.StorageNotAlive]: when the storage is not alive.
// - [applicationerrors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
// - [applicationerrors.InvalidStorageCount]: when the allowed attachment count would be violated.
// - [applicationerrors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
func (st *State) AttachStorage(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	countQuery, err := st.Prepare(`
SELECT count(*) AS &storageCount.count
FROM storage_instance si
JOIN storage_unit_owner suo ON si.uuid = suo.storage_instance_uuid
WHERE suo.unit_uuid = $storageCount.unit_uuid
AND si.storage_name = $storageCount.storage_name
AND si.uuid != $storageCount.uuid
`, storageCount{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// First to the basic life checks for the unit and storage.
		unitLifeID, netNodeUUID, err := st.getUnitLifeAndNetNode(ctx, tx, unitUUID)
		if err != nil {
			return err
		}
		if unitLifeID != life.Alive {
			return errors.Errorf("unit %q is not alive", unitUUID).Add(applicationerrors.UnitNotAlive)
		}

		stor, err := st.getStorageDetails(ctx, tx, storageUUID)
		if err != nil {
			return err
		}
		if stor.LifeID != life.Alive {
			return errors.Errorf("storage %q is not alive", unitUUID).Add(applicationerrors.StorageNotAlive)
		}

		// See if the storage name is supported by the unit's current charm.
		// We might be attaching a storage instance created for a previous charm version
		// and no longer supported.
		charmStorage, err := st.getUnitCharmStorageByName(ctx, tx, unitUUID, stor.StorageName)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"charm for unit %q has no storage called %q",
				unitUUID, stor.StorageName,
			).Add(applicationerrors.StorageNameNotSupported)
		} else if err != nil {
			return errors.Errorf("getting charm storage metadata for storage name %q unit %q: %w", stor.StorageName, unitUUID, err)
		}

		// Check allowed storage attachment counts - will this attachment exceed the max allowed.
		// First get the number of storage instances (excluding the one we are attaching)
		// of the same name already owned by this unit.
		storageCount := storageCount{StorageUUID: storageUUID, StorageName: stor.StorageName, UnitUUID: unitUUID}
		err = tx.Query(ctx, countQuery, storageCount).Get(&storageCount)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying storage count for storage %q on unit %q: %w", stor.StorageName, unitUUID, err)
		}
		// Ensure that the attachment count can increase by 1.
		if err := ensureCharmStorageCountChange(charmStorage, storageCount.Count, 1); err != nil {
			return err
		}

		return st.attachStorage(ctx, tx, stor, unitUUID, netNodeUUID, charmStorage)
	})
	if err != nil {
		return errors.Errorf("attaching storage %q to unit %q: %w", storageUUID, unitUUID, err)
	}
	return nil
}

func (st *State) AddStorageForUnit(ctx context.Context, storageName corestorage.Name, unitUUID coreunit.UUID, directive storage.Directive) ([]corestorage.ID, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (st *State) DetachStorageForUnit(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) DetachStorage(ctx context.Context, storageUUID domainstorage.StorageInstanceUUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) getStorageDetails(ctx context.Context, tx *sqlair.TX, storageUUID domainstorage.StorageInstanceUUID) (storageInstance, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	query := `
SELECT &storageInstance.*
FROM   storage_instance
WHERE  uuid = $storageInstance.uuid
`
	queryStmt, err := st.Prepare(query, inst)
	if err != nil {
		return storageInstance{}, errors.Capture(err)
	}

	err = tx.Query(ctx, queryStmt, inst).Get(&inst)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return storageInstance{}, errors.Errorf("querying storage %q life: %w", storageUUID, err)
		}
		return storageInstance{}, errors.Errorf("%w: %s", storageerrors.StorageInstanceNotFound, storageUUID)
	}
	return inst, nil
}

func (st *State) getUnitCharmStorageByName(ctx context.Context, tx *sqlair.TX, uuid coreunit.UUID, name corestorage.Name) (charmStorage, error) {
	storageSpec := unitCharmStorage{
		UnitUUID:    uuid,
		StorageName: name,
	}
	var result charmStorage
	stmt, err := st.Prepare(`
SELECT cs.* AS &charmStorage.*
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

// ensureCharmStorageCountChange checks that the charm storage can change by
// the specified (positive or negative) increment. This is a backstop - the service
// should already have performed the necessary validation.
func ensureCharmStorageCountChange(charmStorage charmStorage, current, n uint64) error {
	action := "attach"
	gerund := action + "ing"
	pluralise := ""
	if n != 1 {
		pluralise = "s"
	}

	count := current + n
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("cannot %s, storage is singular", action)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"which is less than the minimum of %d",
			gerund, n, pluralise, count,
			charmStorage.CountMin,
		).Add(applicationerrors.InvalidStorageCount)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"exceeding the maximum of %d",
			gerund, n, pluralise, count,
			charmStorage.CountMax,
		).Add(applicationerrors.InvalidStorageCount)
	}
	return nil
}

func (st *State) attachStorage(
	ctx context.Context, tx *sqlair.TX, inst storageInstance, unitUUID coreunit.UUID, netNodeUUID string,
	charmStorage charmStorage,
) error {
	// TODO (tlm) reimplement when we understand what attach storage looks like.
	return coreerrors.NotImplemented
	//	su := storageUnit{StorageUUID: inst.StorageUUID, UnitUUID: unitUUID}
	//	updateStorageInstanceQuery, err := st.Prepare(`
	//
	// UPDATE storage_instance
	// SET    charm_uuid = (SELECT charm_uuid FROM unit where uuid = $storageUnit.unit_uuid)
	// WHERE  uuid = $storageUnit.storage_instance_uuid
	// `, su)
	//
	//	if err != nil {
	//		return errors.Capture(err)
	//	}
	//
	//	err = st.attachStorageToUnit(ctx, tx, inst.StorageUUID, unitUUID)
	//	if err != nil {
	//		return errors.Errorf("attaching storage %q to unit %q: %w", inst.StorageUUID, unitUUID, err)
	//	}
	//
	//	// TODO(storage) - insert data for the unit's assigned machine when that is implemented
	//
	//	// Attach volumes and filesystems for reattached storage on CAAS.
	//	// This only occurs in corner cases where a new pod appears with storage
	//	// that needs to be reconciled with the Juju model. It is part of the
	//	// UnitIntroduction workflow when a pod appears with volumes already attached.
	//	// TODO - this can be removed when ObservedAttachedVolumeIDs are processed.
	//	modelType, err := st.getModelType(ctx, tx)
	//	if err != nil {
	//		return errors.Errorf("getting model type: %w", err)
	//	}
	//	if modelType == model.CAAS {
	//		filesystem, volume, err := st.attachmentParamsForStorageInstance(ctx, tx, inst.StorageUUID, inst.StorageID, inst.StorageName, charmStorage)
	//		if err != nil {
	//			return errors.Errorf("creating storage attachment parameters: %w", err)
	//		}
	//		if filesystem != nil {
	//			if err := st.attachFilesystemToNode(ctx, tx, netNodeUUID, *filesystem); err != nil {
	//				return errors.Errorf("attaching filesystem %q to unit %q: %w", filesystem.filesystemUUID, unitUUID, err)
	//			}
	//		}
	//		if volume != nil {
	//			if err := st.attachVolumeToNode(ctx, tx, netNodeUUID, *volume); err != nil {
	//				return errors.Errorf("attaching volume %q to unit %q: %w", volume.volumeUUID, unitUUID, err)
	//			}
	//		}
	//	}
	//
	//	// Update the charm of the storage instance to match the unit to which it is being attached.
	//	err = tx.Query(ctx, updateStorageInstanceQuery, su).Run()
	//	if err != nil {
	//		return errors.Errorf("updating storage instance %q charm: %w", inst.StorageUUID, err)
	//	}
	//	return nil
}

//type filesystemAttachmentParams struct {
//	// locationAutoGenerated records whether or not the Location
//	// field's value was automatically generated, and thus known
//	// to be unique. This is used to optimise away mount point
//	// conflict checks.
//	locationAutoGenerated bool
//	filesystemUUID        string
//	location              string
//	readOnly              bool
//}

// attachmentParamsForStorageInstance returns parameters for creating
// volume and filesystem attachments for the specified storage.
//func (st *State) attachmentParamsForStorageInstance(
//	ctx context.Context,
//	tx *sqlair.TX,
//	storageUUID domainstorage.StorageInstanceUUID,
//	storageID corestorage.ID,
//	storageName corestorage.Name,
//	charmStorage charmStorage,
//) (filesystemResult *filesystemAttachmentParams, volumeResult *volumeAttachmentParams, _ error) {
//
//	switch charm.StorageType(charmStorage.Kind) {
//	case charm.StorageFilesystem:
//		location, err := domainstorage.FilesystemMountPoint(charmStorage.Location, charmStorage.CountMax, storageID)
//		if err != nil {
//			return nil, nil, errors.Errorf(
//				"getting filesystem mount point for storage %s: %w",
//				storageName, err,
//			).Add(applicationerrors.InvalidStorageMountPoint)
//		}
//
//		filesystem, err := st.getStorageFilesystem(ctx, tx, storageUUID)
//		if err != nil {
//			return nil, nil, errors.Errorf("getting filesystem UUID for storage %q: %w", storageID, err)
//		}
//		// The filesystem already exists, so just attach it.
//		// When creating ops to attach the storage to the
//		// machine, we will check if the attachment already
//		// exists, and whether the storage can be attached to
//		// the machine.
//		if !charmStorage.Shared {
//			// The storage is not shared, so make sure that it is
//			// not currently attached to any other host. If it
//			// is, it should be in the process of being detached.
//			if filesystem.AttachedTo != nil {
//				return nil, nil, errors.Errorf(
//					"filesystem %q is attached to %q", filesystem.UUID, *filesystem.AttachedTo).
//					Add(applicationerrors.FilesystemAlreadyAttached)
//			}
//		}
//		filesystemResult = &filesystemAttachmentParams{
//			locationAutoGenerated: charmStorage.Location == "", // auto-generated location
//			location:              location,
//			readOnly:              charmStorage.ReadOnly,
//			filesystemUUID:        filesystem.UUID,
//		}
//
//		// Fall through to attach the volume that backs the filesystem (if any).
//		fallthrough
//
//	case charm.StorageBlock:
//		volume, err := st.getStorageVolume(ctx, tx, storageUUID)
//		if errors.Is(err, storageerrors.VolumeNotFound) && charm.StorageType(charmStorage.Kind) == charm.StorageFilesystem {
//			break
//		}
//		if err != nil {
//			return nil, nil, errors.Errorf("getting volume UUID for storage %q: %w", storageID, err)
//		}
//
//		// The volume already exists, so just attach it. When
//		// creating ops to attach the storage to the machine,
//		// we will check if the attachment already exists, and
//		// whether the storage can be attached to the machine.
//		if !charmStorage.Shared {
//			// The storage is not shared, so make sure that it is
//			// not currently attached to any other machine. If it
//			// is, it should be in the process of being detached.
//			if volume.AttachedTo != nil {
//				return nil, nil, errors.Errorf("volume %q is attached to %q", volume.UUID, *volume.AttachedTo).
//					Add(applicationerrors.VolumeAlreadyAttached)
//			}
//		}
//		volumeResult = &volumeAttachmentParams{
//			readOnly:   charmStorage.ReadOnly,
//			volumeUUID: volume.UUID,
//		}
//	default:
//		return nil, nil, errors.Errorf("invalid storage kind %v", charmStorage.Kind)
//	}
//	return filesystemResult, volumeResult, nil
//}

//func (st *State) attachFilesystemToNode(
//	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args filesystemAttachmentParams,
//) error {
//	uuid, err := storageprovisioning.NewFilesystemAttachmentUUID()
//	if err != nil {
//		return errors.Capture(err)
//	}
//	fsa := filesystemAttachment{
//		UUID:           uuid.String(),
//		NetNodeUUID:    netNodeUUID,
//		FilesystemUUID: args.filesystemUUID,
//		LifeID:         life.Alive,
//		MountPoint:     args.location,
//		ReadOnly:       args.readOnly,
//	}
//	stmt, err := st.Prepare(`
//INSERT INTO storage_filesystem_attachment (*) VALUES ($filesystemAttachment.*)
//`, fsa)
//	if err != nil {
//		return errors.Capture(err)
//	}
//	err = tx.Query(ctx, stmt, fsa).Run()
//	if err != nil {
//		return errors.Errorf("creating filesystem attachment for %q on %q: %w", args.filesystemUUID, netNodeUUID, err)
//	}
//	return nil
//}

//func (st *State) attachVolumeToNode(
//	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args volumeAttachmentParams,//
//) error {
//	uuid, err := storageprovisioning.NewVolumeAttachmentUUID()
//	if err != nil {
//		return errors.Capture(err)
//	}
//	fsa := volumeAttachment{
//		UUID:        uuid.String(),
//		NetNodeUUID: netNodeUUID,
//		VolumeUUID:  args.volumeUUID,
//		LifeID:      life.Alive,
//		ReadOnly:    args.readOnly,
//	}
//	stmt, err := st.Prepare(`
//INSERT INTO storage_volume_attachment (*) VALUES ($volumeAttachment.*)
//`, fsa)
//	if err != nil {
//		return errors.Capture(err)
//	}
//	err = tx.Query(ctx, stmt, fsa).Run()
//	if err != nil {
//		return errors.Errorf("creating volume attachment for %q on %q: %w", args.volumeUUID, netNodeUUID, err)
//	}
//	return nil
//}
