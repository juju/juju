// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
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

	uuid := unit.MustNewUUID()
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().AttachStorage(gomock.Any(), storage.ID("pgdata/0"), uuid)

	err := s.service.AttachStorage(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestAddStorageToUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unit.MustNewUUID()
	stor := storage.Directive{}
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().AddStorageForUnit(gomock.Any(), storage.Name("pgdata"), uuid, stor).Return([]storage.ID{"pgdata/0"}, nil)

	result, err := s.service.AddStorageForUnit(context.Background(), "pgdata", "postgresql/666", stor)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []storage.ID{"pgdata/0"})
}

func (s *storageSuite) TestDetachStorageForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := unit.MustNewUUID()
	s.mockState.EXPECT().GetUnitUUIDByName(gomock.Any(), unit.Name("postgresql/666")).Return(uuid, nil)
	s.mockState.EXPECT().DetachStorageForUnit(gomock.Any(), storage.ID("pgdata/0"), uuid)

	err := s.service.DetachStorageForUnit(context.Background(), "pgdata/0", "postgresql/666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *storageSuite) TestDetachStorage(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().DetachStorage(gomock.Any(), storage.ID("pgdata/0"))

	err := s.service.DetachStorage(context.Background(), "pgdata/0")
	c.Assert(err, jc.ErrorIsNil)
}
