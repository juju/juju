// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

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
func (st *State) CreateStorageInstanceWithExistingFilesystem(
	ctx context.Context,
	args domainstorageinternal.CreateStorageInstanceWithExistingFilesystem,
) (string, error) {
	return "", errors.New("CreateStorageInstanceWithExistingFilesystem not implemented")
}

// CreateStorageInstanceWithExistingVolumeBackedFilesystem creates a new
// storage instance, with a filesystem and volume using existing provisioned
// volume details. It returns the new storage ID for the created storage
// instance.
func (st *State) CreateStorageInstanceWithExistingVolumeBackedFilesystem(
	ctx context.Context,
	args domainstorageinternal.CreateStorageInstanceWithExistingVolumeBackedFilesystem,
) (string, error) {
	return "", errors.New("CreateStorageInstanceWithExistingVolumeBackedFilesystem not implemented")
}
