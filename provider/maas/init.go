// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/storage/provider/registry"
)

const (
	MAAS = "maas"
)

func init() {
	environs.RegisterProvider(MAAS, maasEnvironProvider{})

	registry.RegisterEnvironStorageProviders(MAAS)
}
