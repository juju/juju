// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
)

func init() {
	environs.RegisterProvider("ec2", environProvider{})

	storage.RegisterProvider(EBS_ProviderType, &ebsProvider{})
	storage.RegisterEnvironStorageProviders("ec2", EBS_ProviderType)

	// TODO(wallyworld) - default pool registration for AWS

	// TODO(wallyworld) - common provider registration
	storage.RegisterEnvironStorageProviders("ec2", provider.LoopProviderType)
}
