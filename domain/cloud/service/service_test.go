// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	baseSuite
}

var _ = tc.Suite(&serviceSuite{})

func (s *serviceSuite) TestCreateCloudSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().CreateCloud(gomock.Any(), usertesting.GenNewName(c, "owner-name"), gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).CreateCloud(context.Background(), usertesting.GenNewName(c, "owner-name"), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCreateCloudFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().CreateCloud(gomock.Any(), usertesting.GenNewName(c, "owner-name"), gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).CreateCloud(context.Background(), usertesting.GenNewName(c, "owner-name"), cloud)
	c.Assert(err, tc.ErrorMatches, `creating cloud "fluffy": boom`)
}

func (s *serviceSuite) TestUpdateCloudSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpdateCloud(gomock.Any(), cloud).Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).UpdateCloud(context.Background(), cloud)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateCloudError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cloud := cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().UpdateCloud(gomock.Any(), cloud).Return(errors.New("boom"))

	err := NewWatchableService(s.state, s.watcherFactory).UpdateCloud(context.Background(), cloud)
	c.Assert(err, tc.ErrorMatches, `updating cloud "fluffy": boom`)
}

func (s *serviceSuite) TestDeleteCloudSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().DeleteCloud(gomock.Any(), "fluffy").Return(nil)

	err := NewWatchableService(s.state, s.watcherFactory).DeleteCloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestListAll(c *tc.C) {
	defer s.setupMocks(c).Finish()

	clouds := []cloud.Cloud{{
		Name: "fluffy",
	}}
	s.state.EXPECT().ListClouds(gomock.Any()).Return(clouds, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).ListAll(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, clouds)
}

func (s *serviceSuite) TestCloud(c *tc.C) {
	defer s.setupMocks(c).Finish()

	one := &cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(one, nil)

	result, err := NewWatchableService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, one)
}

func (s *serviceSuite) TestCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(nil, errors.Errorf(`cloud "fluffy"`+" %w", coreerrors.NotFound))

	result, err := NewWatchableService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, tc.ErrorMatches, `cloud "fluffy" not found`)
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
	c.Check(result, tc.IsNil)
}

func (s *serviceSuite) TestWatchCloud(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchCloud(gomock.Any(), gomock.Any(), "cirrus").Return(nw, nil)

	w, err := NewWatchableService(s.state, s.watcherFactory).WatchCloud(context.Background(), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, tc.NotNil)
}
