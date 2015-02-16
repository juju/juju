// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider"
	storageprovider "github.com/juju/juju/storage/provider"
)

func init() {
	environs.RegisterProvider(provider.AWS, environProvider{})

	//Register the AWS specific providers.
	storageprovider.RegisterProvider(EBS_ProviderType, &ebsProvider{})

	// Inform the storage provider registry about the AWS providers.
	storageprovider.RegisterEnvironStorageProviders(provider.AWS, EBS_ProviderType)
}
