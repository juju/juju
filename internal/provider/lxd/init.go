// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
)

const (
	// providerType is the unique identifier that the lxd provider gets
	// registered with.
	providerType = lxdnames.ProviderType
)

func init() {
	environs.RegisterProvider(providerType, NewProvider())
}
