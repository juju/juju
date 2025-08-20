// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"errors"

	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
)

// StorageProviderValidator is a testing implementation of
// [github.com/juju/juju/domain/application/service.StorageProviderValidator].
//
// This is done as there exists several integration tests that use the
// application service package for testing. To create a proper validator forces
// a lot of external concerns onto the test that it should not have to deal
// with.
//
// This implementation currently returns errors for all func calls until such
// time as more logic is required.
type StorageProviderValidator struct{}

// CheckPoolSupportsCharmStorage returns false and a not implemented error.
func (s StorageProviderValidator) CheckPoolSupportsCharmStorage(
	_ context.Context,
	_ domainstorage.StoragePoolUUID,
	_ internalcharm.StorageType,
) (bool, error) {
	return false, errors.New("CheckPoolSupportsCharmStorage not implemented")
}
