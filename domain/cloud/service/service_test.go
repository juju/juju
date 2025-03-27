// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestCreateCloudSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().CreateCloud(gomock.Any(), usertesting.GenNewName(c, "owner-name"), gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).CreateCloud(context.Background(), usertesting.GenNewName(c, "owner-name"), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateCloudFail(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().CreateCloud(gomock.Any(), usertesting.GenNewName(c, "owner-name"), gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).CreateCloud(context.Background(), usertesting.GenNewName(c, "owner-name"), cloud)
	c.Assert(err, gc.ErrorMatches, `creating cloud "fluffy": boom`)
}

func (s *serviceSuite) TestUpdateCloudSuccess(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpdateCloud(gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateCloud(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateCloudError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpdateCloud(gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpdateCloud(context.Background(), cloud)
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

	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(nil, errors.Errorf(`cloud "fluffy"`+" %w", coreerrors.NotFound))

	result, err := NewWatchableService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, `cloud "fluffy" not found`)
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
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

func (s *serviceSuite) TestGetModelCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := modeltesting.GenModelUUID(c)
	one := &cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().GetModelCloud(gomock.Any(), uuid).Return(one, "region", nil)

	cld, region, err := NewWatchableService(s.state, s.watcherFactory).GetModelCloud(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cld, jc.DeepEquals, one)
	c.Assert(region, jc.DeepEquals, "region")
}
