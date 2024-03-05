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

	"github.com/juju/juju/domain/application"
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

func ptr[T any](v T) *T {
	return &v
}

func (s *serviceSuite) TestUpdateSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666", u).Return(nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := NewService(s.state).CreateApplication(context.Background(), "666", AddApplicationParams{}, a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().UpsertApplication(gomock.Any(), "666").Return(rErr)

	err := NewService(s.state).CreateApplication(context.Background(), "666", AddApplicationParams{})
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `saving application "666": boom`)
}

func (s *serviceSuite) TestDeleteApplicationSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(nil)

	err := NewService(s.state).DeleteApplication(context.Background(), "666")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestDeleteApplicationError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	rErr := errors.New("boom")
	s.state.EXPECT().DeleteApplication(gomock.Any(), "666").Return(rErr)

	err := NewService(s.state).DeleteApplication(context.Background(), "666")
	c.Check(err, jc.ErrorIs, rErr)
	c.Assert(err, gc.ErrorMatches, `deleting application "666": boom`)
}

func (s *serviceSuite) TestAddUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().AddUnits(gomock.Any(), "666", u).Return(nil)

	a := AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := NewService(s.state).AddUnits(context.Background(), "666", a)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestAddUpsertCAASUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	u := application.AddUnitParams{
		UnitName: ptr("foo/666"),
	}
	s.state.EXPECT().UpsertApplication(gomock.Any(), "foo", u).Return(nil)

	p := UpsertCAASUnitParams{
		UnitName: ptr("foo/666"),
	}
	err := NewService(s.state).UpsertCAASUnit(context.Background(), "foo", p)
	c.Assert(err, jc.ErrorIsNil)
}
