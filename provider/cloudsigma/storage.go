// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v2/storage"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (*environ) StorageProviderTypes() ([]storage.ProviderType, error) {
	return nil, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*environ) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}
