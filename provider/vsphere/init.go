// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere

import (
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "vsphere"
)

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	if featureflag.Enabled(feature.VSphereProvider) {
		environs.RegisterProvider(providerType, providerInstance)
		registry.RegisterEnvironStorageProviders(providerType)
	}
}
