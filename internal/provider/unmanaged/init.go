// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unmanaged

import "github.com/juju/juju/environs"

const (
	// providerType is the unique identifier that the unmanaged provider gets
	// registered with.
	providerType = "unmanaged"
)

func init() {
	p := UnmanagedProvider{}
	environs.RegisterProvider(providerType, p, "null")
}
