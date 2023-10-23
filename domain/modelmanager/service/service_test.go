// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modeltesting "github.com/juju/juju/domain/model/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state     *MockState
	dbDeleter *MockDBDeleter
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestCreate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)
	s.state.EXPECT().Create(gomock.Any(), uuid).Return(nil)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, `creating model ".*": boom`)
}

func (s *serviceSuite) TestCreateDuplicateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid).Return(sqlite3.Error{
		ExtendedCode: sqlite3.ErrConstraintUnique,
	})

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, "creating model .*: record already exists")
	c.Assert(errors.Cause(err), jc.ErrorIs, domain.ErrDuplicate)
}

func (s *serviceSuite) TestCreateInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Create(context.Background(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) TestModelList(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().List(gomock.Any()).Return([]model.UUID{uuid}, nil)

	svc := NewService(s.state, s.dbDeleter)
	uuids, err := svc.ModelList(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuids, gc.DeepEquals, []model.UUID{uuid})
}

func (s *serviceSuite) TestDelete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(nil)
	s.dbDeleter.EXPECT().DeleteDB(uuid.String()).Return(nil)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteStateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, `deleting model ".*": boom`)
}

func (s *serviceSuite) TestDeleteNoRecordsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(domain.ErrNoRecord)

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("no records should be idempotent"))
}

func (s *serviceSuite) TestDeleteStateSqliteError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(sqlite3.Error{
		Code:         sqlite3.ErrPerm,
		ExtendedCode: sqlite3.ErrCorruptVTab,
	})

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, `deleting model ".*": access permission denied`)
}

func (s *serviceSuite) TestDeleteManagerError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid).Return(nil)
	s.dbDeleter.EXPECT().DeleteDB(uuid.String()).Return(fmt.Errorf("boom"))

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), uuid)
	c.Assert(err, gc.ErrorMatches, `stopping model ".*": boom`)
}

func (s *serviceSuite) TestDeleteInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state, s.dbDeleter)
	err := svc.Delete(context.Background(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.dbDeleter = NewMockDBDeleter(ctrl)

	return ctrl
}
