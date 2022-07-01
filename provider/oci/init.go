// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import "github.com/juju/juju/v3/environs"

const (
	providerType = "oci"
)

func init() {
	environs.RegisterProvider(providerType, &EnvironProvider{})
}
