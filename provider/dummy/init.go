// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/juju/environs"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
)

func init() {
	registry.RegisterEnvironStorageProviders("dummy", "dummy")
	registry.RegisterProvider("dummy", &dummystorage.StorageProvider{})

	// Register cloud local storage as data source
	environs.RegisterImageDataSourceFunc("cloud local storage", getCustomImageSource)
}
