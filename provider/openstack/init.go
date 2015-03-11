// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "openstack"
)

func init() {
	environs.RegisterProvider(providerType, environProvider{})
	environs.RegisterImageDataSourceFunc("keystone catalog", getKeystoneImageSource)
	tools.RegisterToolsDataSourceFunc("keystone catalog", getKeystoneToolsSource)

	registry.RegisterEnvironStorageProviders(providerType)

	// Register the Openstack specific providers.
	registry.RegisterProvider(
		provider.CinderProviderType,
		&provider.OpenstackProvider{provider.NewGooseAdapter},
	)

	// Inform the storage provider registry about the Openstack provider.
	registry.RegisterEnvironStorageProviders(providerType, provider.CinderProviderType)

}
