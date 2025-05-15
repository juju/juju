// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorefacade

import (
	"context"
	"io"
	"strings"

	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/fortress"
)

type workerSuite struct {
	baseSuite
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) TestObjectStoreGetObjectStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), "foo").Return(s.objectStore, nil)

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	store, err := w.GetObjectStore(context.Background(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(store, tc.NotNil)
}

func (s *workerSuite) TestObjectStoreGetObjectStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any(), "foo").Return(s.objectStore, errors.Errorf("boom"))

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	_, err := w.GetObjectStore(context.Background(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestGetWithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestGetWithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	_, _, err := f.Get(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestGetWithVisitReturnsDrainingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fortress.ErrAborted
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, coreobjectstore.ErrTimeoutWaitingForDraining)
}

func (s *workerSuite) TestGetWithVisitObjectStoreNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(nil, 0, jujuerrors.NotFound)

	f := s.newObjectStoreFacade()
	_, _, err := f.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, jujuerrors.NotFound)
}

func (s *workerSuite) TestGetWithVisitObjectStoreReturnsReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(io.NopCloser(strings.NewReader("foo")), 3, nil)

	f := s.newObjectStoreFacade()
	reader, size, err := f.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, []byte("foo"))
	c.Check(size, tc.Equals, int64(3))
}

func (s *workerSuite) TestGetBySHA256WithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestGetBySHA256WithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestGetBySHA256WithVisitReturnsDrainingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fortress.ErrAborted
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, coreobjectstore.ErrTimeoutWaitingForDraining)
}

func (s *workerSuite) TestGetBySHA256WithVisitObjectStoreNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), "foo").Return(nil, 0, jujuerrors.NotFound)

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, jujuerrors.NotFound)
}

func (s *workerSuite) TestGetBySHA256WithVisitObjectStoreReturnsReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().GetBySHA256(gomock.Any(), "foo").Return(io.NopCloser(strings.NewReader("foo")), 3, nil)

	f := s.newObjectStoreFacade()
	reader, size, err := f.GetBySHA256(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, []byte("foo"))
	c.Check(size, tc.Equals, int64(3))
}

func (s *workerSuite) TestGetBySHA256PrefixWithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256Prefix(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestGetBySHA256PrefixWithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256Prefix(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestGetBySHA256PrefixWithVisitReturnsDrainingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fortress.ErrAborted
	})

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256Prefix(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, coreobjectstore.ErrTimeoutWaitingForDraining)
}

func (s *workerSuite) TestGetBySHA256PrefixWithVisitObjectStoreNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "foo").Return(nil, 0, jujuerrors.NotFound)

	f := s.newObjectStoreFacade()
	_, _, err := f.GetBySHA256Prefix(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, jujuerrors.NotFound)
}

func (s *workerSuite) TestGetBySHA256PrefixWithVisitObjectStoreReturnsReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "foo").Return(io.NopCloser(strings.NewReader("foo")), 3, nil)

	f := s.newObjectStoreFacade()
	reader, size, err := f.GetBySHA256Prefix(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	data, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(data, tc.DeepEquals, []byte("foo"))
	c.Check(size, tc.Equals, int64(3))
}

func (s *workerSuite) TestPutWithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	_, err := f.Put(c.Context(), "foo", strings.NewReader("foo"), 3)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestPutWithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	_, err := f.Put(ctx, "foo", strings.NewReader("foo"), 3)
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestPutWithVisitReturnsObjectStoreUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().Put(gomock.Any(), "foo", gomock.Any(), int64(3)).Return(coreobjectstore.UUID("i-am-a-uuid"), nil)

	f := s.newObjectStoreFacade()
	uuid, err := f.Put(c.Context(), "foo", strings.NewReader("foo"), 3)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, coreobjectstore.UUID("i-am-a-uuid"))
}

func (s *workerSuite) TestPutWithVisitReturnsObjectStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().Put(gomock.Any(), "foo", gomock.Any(), int64(3)).Return(coreobjectstore.UUID("i-am-a-uuid"), errors.Errorf("boom"))

	f := s.newObjectStoreFacade()
	_, err := f.Put(c.Context(), "foo", strings.NewReader("foo"), 3)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestPutAndCheckHashWithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	_, err := f.PutAndCheckHash(c.Context(), "foo", strings.NewReader("foo"), 3, "sha384")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestPutAndCheckHashWithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	_, err := f.PutAndCheckHash(ctx, "foo", strings.NewReader("foo"), 3, "sha384")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestPutAndCheckHashWithVisitReturnsObjectStoreUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), "foo", gomock.Any(), int64(3), "sha384").Return(coreobjectstore.UUID("i-am-a-uuid"), nil)

	f := s.newObjectStoreFacade()
	uuid, err := f.PutAndCheckHash(c.Context(), "foo", strings.NewReader("foo"), 3, "sha384")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid, tc.Equals, coreobjectstore.UUID("i-am-a-uuid"))
}

func (s *workerSuite) TestPutAndCheckHashWithVisitReturnsObjectStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), "foo", gomock.Any(), int64(3), "sha384").Return(coreobjectstore.UUID("i-am-a-uuid"), errors.Errorf("boom"))

	f := s.newObjectStoreFacade()
	_, err := f.PutAndCheckHash(c.Context(), "foo", strings.NewReader("foo"), 3, "sha384")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestRemoveWithVisitReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return errors.Errorf("boom")
	})

	f := s.newObjectStoreFacade()
	err := f.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) TestRemoveWithContextCancelled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(c.Context())
	cancel()

	f := s.newObjectStoreFacade()
	err := f.Remove(ctx, "foo")
	c.Assert(err, tc.ErrorIs, context.Canceled)
}

func (s *workerSuite) TestRemoveWithVisitReturnsObjectStoreError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.guest.EXPECT().Visit(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn fortress.Visit) error {
		return fn()
	})
	s.objectStore.EXPECT().Remove(gomock.Any(), "foo").Return(errors.Errorf("boom"))

	f := s.newObjectStoreFacade()
	err := f.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *workerSuite) newWorker(c *tc.C) *Worker {
	w, err := NewWorker(Config{
		FortressVistor:    s.guest,
		ObjectStoreGetter: s.objectStoreGetter,
		Logger:            loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)
	return w.(*Worker)
}

func (s *workerSuite) newObjectStoreFacade() *objectStoreFacade {
	return &objectStoreFacade{
		FortressVistor: s.guest,
		ObjectStore:    s.objectStore,
	}
}
