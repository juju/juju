// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/storage"
	storageprovider "github.com/juju/juju/internal/storage/provider"
)

// StorageProviderTypes implements storage.ProviderRegistry.
func (*manualEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return []storage.ProviderType{
		storageprovider.TmpfsProviderType,
		storageprovider.RootfsProviderType,
		storageprovider.LoopProviderType,
	}, nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*manualEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	switch t {
	case storageprovider.TmpfsProviderType:
		return storageprovider.NewTmpfsProvider(storageprovider.LogAndExec), nil
	case storageprovider.RootfsProviderType:
		return storageprovider.NewRootfsProvider(storageprovider.LogAndExec), nil
	case storageprovider.LoopProviderType:
		return storageprovider.NewLoopProvider(storageprovider.LogAndExec), nil
	default:
		return nil, errors.NotFoundf("storage provider %q", t)
	}
}
