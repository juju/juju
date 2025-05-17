// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tools"
)

const (
	// providerType is the unique identifier that the openstack provider gets
	// registered with.
	providerType = "openstack"

	// Default root disk size when root-disk-source is volume.
	defaultRootDiskSize = 30 * 1024 // 30 GiB
)

const (
	// BlockDeviceMapping source volume type for cinder block device.
	rootDiskSourceVolume = "volume"
	// BlockDeviceMapping source volume type for local block device.
	rootDiskSourceLocal = "local"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	environs.RegisterImageDataSourceFunc("keystone catalog", getKeystoneImageSource)
	tools.RegisterToolsDataSourceFunc("keystone catalog", getKeystoneToolsSource)
}
