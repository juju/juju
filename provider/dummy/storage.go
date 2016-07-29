// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dummy

import (
	"github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
)

func StorageProviders() storage.ProviderRegistry {
	return testing.StorageProviders()
}
