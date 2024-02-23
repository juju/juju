// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import "github.com/juju/juju/environs"

const (
	providerType = "ec2"
)

func init() {
	environs.RegisterProvider(providerType, environProvider{})
}
