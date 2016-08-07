// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package joyent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/storage"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (*joyentEnviron) StorageProviderTypes() []storage.ProviderType {
	return nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*joyentEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return nil, errors.NotFoundf("storage provider %q", t)
}
