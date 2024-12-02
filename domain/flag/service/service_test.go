// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSetFlag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetFlag(gomock.Any(), "foo", true, "foo set to true").Return(nil)

	service := NewService(s.state)
	err := service.SetFlag(context.Background(), "foo", true, "foo set to true")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetFlag(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFlag(gomock.Any(), "foo").Return(true, nil)

	service := NewService(s.state)
	value, err := service.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(value, jc.IsTrue)
}

func (s *serviceSuite) TestGetFlagNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFlag(gomock.Any(), "foo").Return(false, errors.Errorf("flag %w", coreerrors.NotFound))

	service := NewService(s.state)
	value, err := service.GetFlag(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(value, jc.IsFalse)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
