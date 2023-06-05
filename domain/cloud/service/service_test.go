// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/domain/cloud/state"
)

type serviceSuite struct {
	testing.IsolationSuite
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *serviceSuite) TestUpdateSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Save(gomock.Any(), cloud).Return(nil)

	err := NewService(s.state).Save(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Save(gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewService(s.state).Save(context.Background(), cloud)
	c.Assert(err, gc.ErrorMatches, "updating cloud state: boom")
}

func (s *serviceSuite) TestListAll(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clouds := []cloud.Cloud{{
		Name: "fluffy",
	}}
	s.state.EXPECT().List(gomock.Any(), nil).Return(clouds, nil)

	result, err := NewService(s.state).ListAll(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, clouds)
}

func (s *serviceSuite) TestGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().List(gomock.Any(), &state.Filter{Name: "fluffy"}).Return([]cloud.Cloud{one}, nil)

	result, err := NewService(s.state).Get(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &one)
}

func (s *serviceSuite) TestGetNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().List(gomock.Any(), &state.Filter{Name: "fluffy"}).Return(nil, nil)

	result, err := NewService(s.state).Get(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, `cloud "fluffy" not found`)
	c.Assert(result, gc.IsNil)
}
