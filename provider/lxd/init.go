// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/v2/environs"
	"github.com/juju/juju/v2/provider/lxd/lxdnames"
)

func init() {
	environs.RegisterProvider(lxdnames.ProviderType, NewProvider())
}
