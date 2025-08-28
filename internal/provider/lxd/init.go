// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/lxd/lxdnames"
)

func init() {
	environs.RegisterProvider(lxdnames.ProviderType, NewProvider())
}
