// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/environs"
	storageprovider "github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	Local = "local"
)

func init() {
	environs.RegisterProvider(Local, providerInstance)

	// TODO(wallyworld) - sort out policy for allowing loop provider
	registry.RegisterEnvironStorageProviders(
		Local,
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
