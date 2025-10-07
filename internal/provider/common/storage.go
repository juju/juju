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

// GetCommonRecommendedIAASPoolForKind returns the recommended storage pool to use for
// the supplied storage kind. The recommended pool comes from one of the default
// pools provided by the provider types of [CommonIAASStorageProviderTypes].
func GetCommonRecommendedIAASPoolForKind(
	kind internalstorage.StorageKind,
) *internalstorage.Config {
	var defaultPools []*internalstorage.Config

	if kind == internalstorage.StorageKindBlock {
		defaultPools = internalprovider.NewRootfsProvider(
			internalprovider.LogAndExec,
		).DefaultPools()
	} else if kind == internalstorage.StorageKindFilesystem {
		defaultPools = internalprovider.NewLoopProvider(
			internalprovider.LogAndExec,
		).DefaultPools()
	}

	if len(defaultPools) > 0 {
		// Return the first default pool as the recommended pool.
		return defaultPools[0]
	}
	// No default pool exists.
	return nil
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
