// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	testing.IsolationSuite

	state *MockState
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestSetFlag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SetFlag(gomock.Any(), "foo", true, "foo set to true").Return(nil)

	service := NewService(s.state)
	err := service.SetFlag(context.Background(), "foo", true, "foo set to true")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetFlag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFlag(gomock.Any(), "foo").Return(true, nil)

	service := NewService(s.state)
	value, err := service.GetFlag(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.IsTrue)
}

func (s *serviceSuite) TestGetFlagNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetFlag(gomock.Any(), "foo").Return(false, errors.Errorf("flag %w", coreerrors.NotFound))

	service := NewService(s.state)
	value, err := service.GetFlag(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(value, tc.IsFalse)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)

	return ctrl
}
