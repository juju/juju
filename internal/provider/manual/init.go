// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import "github.com/juju/juju/environs"

const (
	providerType = "manual"
)

func init() {
	p := ManualProvider{}
	environs.RegisterProvider(providerType, p, "null")
}
