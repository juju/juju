// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider"
	storageprovider "github.com/juju/juju/storage/provider"
)

func init() {
	environs.RegisterProvider(provider.Joyent, providerInstance)

	storageprovider.RegisterEnvironStorageProviders(provider.Joyent)
}
