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
	storage.RegisterProvider(HostLoopProviderType, &hostLoopProvider{})

	// local cloud provider
	storage.RegisterEnvironStorageProviders("local", LoopProviderType, HostLoopProviderType)

	// AWS cloud provider
	storage.RegisterEnvironStorageProviders("ec2", LoopProviderType)
	// TODO: register EBS and other storage providers for EC2

	// TODO: register storage providers for other environs

	// All environments providers support rootfs loop devices.
	// As a failsafe, ensure at least this storage provider is registered.
	for _, envType := range environs.RegisteredProviders() {
		storage.RegisterEnvironStorageProviders(envType, LoopProviderType)
	}
}
