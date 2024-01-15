// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/objectstore"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestGetMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := utils.MustNewUUID().String()

	metadata := coreobjectstore.Metadata{
		Path: path,
		Hash: utils.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().GetMetadata(gomock.Any(), path).Return(objectstore.Metadata{
		Path: metadata.Path,
		Size: metadata.Size,
		Hash: metadata.Hash,
	}, nil)

	p, err := NewService(s.state, nil).GetMetadata(context.Background(), path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p, gc.DeepEquals, metadata)
}

func (s *serviceSuite) TestPutMetadata(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := utils.MustNewUUID().String()
	metadata := coreobjectstore.Metadata{
		Path: path,
		Hash: utils.MustNewUUID().String(),
		Size: 666,
	}

	s.state.EXPECT().PutMetadata(gomock.Any(), gomock.AssignableToTypeOf(objectstore.Metadata{})).DoAndReturn(func(ctx context.Context, data objectstore.Metadata) error {
		c.Assert(data.Path, gc.Equals, metadata.Path)
		c.Assert(data.Size, gc.Equals, metadata.Size)
		c.Assert(data.Hash, gc.Equals, metadata.Hash)
		return nil
	})

	err := NewService(s.state, nil).PutMetadata(context.Background(), metadata)
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

	table := "objectstore"
	stmt := "SELECT key FROM objectstore"
	s.state.EXPECT().InitialWatchStatement().Return(table, stmt)
	s.watcherFactory.EXPECT().NewNamespaceWatcher(table, changestream.All, stmt).Return(watcher, nil)

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
