// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tools"
)

const (
	providerType = "openstack"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	environs.RegisterImageDataSourceFunc("keystone catalog", getKeystoneImageSource)
	tools.RegisterToolsDataSourceFunc("keystone catalog", getKeystoneToolsSource)
}
