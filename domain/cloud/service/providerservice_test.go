// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/errors"
)

type providerServiceSuite struct {
	baseSuite
}

var _ = tc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) TestCloud(c *tc.C) {
	defer s.setupMocks(c).Finish()

	one := &cloud.Cloud{
		Name: "fluffy",
	}
	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(one, nil)

	result, err := NewWatchableProviderService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, one)
}

func (s *providerServiceSuite) TestCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().Cloud(gomock.Any(), "fluffy").Return(nil, errors.Errorf(`cloud "fluffy"`+" %w", coreerrors.NotFound))

	result, err := NewWatchableProviderService(s.state, s.watcherFactory).Cloud(context.Background(), "fluffy")
	c.Assert(err, tc.ErrorMatches, `cloud "fluffy" not found`)
	c.Check(err, jc.ErrorIs, coreerrors.NotFound)
	c.Check(result, tc.IsNil)
}

func (s *providerServiceSuite) TestWatchCloud(c *tc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	s.state.EXPECT().WatchCloud(gomock.Any(), gomock.Any(), "cirrus").Return(nw, nil)

	w, err := NewWatchableProviderService(s.state, s.watcherFactory).WatchCloud(context.Background(), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, tc.NotNil)
}
