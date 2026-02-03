// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/life"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// GetStorageResourceTagInfoForModel retrieves the model based resource tag
// information for storage entities.
func (st *State) GetStorageResourceTagInfoForModel(
	ctx context.Context,
	resourceTagModelConfigKey string,
) (domainstorageprovisioning.ModelResourceTagInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	var rval domainstorageprovisioning.ModelResourceTagInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = st.getStorageResourceTagInfoForModel(
			ctx, tx, resourceTagModelConfigKey,
		)
		return err
	})

	if err != nil {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	return rval, nil
}

// getStorageResourceTagInfoForModel retrieves the model based resource tag
// information for storage entities.
func (st *State) getStorageResourceTagInfoForModel(
	ctx context.Context,
	tx *sqlair.TX,
	resourceTagModelConfigKey string,
) (domainstorageprovisioning.ModelResourceTagInfo, error) {
	type modelConfigKey struct {
		Key string `db:"key"`
	}

	var (
		modelConfigKeyInput = modelConfigKey{Key: resourceTagModelConfigKey}
		dbVal               modelResourceTagInfo
	)

	resourceTagStmt, err := st.Prepare(`
SELECT value AS &modelResourceTagInfo.resource_tags
FROM   model_config
WHERE  key = $modelConfigKey.key
`,
		dbVal, modelConfigKeyInput)
	if err != nil {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	modelInfoStmt, err := st.Prepare(`
SELECT (uuid, controller_uuid) AS (&modelResourceTagInfo.*)
FROM model
`,
		dbVal)
	if err != nil {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	err = tx.Query(ctx, resourceTagStmt, modelConfigKeyInput).Get(&dbVal)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Errorf(
			"getting model config value for key %q: %w",
			resourceTagModelConfigKey, err,
		)
	}

	err = tx.Query(ctx, modelInfoStmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		// This must never happen, but we return an error that at least signals
		// the problem correctly in case it does.
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.New(
			"model database has not had its information set",
		)
	} else if err != nil {
		return domainstorageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	return domainstorageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: dbVal.ResourceTags,
		ControllerUUID:   dbVal.ControllerUUID,
		ModelUUID:        dbVal.ModelUUID,
	}, nil
}

// CreateStorageInstanceWithExistingFilesystem creates a new storage
// instance, with a filesystem using existing provisioned filesystem
// details. It returns the new storage ID for the created storage instance.
//
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified UUID
// does not exist.
func (st *State) CreateStorageInstanceWithExistingFilesystem(
	ctx context.Context,
	args domainstorageinternal.CreateStorageInstanceWithExistingFilesystem,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	storageInstance := insertStorageInstance{
		UUID:             args.UUID.String(),
		LifeID:           life.Alive,
		StorageName:      args.Name.String(),
		StorageKindID:    int(args.Kind),
		StoragePoolUUID:  args.StoragePoolUUID.String(),
		RequestedSizeMiB: args.RequestedSizeMiB,
	}
	insertStorageInstanceStmt, err := st.Prepare(`
INSERT INTO storage_instance (*)
VALUES ($insertStorageInstance.*)
`, storageInstance)
	if err != nil {
		return "", errors.Capture(err)
	}

	filesystem := insertStorageFilesystem{
		UUID:             args.FilesystemUUID.String(),
		LifeID:           life.Alive,
		ProvisionScopeID: int(args.FilesystemProvisionScope),
		ProviderID:       args.FilesystemProviderID,
		SizeMiB:          args.FilesystemSize,
	}
	insertFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (*)
VALUES ($insertStorageFilesystem.*)
`, filesystem)
	if err != nil {
		return "", errors.Capture(err)
	}

	storageInstanceFilesystem := insertStorageInstanceFilesystem{
		StorageInstanceUUID:   args.UUID.String(),
		StorageFilesystemUUID: args.FilesystemUUID.String(),
	}
	insertLinkStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*)
VALUES ($insertStorageInstanceFilesystem.*)
`, storageInstanceFilesystem)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStoragePoolExists(
			ctx, tx, args.StoragePoolUUID.String())
		if err != nil {
			return errors.Errorf(
				"checking if storage pool %q exists: %w",
				args.StoragePoolUUID, err,
			)
		} else if !exists {
			return errors.Errorf(
				"storage pool %q does not exist", args.StoragePoolUUID,
			).Add(domainstorageerrors.StoragePoolNotFound)
		}

		storageSeq, err := sequencestate.NextValue(
			ctx, st, tx, domainstorage.StorageInstanceSequenceNamespace,
		)
		if err != nil {
			return errors.Errorf("creating unique storage instance id: %w", err)
		}
		storageInstance.StorageID = corestorage.MakeID(
			corestorage.Name(args.Name), storageSeq,
		).String()

		filesystemSeq, err := sequencestate.NextValue(
			ctx, st, tx, domainstorage.FilesystemSequenceNamespace,
		)
		if err != nil {
			return errors.Errorf("creating unique filesystem id: %w", err)
		}
		filesystem.FilesystemID = fmt.Sprintf("%d", filesystemSeq)

		err = tx.Query(ctx, insertStorageInstanceStmt, storageInstance).Run()
		if err != nil {
			return errors.Errorf("inserting storage instance: %w", err)
		}
		err = tx.Query(ctx, insertFilesystemStmt, filesystem).Run()
		if err != nil {
			return errors.Errorf("inserting filesystem: %w", err)
		}
		err = tx.Query(ctx, insertLinkStmt, storageInstanceFilesystem).Run()
		if err != nil {
			return errors.Errorf("linking storage instance to filesystem: %w", err)
		}

		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return storageInstance.StorageID, nil
}

// CreateStorageInstanceWithExistingVolumeBackedFilesystem creates a new
// storage instance, with a filesystem and volume using existing provisioned
// volume details. It returns the new storage ID for the created storage
// instance.
//
// The following errors can be expected:
// - [domainstorageerrors.StoragePoolNotFound] if a pool with the specified UUID
// does not exist.
func (st *State) CreateStorageInstanceWithExistingVolumeBackedFilesystem(
	ctx context.Context,
	args domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem,
) (string, error) {
	return "", errors.New("CreateStorageInstanceWithExistingVolumeBackedFilesystem not implemented")
}
