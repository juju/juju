// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tools"
)

const (
	providerType = "openstack"

	rootDiskSourceVolume = "volume"
	rootDiskSourceLocal  = "local"
	// Default root disk size when root-disk-source is volume.
	defaultRootDiskSize = 30 * 1024 // 30 GiB
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	environs.RegisterImageDataSourceFunc("keystone catalog", getKeystoneImageSource)
	tools.RegisterToolsDataSourceFunc("keystone catalog", getKeystoneToolsSource)
}
