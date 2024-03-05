// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/juju/environs"
)

const (
	providerType = "maas"
)

func init() {
	environs.RegisterProvider(providerType, EnvironProvider{GetCapabilities: getCapabilities})
}
