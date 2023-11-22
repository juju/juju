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
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()

	metadata := objectstore.Metadata{
		UUID: utils.MustNewUUID().String(),
		Key:  key,
		Path: path,
		Hash: utils.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().GetMetadata(gomock.Any(), key).Return(metadata, nil)

	p, err := NewService(s.state, nil).GetMetadata(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestGetAllMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()
	metadatas := map[string]objectstore.Metadata{
		key: {
			UUID: utils.MustNewUUID().String(),
			Key:  key,
			Path: path,
			Hash: utils.MustNewUUID().String(),
			Size: 666,
		},
	}

	s.state.EXPECT().GetAllMetadata(gomock.Any()).Return(metadatas, nil)

	p, err := NewService(s.state, nil).GetAllMetadata(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, metadatas)
}

func (s *serviceSuite) TestPutMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()
	path := utils.MustNewUUID().String()
	metadata := objectstore.Metadata{
		UUID: utils.MustNewUUID().String(),
		Key:  key,
		Path: path,
		Hash: utils.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().PutMetadata(gomock.Any(), key, metadata).Return(nil)

	err := NewService(s.state, nil).PutMetadata(context.Background(), key, metadata)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := utils.MustNewUUID().String()

	s.state.EXPECT().RemoveMetadata(gomock.Any(), key).Return(nil)

	err := NewService(s.state, nil).RemoveMetadata(context.Background(), key)
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
