// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"io"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/testhelpers"
)

type remoteFileObjectStoreSuite struct {
	testhelpers.IsolationSuite

	tracked *MockTrackedObjectStore
	remote  ReportableWorker

	reader io.ReadCloser
}

func TestRemoteFileObjectStoreSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &remoteFileObjectStoreSuite{})
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreDies(c *tc.C) {
	defer s.setupMocks(c).Finish()

	remoteStore := s.newRemoteFileObjectStore(c)

	workertest.CheckAlive(c, remoteStore)
	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGet(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().Get(gomock.Any(), "foo").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reader, tc.Equals, s.reader)
	c.Check(size, tc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGetBySHA256(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().GetBySHA256(gomock.Any(), "09ca7e4eaa6e8ae9c7d261167129184883644d07dfba7cbfbc4c8a2e08360d5b").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.GetBySHA256(c.Context(), "09ca7e4eaa6e8ae9c7d261167129184883644d07dfba7cbfbc4c8a2e08360d5b")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reader, tc.Equals, s.reader)
	c.Check(size, tc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStoreGetBySHA256Prefix(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().GetBySHA256Prefix(gomock.Any(), "09ca7e4").Return(s.reader, 12, nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	reader, size, err := remoteStore.GetBySHA256Prefix(c.Context(), "09ca7e4")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(reader, tc.Equals, s.reader)
	c.Check(size, tc.Equals, int64(12))

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "hello, world")

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStorePut(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().Put(gomock.Any(), "foo", s.reader, int64(12)).Return("abc", nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	uuid, err := remoteStore.Put(c.Context(), "foo", s.reader, 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, objectstore.UUID("abc"))

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) TestNewRemoteFileObjectStorePutAndCheckHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.tracked.EXPECT().PutAndCheckHash(gomock.Any(), "foo", s.reader, int64(12), "xyz").Return("abc", nil)

	remoteStore := s.newRemoteFileObjectStore(c)
	defer workertest.DirtyKill(c, remoteStore)

	workertest.CheckAlive(c, remoteStore)

	uuid, err := remoteStore.PutAndCheckHash(c.Context(), "foo", s.reader, 12, "xyz")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, objectstore.UUID("abc"))

	workertest.CheckKill(c, remoteStore)
}

func (s *remoteFileObjectStoreSuite) newRemoteFileObjectStore(c *tc.C) *remoteFileObjectStore {
	remoteStore, err := newRemoteFileObjectStore(newTrackedWorker(s.tracked), s.remote)
	c.Assert(err, tc.ErrorIsNil)

	return remoteStore
}

func (s *remoteFileObjectStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.tracked = NewMockTrackedObjectStore(ctrl)
	s.remote = newReportableWorker(workertest.NewErrorWorker(nil))

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

type reportableWorker struct {
	worker worker.Worker
}

func newReportableWorker(w worker.Worker) *reportableWorker {
	return &reportableWorker{
		worker: w,
	}
}

func (w *reportableWorker) Kill() {
	w.worker.Kill()
}

func (w *reportableWorker) Wait() error {
	return w.worker.Wait()
}

func (w *reportableWorker) Report() map[string]any {
	return make(map[string]any)
}
