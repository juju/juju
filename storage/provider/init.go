// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/environs"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/storage"
)

func init() {
	storage.RegisterProvider(LoopProviderType, &loopProvider{})

	// All environments providers support rootfs loop devices.
	// As a failsafe, ensure at least this storage provider is registered.
	for _, envType := range environs.RegisteredProviders() {
		storage.RegisterEnvironStorageProviders(envType, LoopProviderType)
	}
}
