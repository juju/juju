// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/juju/storage"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
)

func StorageProviders() storage.ProviderRegistry {
	return dummystorage.StorageProviders()
}
