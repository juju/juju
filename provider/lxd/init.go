// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/lxd/lxdnames"
	"github.com/juju/juju/storage/provider/registry"
)

func init() {
	environs.RegisterProvider(lxdnames.ProviderType, providerInstance)

	registry.RegisterEnvironStorageProviders(lxdnames.ProviderType)
}
