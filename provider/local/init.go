// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"github.com/juju/juju/environs"
	storageprovider "github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "local"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	// TODO(wallyworld) - sort out policy for allowing loop provider
	registry.RegisterEnvironStorageProviders(
		providerType,
		storageprovider.HostLoopProviderType,
	)
}
