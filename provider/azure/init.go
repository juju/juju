// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider"
	storageprovider "github.com/juju/juju/storage/provider"
)

func init() {
	environs.RegisterProvider("azure", azureEnvironProvider{})

	storageprovider.RegisterEnvironStorageProviders(provider.Azure)
}
