// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/storage"
)

type storageSuite struct {
	testing.IsolationSuite

	mockState *MockState

	service *Service
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockState = NewMockState(ctrl)

	s.service = &Service{
		st:     s.mockState,
		logger: loggertesting.WrapCheckLog(c),
	}
	return ctrl
}

func (s *storageSuite) TestAttachStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), corestorage.ID("pgdata/0"), uuid)

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestAttachStorageValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "666")
	c.Assert(err, jc.ErrorIs, unit.InvalidUnitName)
	err = s.service.AttachStorage(context.Background(), "0", "postgresql/666")
	c.Assert(err, jc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestAddStorageToUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	stor := storage.Directive{}
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().AddStorageForUnit(gomock.Any(), corestorage.Name("pgdata"), uuid, stor).Return([]corestorage.ID{"pgdata/0"}, nil)

	result, err := s.service.AddStorageForUnit(context.Background(), "pgdata", "postgresql/666", stor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []corestorage.ID{"pgdata/0"})
}

func (s *storageSuite) TestAddStorageForUnitValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.service.AddStorageForUnit(context.Background(), "pgdata", "666", storage.Directive{})
	c.Assert(err, jc.ErrorIs, unit.InvalidUnitName)
	_, err = s.service.AddStorageForUnit(context.Background(), "0", "postgresql/666", storage.Directive{})
	c.Assert(err, jc.ErrorIs, corestorage.InvalidStorageName)
}

func (s *storageSuite) TestDetachStorageForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unittesting.GenUnitUUID(c)
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().DetachStorageForUnit(gomock.Any(), corestorage.ID("pgdata/0"), uuid)

	err := s.service.DetachStorageForUnit(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageForUnitValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorageForUnit(context.Background(), "pgdata/0", "666")
	c.Assert(err, jc.ErrorIs, unit.InvalidUnitName)
	err = s.service.DetachStorageForUnit(context.Background(), "0", "postgresql/666")
	c.Assert(err, jc.ErrorIs, corestorage.InvalidStorageID)
}

func (s *storageSuite) TestDetachStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().DetachStorage(gomock.Any(), corestorage.ID("pgdata/0"))

	err := s.service.DetachStorage(context.Background(), "pgdata/0")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorageValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.service.DetachStorage(context.Background(), "0")
	c.Assert(err, jc.ErrorIs, corestorage.InvalidStorageID)
}
