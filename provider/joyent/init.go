// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	Joyent = "joyent"
)

func init() {
	environs.RegisterProvider(Joyent, providerInstance)

	registry.RegisterEnvironStorageProviders(Joyent)
}
