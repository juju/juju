// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"github.com/juju/juju/v2/environs"
)

const (
	providerType = "equinix"
)

func init() {
	environs.RegisterProvider(providerType, environProvider{})
}

func NewProvider() environs.CloudEnvironProvider {
	return environProvider{}
}
