// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

func (s State) ImportFilesystem(ctx context.Context, name storage.Name, filesystem domainstorage.FilesystemInfo) (storage.ID, error) {
	//TODO implement me
	return "", errors.New("not implemented")
}

// GetStorageResourceTagInfoForModel retrieves the model based resource tag
// information for storage entities.
func (s *State) GetStorageResourceTagInfoForModel(
	ctx context.Context,
	resourceTagModelConfigKey string,
) (domainstorageprovisioning.ModelResourceTagInfo, error) {
	return domainstorageprovisioning.ModelResourceTagInfo{}, errors.New("GetStorageResourceTagInfoForModel not implemented")
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
