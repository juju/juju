// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdnames

// NOTE: this package exists to get around circular imports from cloud and
// provider/lxd.

// DefaultCloud is the name of the default lxd cloud, which corresponds to
// the local lxd daemon.
const DefaultCloud = "localhost"

// DefaultCloudAltName is the alternative name of the default lxd cloud,
// which corresponds to the local lxd daemon.
const DefaultCloudAltName = "lxd"

// DefaultLocalRegion is the name of the "region" we support in a local lxd,
// which corresponds to the local lxd daemon.
const DefaultLocalRegion = "localhost"

// DefaultRemoteRegion is the name of the "region" we report if there are no
// other regions for a remote lxd server.
const DefaultRemoteRegion = "default"

// ProviderType defines the provider/cloud type for lxd.
const ProviderType = "lxd"

// IsDefaultCloud returns true if the cloud name is the default lxd cloud.
func IsDefaultCloud(cloudName string) bool {
	return cloudName == DefaultCloud || cloudName == DefaultCloudAltName
}
