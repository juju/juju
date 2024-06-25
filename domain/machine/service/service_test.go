// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/life"
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

	s.state.EXPECT().CreateMachine(gomock.Any(), "666", gomock.Any(), gomock.Any()).Return(nil)

	_, err := NewService(s.state).CreateMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().CreateMachine(gomock.Any(), "666", gomock.Any(), gomock.Any()).Return(rErr)

	_, err := NewService(s.state).CreateMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `creating machine "666": boom`)
}

func (s *serviceSuite) TestDeleteMachineSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), "666").Return(nil)

	err := NewService(s.state).DeleteMachine(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteMachineError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), "666").Return(rErr)

	err := NewService(s.state).DeleteMachine(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting machine "666": boom`)
}

func (s *serviceSuite) TestGetLifeSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	life := life.Alive
	s.state.EXPECT().GetMachineLife(gomock.Any(), "666").Return(&life, nil)

	l, err := NewService(s.state).GetMachineLife(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(l, gc.Equals, &life)
}

func (s *serviceSuite) TestGetLifeError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().GetMachineLife(gomock.Any(), "666").Return(nil, rErr)

	l, err := NewService(s.state).GetMachineLife(context.Background(), "666")
	c.Check(l, gc.IsNil)
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `getting life status for machine "666": boom`)
}
