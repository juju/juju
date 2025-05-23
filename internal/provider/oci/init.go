// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import "github.com/juju/juju/environs"

const (
	// providerType is the unique identifier that the oci provider gets
	// registered with.
	providerType = "oci"
)

func init() {
	environs.RegisterProvider(providerType, &EnvironProvider{})
}
