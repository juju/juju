// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import "github.com/juju/juju/environs"

const (
	providerType = "gce"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)
}
