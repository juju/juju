// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
)

type remoteFileObjectStoreSuite struct {
	testing.IsolationSuite

	tracked *MockTrackedObjectStore
	remote  worker.Worker

	reader io.ReadCloser
}

var _ = gc.Suite(&remoteFileObjectStoreSuite{})

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreDies(c *gc.C) {
	defer s.setupMocks(c).Finish()

	remoteStore := s.newRemoteFileObjectStore(c)

	workertest.CheckAlive(c, remoteStore)
	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().Get(gomock.Any(), "foo").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reader, gc.Equals, s.reader)
	c.Check(size, gc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGetBySHA256(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().GetBySHA256(gomock.Any(), "09ca7e4eaa6e8ae9c7d261167129184883644d07dfba7cbfbc4c8a2e08360d5b").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.GetBySHA256(context.Background(), "09ca7e4eaa6e8ae9c7d261167129184883644d07dfba7cbfbc4c8a2e08360d5b")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reader, gc.Equals, s.reader)
	c.Check(size, gc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGetBySHA256Prefix(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().GetBySHA256Prefix(gomock.Any(), "09ca7e4").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.GetBySHA256Prefix(context.Background(), "09ca7e4")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reader, gc.Equals, s.reader)
	c.Check(size, gc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(data), gc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStorePut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().Put(gomock.Any(), "foo", s.reader, int64(12)).Return("abc", nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	uuid, err := remoteStore.Put(context.Background(), "foo", s.reader, 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, objectstore.UUID("abc"))

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStorePutAndCheckHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().PutAndCheckHash(gomock.Any(), "foo", s.reader, int64(12), "xyz").Return("abc", nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	uuid, err := remoteStore.PutAndCheckHash(context.Background(), "foo", s.reader, 12, "xyz")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(uuid, gc.Equals, objectstore.UUID("abc"))

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) newRemoteFileObjectStore(c *gc.C) *remoteFileObjectStore {
	remoteStore, err := newRemoteFileObjectStore(newTrackedWorker(s.tracked), s.remote)
	c.Assert(err, jc.ErrorIsNil)

	return remoteStore
}

func (s *remoteFileObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.tracked = NewMockTrackedObjectStore(ctrl)
	s.remote = workertest.NewErrorWorker(nil)

	s.reader = io.NopCloser(bytes.NewBufferString("hello, world"))

	return ctrl
}

type trackedWorker struct {
	tomb tomb.Tomb

	*MockTrackedObjectStore
}

func newTrackedWorker(store *MockTrackedObjectStore) *trackedWorker {
	w := &trackedWorker{
		MockTrackedObjectStore: store,
	}

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})

	return w
}

func (w *trackedWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *trackedWorker) Wait() error {
	return w.tomb.Wait()
}
