// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	corestorage "github.com/juju/juju/core/storage"
	storagetesting "github.com/juju/juju/core/storage/testing"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
)

type storageSuite struct {
	testing.IsolationSuite

	mockState *MockState

	service *Service
}

var _ = tc.Suite(&storageSuite{})

func (s *storageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)
	s.service = NewService(
		s.mockState,
		nil,
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return provider.CommonStorageProviders()
		}),
		nil,
		domain.NewStatusHistory(loggertesting.WrapCheckLog(c), clock.WallClock),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
	return ctrl
}

func (s *storageSuite) TestAttachStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), application.StorageParentDir, storageUUID, unitUUID)

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestAttachStorageAlreadyAttached(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), application.StorageParentDir, storageUUID, unitUUID).Return(errors.StorageAlreadyAttached)

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestAttachStorageValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "666")
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	err = s.service.AttachStorage(context.Background(), "0", "postgresql/666")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestAddStorageToUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	stor := storage.Directive{}
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().AddStorageForUnit(gomock.Any(), corestorage.Name("pgdata"), unitUUID, stor).Return([]corestorage.ID{"pgdata/0"}, nil)

	result, err := s.service.AddStorageForUnit(context.Background(), "pgdata", "postgresql/666", stor)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, []corestorage.ID{"pgdata/0"})
}

func (s *storageSuite) TestAddStorageForUnitValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.AddStorageForUnit(context.Background(), "pgdata", "666", storage.Directive{})
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	_, err = s.service.AddStorageForUnit(context.Background(), "0", "postgresql/666", storage.Directive{})
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageName)
}

func (s *storageSuite) TestDetachStorageForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(unitUUID, nil)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().DetachStorageForUnit(gomock.Any(), storageUUID, unitUUID)

	err := s.service.DetachStorageForUnit(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageForUnitValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorageForUnit(context.Background(), "pgdata/0", "666")
	c.Assert(err, tc.ErrorIs, unit.InvalidUnitName)
	err = s.service.DetachStorageForUnit(context.Background(), "0", "postgresql/666")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestDetachStorage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	storageUUID := storagetesting.GenStorageUUID(c)
	s.mockState.EXPECT().GetStorageUUIDByID(gomock.Any(), corestorage.ID("pgdata/0")).Return(storageUUID, nil)
	s.mockState.EXPECT().DetachStorage(gomock.Any(), storageUUID)

	err := s.service.DetachStorage(context.Background(), "pgdata/0")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorage(context.Background(), "0")
	c.Assert(err, tc.ErrorIs, corestorage.InvalidStorageID)
}
