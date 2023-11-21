// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher/watchertest"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetPath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()

	s.state.EXPECT().GetPath(gomock.Any(), key).Return(path, nil)

	p, err := NewService(s.state, nil).GetPath(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.Equals, path)
}

func (s *serviceSuite) TestListPaths(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()
	paths := map[string]string{key: path}

	s.state.EXPECT().ListPaths(gomock.Any()).Return(paths, nil)

	p, err := NewService(s.state, nil).ListPaths(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, paths)
}

func (s *serviceSuite) TestPutPath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()

	s.state.EXPECT().PutPath(gomock.Any(), key, path).Return(nil)

	err := NewService(s.state, nil).PutPath(context.Background(), key, path)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemovePath(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()

	s.state.EXPECT().RemovePath(gomock.Any(), key).Return(nil)

	err := NewService(s.state, nil).RemovePath(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

// Test watch returns a watcher that watches the specified path.
func (s *serviceSuite) TestWatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	watcher := watchertest.NewMockStringsWatcher(nil)
	defer workertest.DirtyKill(c, watcher)

	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(stmt)
	s.watcherFactory.EXPECT().NewNamespaceWatcher("objectstore", changestream.All, stmt).Return(watcher, nil)

	w, err := NewService(s.state, s.watcherFactory).Watch()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}
