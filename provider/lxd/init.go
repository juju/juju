// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "lxd"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)

	registry.RegisterEnvironStorageProviders(providerType)
}
