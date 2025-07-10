// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	storagetesting "github.com/juju/juju/core/storage/testing"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
)

// storageProviderSuite provides a set of tests for validating
// [DefaultStorageProviderValidator].
type storageProviderSuite struct {
	provider *MockStorageProvider
	registry *MockProviderRegistry
	state    *MockStorageProviderState
}

type storageSuite struct {
	testhelpers.IsolationSuite

	mockState *MockState

	service *Service
}

func TestStorageProviderSuite(t *testing.T) {
	tc.Run(t, &storageProviderSuite{})
}

func TestStorageSuite(t *testing.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageProviderSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.provider = NewMockStorageProvider(ctrl)
	s.registry = NewMockProviderRegistry(ctrl)
	s.state = NewMockStorageProviderState(ctrl)

	c.Cleanup(func() {
		s.provider = nil
		s.registry = nil
		s.state = nil
	})
	return ctrl
}

func (s *storageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.service = NewService(
		s.mockState,
		nil,
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	return ctrl
}

// GetStorageRegistry returns the [storageProviderSuite.registry] mock. This
// func implements the [corestorage.ModelStorageRegistryGetter] interface for
// the purpose of testing.
func (s *storageProviderSuite) GetStorageRegistry(
	_ context.Context,
) (internalstorage.ProviderRegistry, error) {
	return s.registry, nil
}

func (s *storageSuite) TestAttachStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), storageUUID, unitUUID)

	err := s.service.AttachStorage(c.Context(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestAttachStorageAlreadyAttached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), storageUUID, unitUUID).Return(errors.StorageAlreadyAttached)

	err := s.service.AttachStorage(c.Context(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestAttachStorageValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.AttachStorage(c.Context(), "pgdata/0", "666")
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	err = s.service.AttachStorage(c.Context(), "0", "postgresql/666")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestAddStorageToUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	stor := internalstorage.Directive{}
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().AddStorageForUnit(gomock.Any(), corestorage.Name("pgdata"), unitUUID, stor).Return([]corestorage.ID{"pgdata/0"}, nil)

	result, err := s.service.AddStorageForUnit(c.Context(), "pgdata", "postgresql/666", stor)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []corestorage.ID{"pgdata/0"})
}

func (s *storageSuite) TestAddStorageForUnitValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.AddStorageForUnit(c.Context(), "pgdata", "666", internalstorage.Directive{})
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	_, err = s.service.AddStorageForUnit(c.Context(), "0", "postgresql/666", internalstorage.Directive{})
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageName)
}

func (s *storageSuite) TestDetachStorageForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().DetachStorageForUnit(gomock.Any(), storageUUID, unitUUID)

	err := s.service.DetachStorageForUnit(c.Context(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageForUnitValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorageForUnit(c.Context(), "pgdata/0", "666")
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	err = s.service.DetachStorageForUnit(c.Context(), "0", "postgresql/666")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestDetachStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().DetachStorage(gomock.Any(), storageUUID)

	err := s.service.DetachStorage(c.Context(), "pgdata/0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorage(c.Context(), "0")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}

// TestPoolSupportsCharmStorageNotFound tests that if no storage pool exists for
// a given storage pool uuid the caller gets back an error satisfying
// [storageerrors.PoolNotFoundError].
func (s *storageProviderSuite) TestPoolSupportsCharmStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetProviderTypeOfPool(gomock.Any(), poolUUID).Return(
		"", storageerrors.PoolNotFoundError,
	)

	validator := NewStorageProviderValidator(s, s.state)
	_, err = validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
}

// TestPoolSupportsCharmStorageFilesystem tests that the storage pool exists
// and supports charm filesystem storage.
func (s *storageProviderSuite) TestPoolSupportsCharmStorageFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetProviderTypeOfPool(gomock.Any(), poolUUID).Return(
		"testprovider", nil,
	)
	s.registry.EXPECT().StorageProvider(internalstorage.ProviderType("testprovider")).Return(
		s.provider, nil,
	)
	s.provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true)

	validator := NewStorageProviderValidator(s, s.state)
	supports, err := validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}

// TestPoolSupportsCharmStorageBlockdevice tests that the storage pool exists
// and supports charm blockdevice storage.
func (s *storageProviderSuite) TestPoolSupportsCharmStorageBlockdevice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetProviderTypeOfPool(gomock.Any(), poolUUID).Return(
		"testprovider", nil,
	)
	s.registry.EXPECT().StorageProvider(internalstorage.ProviderType("testprovider")).Return(
		s.provider, nil,
	)
	s.provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true)

	validator := NewStorageProviderValidator(s, s.state)
	supports, err := validator.CheckPoolSupportsCharmStorage(
		c.Context(), poolUUID, internalcharm.StorageBlock,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}

// TestPoolSupportsCharmStorageNotSupported tests that if no provider exists for
// the supplied provider type the caller gets back an error satisfying
// [storageerrors.ProviderTypeNotFound].
func (s *storageProviderSuite) TestProviderTypeSupportsCharmStorageNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerType := internalstorage.ProviderType("testprovider")
	s.registry.EXPECT().StorageProvider(providerType).Return(
		nil, storageerrors.ProviderTypeNotFound,
	)

	validator := NewStorageProviderValidator(s, s.state)
	_, err := validator.CheckProviderTypeSupportsCharmStorage(
		c.Context(), "testprovider", internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIs, storageerrors.ProviderTypeNotFound)
}

// TestProviderTypeSupportsCharmStorageFilesystem tests that the provider type
// supports charm filesystem storage.
func (s *storageProviderSuite) TestProviderTypeSupportsCharmStorageFilesystem(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerType := internalstorage.ProviderType("testprovider")
	s.registry.EXPECT().StorageProvider(providerType).Return(
		s.provider, nil,
	)
	s.provider.EXPECT().Supports(internalstorage.StorageKindFilesystem).Return(true)

	validator := NewStorageProviderValidator(s, s.state)
	supports, err := validator.CheckProviderTypeSupportsCharmStorage(
		c.Context(), "testprovider", internalcharm.StorageFilesystem,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}

// TestProviderTypeSupportsCharmStorageBlockdevice tests that the provider type
// supports charm blockdevice storage.
func (s *storageProviderSuite) TestProviderTypeSupportsCharmStorageBlockdevice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	providerType := internalstorage.ProviderType("testprovider")
	s.registry.EXPECT().StorageProvider(providerType).Return(
		s.provider, nil,
	)
	s.provider.EXPECT().Supports(internalstorage.StorageKindBlock).Return(true)

	validator := NewStorageProviderValidator(s, s.state)
	supports, err := validator.CheckProviderTypeSupportsCharmStorage(
		c.Context(), "testprovider", internalcharm.StorageBlock,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(supports, tc.IsTrue)
}
