// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	providerType = "manual"
)

func init() {
	p := manualProvider{}
	environs.RegisterProvider(providerType, p, "null")

	registry.RegisterEnvironStorageProviders(providerType)
}
