// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/juju/environs"
)

const (
	// providerType is the unique identifier that the maas provider gets
	// registered with.
	providerType = "maas"
)

func init() {
	environs.RegisterProvider(providerType, EnvironProvider{GetCapabilities: getCapabilities})
}
