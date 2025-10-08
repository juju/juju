// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/storage"
)

// RecommendedPoolForKind returns the recommended storage pool to use for
// the given storage kind. If no pool can be recommended nil is returned.
//
// Implements [storage.PoolAdvisor] interface.
func (*manualEnviron) RecommendedPoolForKind(
	kind storage.StorageKind,
) *storage.Config {
	return common.GetCommonRecommendedIAASPoolForKind(kind)
}

// StorageProviderTypes implements storage.ProviderRegistry.
func (*manualEnviron) StorageProviderTypes() ([]storage.ProviderType, error) {
	return common.CommonIAASStorageProviderTypes(), nil
}

// StorageProvider implements storage.ProviderRegistry.
func (*manualEnviron) StorageProvider(t storage.ProviderType) (storage.Provider, error) {
	return common.GetCommonIAASStorageProvider(t)
}
