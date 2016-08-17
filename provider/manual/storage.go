// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (*manualEnviron) StorageProviderTypes() []storage.ProviderType {
	return nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*manualEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}
