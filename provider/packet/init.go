// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packet

import (
	"github.com/juju/juju/environs"
)

const (
	providerType = "packet"
)

func init() {
	environs.RegisterProvider(providerType, environProvider{})
}
