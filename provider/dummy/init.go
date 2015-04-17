// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	dummystorage "github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/storage/provider/registry"
)

func init() {
	registry.RegisterEnvironStorageProviders("dummy", "dummy")
	registry.RegisterProvider("dummy", &dummystorage.StorageProvider{})
}
