// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/watcher/watchertest"
)

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestUpsertCloudSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpsertCloud(gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpsertCloud(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpsertCloudError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpsertCloud(gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpsertCloud(context.Background(), cloud)
	c.Assert(err, gc.ErrorMatches, `updating cloud "fluffy": boom`)
}

func (s *serviceSuite) TestDeleteCloudSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteCloud(gomock.Any(), "fluffy").Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).DeleteCloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestListAll(c *gc.C) {
	defer s.setupMocks(c).Finish()

	clouds := []cloud.Cloud{{
		Name: "fluffy",
	}}
	s.state.EXPECT().ListClouds(gomock.Any()).Return(clouds, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).ListAll(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, clouds)
}

func (s *serviceSuite) TestCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := &cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(one, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, one)
}

func (s *serviceSuite) TestCloudNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(nil, errors.NotFoundf(`cloud "fluffy"`))

	result, err := NewWatchableService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, `cloud "fluffy" not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(result, gc.IsNil)
}

func (s *serviceSuite) TestWatchCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchCloud(gomock.Any(), gomock.Any(), "cirrus").Return(nw, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).WatchCloud(context.Background(), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)
}
