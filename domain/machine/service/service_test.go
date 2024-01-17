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

	s.state.EXPECT().UpsertMachine(gomock.Any(), "666").Return(nil)

	err := NewService(s.state).Save(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().UpsertMachine(gomock.Any(), "666").Return(rErr)

	err := NewService(s.state).Save(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `saving machine "666": boom`)
}

func (s *serviceSuite) TestDeleteSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteMachine(gomock.Any(), "666").Return(nil)

	err := NewService(s.state).Delete(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteMachine(gomock.Any(), "666").Return(rErr)

	err := NewService(s.state).Delete(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting machine "666": boom`)
}
