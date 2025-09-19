// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"errors"

	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	internalstorage "github.com/juju/juju/internal/storage"
)

// StoragePoolProvider is a testing implementation of
// [github.com/juju/juju/domain/application/service.StoragePoolProvider].
//
// This is done as there exists several integration tests that use the
// application service package for testing. To create a proper validator forces
// a lot of external concerns onto the test that it should not have to deal
// with.
//
// This implementation currently returns errors for all func calls until such
// time as more logic is required.
type StoragePoolProvider struct{}

// CheckPoolSupportsCharmStorage returns false and a not implemented error.
func (s StoragePoolProvider) CheckPoolSupportsCharmStorage(
	_ context.Context,
	_ domainstorage.StoragePoolUUID,
	_ internalcharm.StorageType,
) (bool, error) {
	return false, errors.New("CheckPoolSupportsCharmStorage not implemented")
}

// GetProviderForPool returns nil and a not implemented error.
func (s StoragePoolProvider) GetProviderForPool(
	_ context.Context,
	_ domainstorage.StoragePoolUUID,
) (internalstorage.Provider, error) {
	return nil, errors.New("GetProviderForPool not implemented")
}
