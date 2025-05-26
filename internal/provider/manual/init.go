// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import "github.com/juju/juju/environs"

const (
	// providerType is the unique identifier that the manual provider gets
	// registered with.
	providerType = "manual"
)

func init() {
	p := ManualProvider{}
	environs.RegisterProvider(providerType, p, "null")
}
