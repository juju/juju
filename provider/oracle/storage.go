// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/juju/errors"
	oci "github.com/juju/go-oracle-cloud/api"

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

	// blockDeviceStartIndex is the base from which we start incrementing block device names
	// start from 'a'. xvda will always be the root disk.
	blockDeviceStartIndex = 'a'
)

// storageProvider implements the storage.Provider interface
type storageProvider struct {
	env *OracleEnviron
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

func (o *OracleEnviron) canAccessStorageAPI() (bool, error) {
	// Check if we actually have access to the storage APIs.
	_, err := o.client.AllStorageVolumes(nil)
	if err != nil {
		// The API usually returns a 405 error if we don't have access to that bit of the cloud
		// this is something that happens for trial accounts.
		if oci.IsMethodNotAllowed(err) {
			return false, nil
		}
		return false, errors.Trace(err)
	}
	return true, nil
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (o *OracleEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	if access, err := o.canAccessStorageAPI(); access {
		return []storage.ProviderType{oracleStorageProvideType}, nil
	} else {
		return nil, errors.Trace(err)
	}
}

// StorageProvider implements storage.ProviderRegistry.
func (o *OracleEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	access, err := o.canAccessStorageAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if access && t == oracleStorageProvideType {
		return &storageProvider{
			env: o,
			api: o.client,
		}, nil
	}

	return nil, errors.NotFoundf("storage provider %q", t)
}
