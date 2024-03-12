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

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/domain"
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

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.dbDeleter = NewMockDBDeleter(ctrl)

	return ctrl
}
