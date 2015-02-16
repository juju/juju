// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	Manual = "manual"
)

func init() {
	p := manualProvider{}
	environs.RegisterProvider(Manual, p, "null")

	registry.RegisterEnvironStorageProviders(Manual)
}
