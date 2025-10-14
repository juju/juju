// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	internalstorage "github.com/juju/juju/internal/storage"
)

type storagePoolProviderSuite struct {
	registry *MockProviderRegistry
	state    *MockProviderState
}

func TestStoragePoolProviderSuite(t *testing.T) {
	tc.Run(t, &storagePoolProviderSuite{})
}

// GetStorageRegistry returns the [storageProviderSuite.registry] mock. This
// func implements the [corestorage.ModelStorageRegistryGetter] interface for
// the purpose of testing.
func (s *storagePoolProviderSuite) GetStorageRegistry(
	_ context.Context,
) (internalstorage.ProviderRegistry, error) {
	return s.registry, nil
}

func (s *storagePoolProviderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.registry = NewMockProviderRegistry(ctrl)
	s.state = NewMockProviderState(ctrl)

	c.Cleanup(func() {
		s.registry = nil
		s.state = nil
	})
	return ctrl
}

// TestPoolSupportsCharmStorageNotFound tests that if no storage pool exists for
// a given storage pool uuid the caller gets back an error satisfying
// [storageerrors.PoolNotFoundError].
func (s *storagePoolProviderSuite) TestPoolSupportsCharmStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	s.state.EXPECT().GetProviderTypeForPool(gomock.Any(), poolUUID).Return(
		"", storageerrors.PoolNotFoundError,
	)

	validator := NewStoragePoolProvider(s, s.state)
	_, err := validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
}

// TestPoolSupportsCharmStorageFilesystem tests that the storage pool exists
// and supports charm filesystem storage.
func (s *storagePoolProviderSuite) TestPoolSupportsCharmStorageFilesystem(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	provider := NewMockStorageProvider(ctrl)
	s.state.EXPECT().GetProviderTypeForPool(gomock.Any(), poolUUID).Return(
		"testprovider", nil,
	)
	s.registry.EXPECT().StorageProvider(internalstorage.ProviderType("testprovider")).Return(
		provider, nil,
	)
	provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true)

	validator := NewStoragePoolProvider(s, s.state)
	supports, err := validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}

// TestPoolSupportsCharmStorageBlockdevice tests that the storage pool exists
// and supports charm blockdevice storage.
func (s *storagePoolProviderSuite) TestPoolSupportsCharmStorageBlockdevice(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)

	provider := NewMockStorageProvider(ctrl)
	s.state.EXPECT().GetProviderTypeForPool(gomock.Any(), poolUUID).Return(
		"testprovider", nil,
	)
	s.registry.EXPECT().StorageProvider(internalstorage.ProviderType("testprovider")).Return(
		provider, nil,
	)
	provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true)

	validator := NewStoragePoolProvider(s, s.state)
	supports, err := validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageBlock,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}
