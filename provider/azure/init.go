// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "azure"
)

func init() {
	environs.RegisterProvider(providerType, azureEnvironProvider{})

	registry.RegisterEnvironStorageProviders(providerType)

	// Register cloud local storage as simplestreams image data source.
	environs.RegisterImageDataSourceFunc(common.CloudLocalStorageDesc, getCustomImageSource)
}
