// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider"
	storageprovider "github.com/juju/juju/storage/provider"
)

func init() {
	environs.RegisterProvider(provider.Local, providerInstance)

	// TODO(wallyworld) - sort out policy for allowing loop provider
	storageprovider.RegisterEnvironStorageProviders(
		provider.Local,
		storageprovider.HostLoopProviderType,
	)
	// TODO(wallyworld) - implement when available
	//	storageprovider.RegisterDefaultPool(
	//		provider.Local,
	//		storage.StorageKindBlock,
	//		storageprovider.LoopPool,
	//	)
	//	storageprovider.RegisterDefaultPool(
	//		provider.Local,
	//		storage.StorageKindFilesystem,
	//		storageprovider.RootfsPool,
	//	)
}
