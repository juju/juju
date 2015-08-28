// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "gce"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	// Register the GCE specific providers.
	registry.RegisterProvider(GCEProviderType, &storageProvider{})

	// Inform the storage provider registry about the GCE providers.
	registry.RegisterEnvironStorageProviders(providerType, GCEProviderType)
}
