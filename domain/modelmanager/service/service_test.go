// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestServiceCreate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Create(gomock.Any(), uuid.String())

	svc := NewService(s.state)
	err := svc.Create(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceCreateInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state)
	err := svc.Create(context.TODO(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) TestServiceDelete(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := mustUUID(c)

	s.state.EXPECT().Delete(gomock.Any(), uuid.String())

	svc := NewService(s.state)
	err := svc.Delete(context.TODO(), uuid)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestServiceDeleteInvalidUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.state)
	err := svc.Delete(context.TODO(), "invalid")
	c.Assert(err, gc.ErrorMatches, "validating model uuid.*")
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}

func mustUUID(c *gc.C) UUID {
	return UUID(utils.MustNewUUID().String())
}
