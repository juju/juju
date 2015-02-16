// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider"
	storageprovider "github.com/juju/juju/storage/provider"
)

func init() {
	p := manualProvider{}
	environs.RegisterProvider(provider.Manual, p, "null")

	storageprovider.RegisterEnvironStorageProviders(provider.Manual)
}
