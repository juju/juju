// Copyright 2024 Canonical Ltd.
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

type providerServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) TestCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := &cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(one, nil)

	result, err := NewWatchableProviderService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, one)
}

func (s *providerServiceSuite) TestCloudNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(nil, errors.NotFoundf(`cloud "fluffy"`))

	result, err := NewWatchableProviderService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, gc.ErrorMatches, `cloud "fluffy" not found`)
	c.Check(err, jc.ErrorIs, errors.NotFound)
	c.Check(result, gc.IsNil)
}

func (s *providerServiceSuite) TestWatchCloud(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchCloud(gomock.Any(), gomock.Any(), "cirrus").Return(nw, nil)

	w, err := NewWatchableProviderService(s.state, s.watcherFactory).WatchCloud(context.Background(), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)
}
