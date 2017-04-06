// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/errors"

	"github.com/juju/juju/provider/oracle/common"
	"github.com/juju/juju/storage"
)

const (
	oracleStorageProvideType = storage.ProviderType("oracle")

	// maxVolumeSizeInGB represents the maximum size in GiB for
	// a single volume. For more information please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-volume--post.html#request
	maxVolumeSizeInGB = 2000
	// minVolumeSizeInGB represents the minimum size in GiB for
	// a single volume. For more information please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-volume--post.html#request
	minVolumeSizeInGB = 1
	// maxDevices is the maximum number of volumes that can be attached to a single instance
	// for more information, please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-attachment--post.html#request
	maxDevices = 10
	// blockDevicePrefix is the default block storage prefix for an oracle
	// cloud storage volume, as seen by GNU/Linux guests. For more information, please see:
	// https://docs.oracle.com/cloud/latest/stcomputecs/STCSA/op-storage-attachment--post.html#request
	blockDevicePrefix = "xvd"
)

// storageProvider implements the storage.Provider interface
type storageProvider struct {
	env *oracleEnviron
	api StorageAPI
}

var _ storage.Provider = (*storageProvider)(nil)

// StorageAPI defines the storage API in the oracle cloud provider
// this enables the provider to talk with the storage API and make
// storage specific operations
type StorageAPI interface {
	common.StorageVolumeAPI
	common.StorageAttachmentAPI
	common.Composer
}

func mibToGib(m uint64) uint64 {
	return (m + 1023) / 1024
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (o *oracleEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{oracleStorageProvideType}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (o *oracleEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	if t == oracleStorageProvideType {
		return &storageProvider{
			env: o,
			api: o.client,
		}, nil
	}

	return nil, errors.NotFoundf("storage provider %q", t)
}
