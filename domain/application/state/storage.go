// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageprov "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/charm"
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
	appUUID coreapplication.ID,
) ([]application.StorageDirective, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	appUUIDInput := entityUUID{UUID: appUUID.String()}
	query, err := st.Prepare(`
SELECT &applicationStorageDirective.*
FROM   application_storage_directive
WHERE  application_uuid = $entityUUID.uuid
		`,
		appUUIDInput, applicationStorageDirective{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	dbVals := []applicationStorageDirective{}
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
		var (
			poolUUID     *domainstorage.StoragePoolUUID
			providerType *string
		)

		if val.StoragePoolUUID.Valid {
			poolUUIDT := domainstorage.StoragePoolUUID(val.StoragePoolUUID.V)
			poolUUID = &poolUUIDT
		}
		if val.StorageType.Valid {
			providerTypeStr := val.StorageType.V
			providerType = &providerTypeStr
		}

		rval = append(rval, application.StorageDirective{
			Count:        val.Count,
			Name:         domainstorage.Name(val.StorageName),
			PoolUUID:     poolUUID,
			ProviderType: providerType,
			Size:         val.SizeMiB,
		})
	}
	return rval, nil
}

func (st *State) GetStorageInstancesForProviderIDs(
	ctx context.Context,
	appUUID coreapplication.ID,
	ids []string,
) (map[string]domainstorage.StorageInstanceUUID, error) {
	return nil, nil
}

func (st *State) GetUnitOwnedStorageInstances(
	ctx context.Context,
	unitUUID coreunit.UUID,
) (map[domainstorage.Name][]domainstorage.StorageInstanceUUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitUUIDInput := entityUUID{UUID: unitUUID.String()}
	unitOwnedQuery, err := st.Prepare(`
SELECT &unitOwnedStorage.*
FROM   storage_unit_owner suo
JOIN   storage_instance si ON suo.storage_instance_uuid = si.uuid
WHERE  suo.unit_uuid = $entityUUID.uuid
		`,
		unitUUIDInput, unitOwnedStorage{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []unitOwnedStorage
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf(
				"checking unit %q existS: %w", unitUUID, err,
			)
		}
		if !exists {
			return errors.Errorf("unit %q doest not exist: %w", unitUUID, err)
		}

		err = tx.Query(ctx, unitOwnedQuery, unitUUIDInput).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
}

func (st *State) GetUnitStorageDirectives(
	ctx context.Context,
	unitUUID coreunit.UUID,
) ([]application.StorageDirective, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitUUIDInput := entityUUID{UUID: unitUUID.String()}
	query, err := st.Prepare(`
SELECT &unitStorageDirective.*
FROM   unit_storage_directive
WHERE  unit_uuid = $entityUUID.uuid
		`,
		unitUUIDInput, unitStorageDirective{},
	)
	if err != nil {
		return nil, errors.Capture(err)
	}

	dbVals := []unitStorageDirective{}
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
		var (
			poolUUID     *domainstorage.StoragePoolUUID
			providerType *string
		)

		if val.StoragePoolUUID.Valid {
			poolUUIDT := domainstorage.StoragePoolUUID(val.StoragePoolUUID.V)
			poolUUID = &poolUUIDT
		}
		if val.StorageType.Valid {
			providerTypeStr := val.StorageType.V
			providerType = &providerTypeStr
		}

		rval = append(rval, application.StorageDirective{
			Count:        val.Count,
			Name:         domainstorage.Name(val.StorageName),
			PoolUUID:     poolUUID,
			ProviderType: providerType,
			Size:         val.SizeMiB,
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
	uuid coreapplication.ID,
	charmUUID corecharm.ID,
	directives []application.CreateApplicationStorageDirectiveArg,
) error {
	if len(directives) == 0 {
		return nil
	}

	insertDirectivesInput := make([]insertApplicationStorageDirective, 0, len(directives))
	for _, d := range directives {
		var (
			poolUUIDVal     sql.Null[string]
			providerTypeVal sql.Null[string]
		)
		if d.PoolUUID != nil {
			poolUUIDVal = sql.Null[string]{
				V:     d.PoolUUID.String(),
				Valid: true,
			}
		}
		if d.ProviderType != nil {
			providerTypeVal = sql.Null[string]{
				V:     *d.ProviderType,
				Valid: true,
			}
		}

		insertDirectivesInput = append(
			insertDirectivesInput,
			insertApplicationStorageDirective{
				ApplicationUUID:     uuid.String(),
				CharmUUID:           charmUUID.String(),
				Count:               d.Count,
				Size:                d.Size,
				StorageName:         d.Name.String(),
				StoragePoolUUID:     poolUUIDVal,
				StorageProviderType: providerTypeVal,
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
	netNodeUUID domainnetwork.NetNodeUUID,
	storageToAttach []domainstorage.StorageInstanceUUID,
) error {
	storageAttachmentArgs, err := makeInsertUnitStorageAttachmentArgs(
		ctx, unitUUID, storageToAttach,
	)
	if err != nil {
		return errors.Errorf(
			"creating database input for makeing unit storage attachments: %w",
			err,
		)
	}

	fsAttachmentArgs, err := st.makeInsertUnitFilesystemAttachmentArgs(
		ctx, tx, netNodeUUID, storageToAttach,
	)
	if err != nil {
		return errors.Errorf(
			"create database input for making unit filesystem attachments: %w",
			err,
		)
	}

	volAttachmentArgs, err := st.makeInsertUnitVolumeAttachmentArgs(
		ctx, tx, netNodeUUID, storageToAttach,
	)
	if err != nil {
		return errors.Errorf(
			"create database input for making unit volume attachments: %w",
			err,
		)
	}

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
	args []application.CreateUnitStorageDirectiveArg,
) ([]unitStorageDirective, error) {
	if len(args) == 0 {
		return []unitStorageDirective{}, nil
	}

	insertStorageDirectiveStmt, err := st.Prepare(`
INSERT INTO unit_storage_directive (*) VALUES ($insertUnitStorageDirective.*)
`,
		insertUnitStorageDirective{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertArgs := make([]insertUnitStorageDirective, 0, len(args))
	rval := make([]unitStorageDirective, 0, len(args))
	for _, arg := range args {
		storagePoolUUIDVal := sql.Null[string]{}
		storageTypeVal := sql.Null[string]{}
		if arg.PoolUUID != nil {
			storagePoolUUIDVal = sql.Null[string]{
				V:     arg.PoolUUID.String(),
				Valid: true,
			}
		}
		if arg.ProviderType != nil {
			storageTypeVal = sql.Null[string]{
				V:     *arg.ProviderType,
				Valid: true,
			}
		}
		insertArgs = append(insertArgs, insertUnitStorageDirective{
			CharmUUID:       charmUUID.String(),
			Count:           arg.Count,
			Size:            arg.Size,
			StorageName:     arg.Name.String(),
			StoragePoolUUID: storagePoolUUIDVal,
			StorageType:     storageTypeVal,
			UnitUUID:        unitUUID.String(),
		})

		rval = append(rval, unitStorageDirective{
			CharmUUID:       charmUUID,
			Count:           arg.Count,
			Name:            arg.Name.String(),
			StoragePoolUUID: storagePoolUUIDVal,
			StorageProvider: storageTypeVal,
			Size:            arg.Size,
		})
	}

	err = tx.Query(ctx, insertStorageDirectiveStmt, insertArgs).Run()
	if err != nil {
		return nil, errors.Errorf("creating unit %q storage directives: %w", unitUUID, err)
	}

	return rval, nil
}

// insertUnitStorageInstances is responsible for creating all of the needed
// storage instances to satisfy the storage instance arguments supplied.
func (st *State) insertUnitStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	stDirectives []unitStorageDirective,
	stArgs []application.CreateUnitStorageInstanceArg,
) error {
	storageInstArgs, err := st.makeInsertUnitStorageInstanceArgs(
		ctx, tx, stDirectives, stArgs)
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
		err := tx.Query(ctx, insertStorageVolumeStmt, vInstanceArgs).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage volumes: %w",
				len(fsArgs), err,
			)
		}
	}

	if len(vInstanceArgs) != 0 {
		err := tx.Query(ctx, insertStorageVolumeInstStmt, fsInstanceArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting storafe volume to instance relationship for new volumes: %w",
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

// insertUnitStorageOnwership is responsible setting unit ownership records for
// the supplied storage instance uuids.
func (st *State) insertUnitStorageOwnership(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	storageToOwn []domainstorage.StorageInstanceUUID,
) error {
	args := makeInsertUnitStorageOwnerArgs(ctx, unitUUID, storageToOwn)

	insertStorageOwnerStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($insertStorageUnitOwner.*)
`,
		insertStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(args) == 0 {
		return nil
	}

	err = tx.Query(ctx, insertStorageOwnerStmt, args).Run()
	if err != nil {
		return errors.Errorf(
			"setting storage instance unit owner: %w", err,
		)
	}

	return nil
}

// GetProviderTypeOfPool returns the provider type that is in use for the
// given pool.
//
// The following error types can be expected:
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (st *State) GetProviderTypeOfPool(
	ctx context.Context, poolUUID domainstorage.StoragePoolUUID,
) (string, error) {
	db, err := st.DB()
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
	args []application.CreateUnitStorageInstanceArg,
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
		if arg.FilesystemUUID == nil {
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

	fsRval := make([]insertStorageFilesystem, len(argIndexes))
	fsInstanceRval := make([]insertStorageFilesystemInstance, len(argIndexes))
	fsStatusRval := make([]insertStorageFilesystemStatus, len(argIndexes))
	statusTime := time.Now()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		fsRval = append(fsRval, insertStorageFilesystem{
			FilesystemID: corestorage.MakeID(
				corestorage.Name(instArg.Name), fsIDS[i],
			).String(),
			LifeID: int(life.Alive),
			UUID:   instArg.FilesystemUUID.String(),
		})
		fsInstanceRval = append(fsInstanceRval, insertStorageFilesystemInstance{
			StorageInstanceUUID:    instArg.UUID.String(),
			StorageFilesystemUUUID: instArg.FilesystemUUID.String(),
		})
		fsStatusRval = append(fsStatusRval, insertStorageFilesystemStatus{
			FilesystemUUID: instArg.FilesystemUUID.String(),
			StatusID:       fsStatus,
			UpdateAt:       statusTime,
		})
	}

	return fsRval, fsInstanceRval, fsStatusRval, nil
}

// makeInsertUnitFilesystemAttachmentArgs will make a slice of
// [insertStorageFilesystemAttachment] values that will make a filesystem
// attachment for every storage instance supplied that has a filesystem.
//
// The returned attachment values will be attached to the supplied net node.
func (st *State) makeInsertUnitFilesystemAttachmentArgs(
	ctx context.Context,
	tx *sqlair.TX,
	netNodeUUID domainnetwork.NetNodeUUID,
	storageInstances []domainstorage.StorageInstanceUUID,
) ([]insertStorageFilesystemAttachment, error) {
	storageInstancesInput := make(sqlair.S, 0, len(storageInstances))
	for _, uuid := range storageInstances {
		storageInstancesInput = append(storageInstancesInput, uuid.String())
	}

	filesystemUUIDStmt, err := st.Prepare(`
SELECT &storageFilesystemUUIDRef.*
FROM storage_instance_filesystem
WHERE storage_instance_uuid IN ($S[:])
`,
		storageFilesystemUUIDRef{}, storageInstancesInput)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageFilesystemUUIDRef
	err = tx.Query(ctx, filesystemUUIDStmt, storageInstancesInput).GetAll(&dbVals)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	rval := make([]insertStorageFilesystemAttachment, 0, len(dbVals))
	for _, val := range dbVals {
		uuid, err := domainstorageprov.NewFilesystemAttachmentUUID()
		if err != nil {
			return nil, errors.Errorf(
				"generating new filesystem %q attachment uuid for net node uuid %q: %w",
				val.UUID, netNodeUUID, err,
			)
		}

		rval = append(rval, insertStorageFilesystemAttachment{
			LifeID:                int(domainlife.Alive),
			NetNodeUUID:           netNodeUUID.String(),
			StorageFilesystemUUID: val.UUID,
			UUID:                  uuid.String(),
		})
	}

	return rval, nil
}

// makeInsertUnitStorageInstanceArgs is responsible for making the insert args
// required for instantiating new storage instances that match a unit's storage
// directive Included in the return is the set of insert values required for
// making the unit the owner of the new storage instance(s). Attachment records
// are also returned for each of the storage instances.
func (st *State) makeInsertUnitStorageInstanceArgs(
	ctx context.Context,
	tx *sqlair.TX,
	directives []unitStorageDirective,
	args []application.CreateUnitStorageInstanceArg,
) ([]insertStorageInstance, error) {

	directiveMap := make(map[domainstorage.Name]unitStorageDirective, len(directives))
	for _, directive := range directives {
		directiveMap[domainstorage.Name(directive.Name)] = directive
	}

	storageInstancesRval := make([]insertStorageInstance, 0, len(args))

	for _, arg := range args {
		directive := directiveMap[arg.Name]

		id, err := sequencestate.NextValue(ctx, st, tx, storageNamespace)
		if err != nil {
			return nil, errors.Errorf(
				"creating unique storage instance id: %w", err,
			)
		}
		storageID := corestorage.MakeID(
			corestorage.Name(arg.Name), id,
		).String()

		storagePoolVal := sql.Null[string]{}
		if directive.StoragePoolUUID != nil {
			storagePoolVal.V = *directive.StoragePoolUUID
			storagePoolVal.Valid = true
		}
		storageTypeVal := sql.Null[string]{}
		if directive.StorageProvider != nil {
			storageTypeVal.V = *directive.StorageProvider
			storageTypeVal.Valid = true
		}

		storageInstancesRval = append(storageInstancesRval, insertStorageInstance{
			CharmUUID:       directive.CharmUUID.String(),
			LifeID:          int(life.Alive),
			RequestSizeMiB:  directive.Size,
			StorageID:       storageID,
			StorageName:     arg.Name.String(),
			StoragePoolUUID: directive.StoragePoolUUID,
			StorageType:     directive.StorageType,
			UUID:            arg.UUID.String(),
		})
	}

	return storageInstancesRval, nil
}

// makeInsertUnitStorageAttachmentArgs is responsible for making the set of
// storage instance attachment arguments that correspond to the storage uuids.
func makeInsertUnitStorageAttachmentArgs(
	ctx context.Context,
	unitUUID coreunit.UUID,
	storageToAttach []domainstorage.StorageInstanceUUID,
) ([]insertStorageInstanceAttachment, error) {
	rval := make([]insertStorageInstanceAttachment, 0, len(storageToAttach))
	for _, instUUID := range storageToAttach {
		rval = append(rval, insertStorageInstanceAttachment{
			LifeID:              int(domainlife.Alive),
			StorageInstanceUUID: instUUID.String(),
			UnitUUID:            unitUUID.String(),
		})
	}

	return rval, nil
}

// makeInsertUnitStorageOwnerArgs is responsible for making the set of
// storage instance unit owner arguments that correspond to the unit and storage
// instances supplied.
func makeInsertUnitStorageOwnerArgs(
	ctx context.Context,
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
	args []application.CreateUnitStorageInstanceArg,
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
		if arg.VolumeUUID == nil {
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

	vRval := make([]insertStorageVolume, len(argIndexes))
	vInstanceRval := make([]insertStorageVolumeInstance, len(argIndexes))
	vStatusRval := make([]insertStorageVolumeStatus, len(argIndexes))
	statusTime := time.Now()
	for i, argIndex := range argIndexes {
		instArg := args[argIndex]
		vRval = append(vRval, insertStorageVolume{
			VolumeID: corestorage.MakeID(
				corestorage.Name(instArg.Name), fsIDS[i],
			).String(),
			LifeID: int(life.Alive),
			UUID:   instArg.VolumeUUID.String(),
		})
		vInstanceRval = append(vInstanceRval, insertStorageVolumeInstance{
			StorageInstanceUUID: instArg.UUID.String(),
			StorageVolumeUUID:   instArg.VolumeUUID.String(),
		})
		vStatusRval = append(vStatusRval, insertStorageVolumeStatus{
			VolumeUUID: instArg.VolumeUUID.String(),
			StatusID:   vStatus,
			UpdateAt:   statusTime,
		})
	}

	return vRval, vInstanceRval, vStatusRval, nil
}

// makeInsertUnitVolumeAttachmentArgs will make a slice of
// [insertStorageVolumeAttachment] values that will make a volume
// attachment for every storage instance supplied that has a volume.
//
// The returned attachment values will be attached to the supplied net node.
func (st *State) makeInsertUnitVolumeAttachmentArgs(
	ctx context.Context,
	tx *sqlair.TX,
	netNodeUUID domainnetwork.NetNodeUUID,
	storageInstances []domainstorage.StorageInstanceUUID,
) ([]insertStorageVolumeAttachment, error) {
	storageInstancesInput := make(sqlair.S, 0, len(storageInstances))
	for _, uuid := range storageInstances {
		storageInstancesInput = append(storageInstancesInput, uuid.String())
	}

	volumeUUIDStmt, err := st.Prepare(`
SELECT &storageVolumeUUIDRef.*
FROM storage_instance_volume
WHERE storage_instance_uuid IN ($S[:])
`,
		storageVolumeUUIDRef{}, storageInstancesInput)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []storageVolumeUUIDRef
	err = tx.Query(ctx, volumeUUIDStmt, storageInstancesInput).GetAll(&dbVals)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	rval := make([]insertStorageVolumeAttachment, 0, len(dbVals))
	for _, val := range dbVals {
		uuid, err := domainstorageprov.NewVolumeAttachmentUUID()
		if err != nil {
			return nil, errors.Errorf(
				"generating new volume %q attachment uuid for net node uuid %q: %w",
				val.UUID, netNodeUUID, err,
			)
		}

		rval = append(rval, insertStorageVolumeAttachment{
			LifeID:            int(domainlife.Alive),
			NetNodeUUID:       netNodeUUID.String(),
			StorageVolumeUUID: val.UUID,
			UUID:              uuid.String(),
		})
	}

	return rval, nil
}

// GetStorageUUIDByID returns the UUID for the specified storage, returning an error
// satisfying [storageerrors.StorageNotFound] if the storage doesn't exist.
func (st *State) GetStorageUUIDByID(ctx context.Context, storageID corestorage.ID) (domainstorage.StorageInstanceUUID, error) {
	db, err := st.DB()
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
			return errors.Errorf("storage %q not found", storageID).Add(storageerrors.StorageNotFound)
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
// - [storageerrors.StorageNotFound] when the storage doesn't exist.
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
	db, err := st.DB()
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
		return storageInstance{}, errors.Errorf("%w: %s", storageerrors.StorageNotFound, storageUUID)
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

// GetDefaultStorageProvisioners returns the default storage provisioners
// that have been set for the model.
func (st *State) GetDefaultStorageProvisioners(
	ctx context.Context,
) (application.DefaultStorageProvisioners, error) {
	// TODO (tlm) get the default storage provisioners for the model.
	defaultProviderType := "loop"
	return application.DefaultStorageProvisioners{
		BlockdeviceProviderType: &defaultProviderType,
		FilesystemProviderType:  &defaultProviderType,
	}, nil
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
	su := storageUnit{StorageUUID: inst.StorageUUID, UnitUUID: unitUUID}
	updateStorageInstanceQuery, err := st.Prepare(`
UPDATE storage_instance
SET    charm_uuid = (SELECT charm_uuid FROM unit where uuid = $storageUnit.unit_uuid)
WHERE  uuid = $storageUnit.storage_instance_uuid
`, su)
	if err != nil {
		return errors.Capture(err)
	}

	err = st.attachStorageToUnit(ctx, tx, inst.StorageUUID, unitUUID)
	if err != nil {
		return errors.Errorf("attaching storage %q to unit %q: %w", inst.StorageUUID, unitUUID, err)
	}

	// TODO(storage) - insert data for the unit's assigned machine when that is implemented

	// Attach volumes and filesystems for reattached storage on CAAS.
	// This only occurs in corner cases where a new pod appears with storage
	// that needs to be reconciled with the Juju model. It is part of the
	// UnitIntroduction workflow when a pod appears with volumes already attached.
	// TODO - this can be removed when ObservedAttachedVolumeIDs are processed.
	modelType, err := st.getModelType(ctx, tx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}
	if modelType == model.CAAS {
		filesystem, volume, err := st.attachmentParamsForStorageInstance(ctx, tx, inst.StorageUUID, inst.StorageID, inst.StorageName, charmStorage)
		if err != nil {
			return errors.Errorf("creating storage attachment parameters: %w", err)
		}
		if filesystem != nil {
			if err := st.attachFilesystemToNode(ctx, tx, netNodeUUID, *filesystem); err != nil {
				return errors.Errorf("attaching filesystem %q to unit %q: %w", filesystem.filesystemUUID, unitUUID, err)
			}
		}
		if volume != nil {
			if err := st.attachVolumeToNode(ctx, tx, netNodeUUID, *volume); err != nil {
				return errors.Errorf("attaching volume %q to unit %q: %w", volume.volumeUUID, unitUUID, err)
			}
		}
	}

	// Update the charm of the storage instance to match the unit to which it is being attached.
	err = tx.Query(ctx, updateStorageInstanceQuery, su).Run()
	if err != nil {
		return errors.Errorf("updating storage instance %q charm: %w", inst.StorageUUID, err)
	}
	return nil
}

type volumeAttachmentParams struct {
	volumeUUID corestorage.VolumeUUID
	readOnly   bool
}

type filesystemAttachmentParams struct {
	// locationAutoGenerated records whether or not the Location
	// field's value was automatically generated, and thus known
	// to be unique. This is used to optimise away mount point
	// conflict checks.
	locationAutoGenerated bool
	filesystemUUID        corestorage.FilesystemUUID
	location              string
	readOnly              bool
}

// getStorageFilesystem gets the filesystem a storage instance is associated with
// and the net node (if any) it is attached to.
func (st *State) getStorageFilesystem(ctx context.Context, tx *sqlair.TX, storageUUID domainstorage.StorageInstanceUUID) (filesystemUUID, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	result := filesystemUUID{}
	query, err := st.Prepare(`
SELECT    sf.uuid AS &filesystemUUID.uuid,
          sfa.net_node_uuid AS &filesystemUUID.net_node_uuid
FROM      storage_filesystem sf
JOIN      storage_instance_filesystem sif ON sif.storage_filesystem_uuid = sf.uuid
LEFT JOIN storage_filesystem_attachment sfa ON sfa.storage_filesystem_uuid = sf.uuid
WHERE     sif.storage_instance_uuid = $storageInstance.uuid
	`, inst, result)
	if err != nil {
		return result, errors.Capture(err)
	}
	err = tx.Query(ctx, query, inst).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.Errorf("storage filesystem for %q not found", storageUUID).Add(storageerrors.FilesystemNotFound)
	}
	if err != nil {
		return result, errors.Errorf("querying filesystem storage for %q: %w", storageUUID, err)
	}
	return result, nil
}

// getStorageVolume gets the volume a storage instance is associated with
// and the net node (if any) it is attached to.
func (st *State) getStorageVolume(ctx context.Context, tx *sqlair.TX, storageUUID domainstorage.StorageInstanceUUID) (volumeUUID, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	result := volumeUUID{}
	query, err := st.Prepare(`
SELECT    sv.uuid AS &volumeUUID.uuid,
          sva.net_node_uuid AS &volumeUUID.net_node_uuid
FROM      storage_volume sv
JOIN      storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
LEFT JOIN storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
WHERE     siv.storage_instance_uuid = $storageInstance.uuid
	`, inst, result)
	if err != nil {
		return result, errors.Capture(err)
	}
	err = tx.Query(ctx, query, inst).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.Errorf("storage volume for %q not found", storageUUID).Add(storageerrors.VolumeNotFound)
	}
	if err != nil {
		return result, errors.Errorf("querying volume storage for %q: %w", storageUUID, err)
	}
	return result, nil
}

// attachmentParamsForStorageInstance returns parameters for creating
// volume and filesystem attachments for the specified storage.
func (st *State) attachmentParamsForStorageInstance(
	ctx context.Context,
	tx *sqlair.TX,
	storageUUID domainstorage.StorageInstanceUUID,
	storageID corestorage.ID,
	storageName corestorage.Name,
	charmStorage charmStorage,
) (filesystemResult *filesystemAttachmentParams, volumeResult *volumeAttachmentParams, _ error) {

	switch charm.StorageType(charmStorage.Kind) {
	case charm.StorageFilesystem:
		location, err := domainstorage.FilesystemMountPoint(charmStorage.Location, charmStorage.CountMax, storageID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting filesystem mount point for storage %s: %w",
				storageName, err,
			).Add(applicationerrors.InvalidStorageMountPoint)
		}

		filesystem, err := st.getStorageFilesystem(ctx, tx, storageUUID)
		if err != nil {
			return nil, nil, errors.Errorf("getting filesystem UUID for storage %q: %w", storageID, err)
		}
		// The filesystem already exists, so just attach it.
		// When creating ops to attach the storage to the
		// machine, we will check if the attachment already
		// exists, and whether the storage can be attached to
		// the machine.
		if !charmStorage.Shared {
			// The storage is not shared, so make sure that it is
			// not currently attached to any other host. If it
			// is, it should be in the process of being detached.
			if filesystem.AttachedTo != nil {
				return nil, nil, errors.Errorf(
					"filesystem %q is attached to %q", filesystem.UUID, *filesystem.AttachedTo).
					Add(applicationerrors.FilesystemAlreadyAttached)
			}
		}
		filesystemResult = &filesystemAttachmentParams{
			locationAutoGenerated: charmStorage.Location == "", // auto-generated location
			location:              location,
			readOnly:              charmStorage.ReadOnly,
			filesystemUUID:        filesystem.UUID,
		}

		// Fall through to attach the volume that backs the filesystem (if any).
		fallthrough

	case charm.StorageBlock:
		volume, err := st.getStorageVolume(ctx, tx, storageUUID)
		if errors.Is(err, storageerrors.VolumeNotFound) && charm.StorageType(charmStorage.Kind) == charm.StorageFilesystem {
			break
		}
		if err != nil {
			return nil, nil, errors.Errorf("getting volume UUID for storage %q: %w", storageID, err)
		}

		// The volume already exists, so just attach it. When
		// creating ops to attach the storage to the machine,
		// we will check if the attachment already exists, and
		// whether the storage can be attached to the machine.
		if !charmStorage.Shared {
			// The storage is not shared, so make sure that it is
			// not currently attached to any other machine. If it
			// is, it should be in the process of being detached.
			if volume.AttachedTo != nil {
				return nil, nil, errors.Errorf("volume %q is attached to %q", volume.UUID, *volume.AttachedTo).
					Add(applicationerrors.VolumeAlreadyAttached)
			}
		}
		volumeResult = &volumeAttachmentParams{
			readOnly:   charmStorage.ReadOnly,
			volumeUUID: volume.UUID,
		}
	default:
		return nil, nil, errors.Errorf("invalid storage kind %v", charmStorage.Kind)
	}
	return filesystemResult, volumeResult, nil
}

func (st *State) attachStorageToUnit(
	ctx context.Context, tx *sqlair.TX, storageUUID domainstorage.StorageInstanceUUID, unitUUID coreunit.UUID,
) error {
	sa := storageAttachment{StorageUUID: storageUUID, UnitUUID: unitUUID, LifeID: life.Alive}
	attachmentQuery, err := st.Prepare(`
SELECT &storageAttachment.* FROM storage_attachment
WHERE  unit_uuid = $storageAttachment.unit_uuid
AND    storage_instance_uuid = $storageAttachment.storage_instance_uuid
`, sa)
	if err != nil {
		return errors.Capture(err)
	}

	// See if there's already a row in the storage attachment table.
	var attachments []storageAttachment
	err = tx.Query(ctx, attachmentQuery, sa).GetAll(&attachments)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying storage attachment for storage %q and unit %q: %w", storageUUID, unitUUID, err)
	}
	if err == nil {
		return errors.Errorf("storage %q is already attached to unit %q", storageUUID, unitUUID).Add(applicationerrors.StorageAlreadyAttached)
	}

	stmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($storageAttachment.*)
`, sa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, sa).Run()
	if err != nil {
		return errors.Errorf("creating unit storage attachment for %q on %q: %w", storageUUID, unitUUID, err)
	}
	return nil
}

func (st *State) attachFilesystemToNode(
	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args filesystemAttachmentParams,
) error {
	uuid, err := corestorage.NewFilesystemAttachmentUUID()
	if err != nil {
		return errors.Capture(err)
	}
	fsa := filesystemAttachment{
		UUID:           uuid,
		NetNodeUUID:    netNodeUUID,
		FilesystemUUID: args.filesystemUUID,
		LifeID:         life.Alive,
		MountPoint:     args.location,
		ReadOnly:       args.readOnly,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*) VALUES ($filesystemAttachment.*)
`, fsa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, fsa).Run()
	if err != nil {
		return errors.Errorf("creating filesystem attachment for %q on %q: %w", args.filesystemUUID, netNodeUUID, err)
	}
	return nil
}

func (st *State) attachVolumeToNode(
	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args volumeAttachmentParams,
) error {
	uuid, err := corestorage.NewVolumeAttachmentUUID()
	if err != nil {
		return errors.Capture(err)
	}
	fsa := volumeAttachment{
		UUID:        uuid,
		NetNodeUUID: netNodeUUID,
		VolumeUUID:  args.volumeUUID,
		LifeID:      life.Alive,
		ReadOnly:    args.readOnly,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*) VALUES ($volumeAttachment.*)
`, fsa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, fsa).Run()
	if err != nil {
		return errors.Errorf("creating volume attachment for %q on %q: %w", args.volumeUUID, netNodeUUID, err)
	}
	return nil
}
