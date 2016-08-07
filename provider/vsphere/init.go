// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere

import "github.com/juju/juju/environs"

const (
	providerType = "vsphere"
)

func init() {
	environs.RegisterProvider(providerType, providerInstance)
}
