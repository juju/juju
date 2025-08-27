// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
	internalprovider "github.com/juju/juju/internal/storage/provider"
)

// CommonIAASStorageProviderTypes returns common storage provider types that are
// available through IAAS environs.
func CommonIAASStorageProviderTypes() []internalstorage.ProviderType {
	return []internalstorage.ProviderType{
		internalprovider.TmpfsProviderType,
		internalprovider.RootfsProviderType,
		internalprovider.LoopProviderType,
	}
}

// GetCommonIAASStorageProvider returns a storage provider for the supplied
// provider type.
//
// If no storage provider is available for the supplied type the caller should
// expect back an error satisfying [coreerrors.NotFound].
func GetCommonIAASStorageProvider(t internalstorage.ProviderType) (
	internalstorage.Provider, error,
) {
	switch t {
	case internalprovider.TmpfsProviderType:
		return internalprovider.NewTmpfsProvider(internalprovider.LogAndExec), nil
	case internalprovider.RootfsProviderType:
		return internalprovider.NewRootfsProvider(internalprovider.LogAndExec), nil
	case internalprovider.LoopProviderType:
		return internalprovider.NewLoopProvider(internalprovider.LogAndExec), nil
	default:
		return nil, errors.Errorf(
			"no storage provider exists for type %q", t,
		).Add(coreerrors.NotFound)
	}
}
