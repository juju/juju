// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import "github.com/juju/juju/environs"

const (
	// providerType is the unique identifier that the ec2 provider gets
	// registered with.
	providerType = "ec2"
)

func init() {
	environs.RegisterProvider(providerType, environProvider{})
}
