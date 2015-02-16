// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	AWS = "ec2"
)

func init() {
	environs.RegisterProvider(AWS, environProvider{})

	//Register the AWS specific providers.
	registry.RegisterProvider(EBS_ProviderType, &ebsProvider{})

	// Inform the storage provider registry about the AWS providers.
	registry.RegisterEnvironStorageProviders(AWS, EBS_ProviderType)
}
