// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/watcher/watchertest"
)

type serviceSuite struct {
	testing.IsolationSuite
	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)
	return ctrl
}

func (s *serviceSuite) TestUpdateSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpsertCloud(gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).Save(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpsertCloud(gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).Save(context.Background(), cloud)
	c.Assert(err, gc.ErrorMatches, `updating cloud "fluffy": boom`)
}

func (s *serviceSuite) TestListAll(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clouds := []cloud.Cloud{{
		Name: "fluffy",
	}}
	s.state.EXPECT().ListClouds(gomock.Any(), "").Return(clouds, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).ListAll(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, clouds)
}

func (s *serviceSuite) TestGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().ListClouds(gomock.Any(), "fluffy").Return([]cloud.Cloud{one}, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).Get(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, &one)
}

func (s *serviceSuite) TestGetNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().ListClouds(gomock.Any(), "fluffy").Return(nil, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).Get(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, `cloud "fluffy" not found`)
	c.Assert(result, gc.IsNil)
}

func (s *serviceSuite) TestWatchCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchCloud(gomock.Any(), gomock.Any(), "cirrus").Return(nw, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).WatchCloud(context.Background(), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}
