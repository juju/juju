// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import "github.com/juju/juju/environs"

const (
	// providerType is the unique identifier that the gce provider gets
	// registered with.
	providerType = "gce"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)
}
