// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/caas"
	// "github.com/juju/juju/environs"
)

const (
	providerType = "kubernetes"
)

func init() {
	caas.RegisterContainerProvider(providerType, providerInstance)
	// environs.RegisterProvider(providerType, providerInstance)
}
