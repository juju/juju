// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/objectstore"
	jujutesting "github.com/juju/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type S3ObjectStoreSuite struct {
	baseSuite

	session *MockSession
	client  *client
}

var _ = gc.Suite(&S3ObjectStoreSuite{})

func (s *S3ObjectStoreSuite) TestGetMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	store, err := NewFileObjectStore(context.Background(), "inferi", path, s.service, s.claimer, jujutesting.NewCheckLogger(c), clock.WallClock)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, errors.NotFound).Times(2)

	_, _, err = store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *S3ObjectStoreSuite) TestGetMetadataFoundNoFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	store, err := NewFileObjectStore(context.Background(), "inferi", path, s.service, s.claimer, jujutesting.NewCheckLogger(c), clock.WallClock)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: "blah",
		Path: "foo",
		Size: 666,
	}, nil).Times(2)

	_, _, err = store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *S3ObjectStoreSuite) TestGetMetadataAndFileNotFoundThenFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	namespace := "inferi"
	fileName := "foo"
	hash := "blah"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	store, err := NewS3ObjectStore(context.Background(), namespace, s.client, s.service, s.claimer, jujutesting.NewCheckLogger(c), clock.WallClock)
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size,
	}, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), namespace, hash).Return(reader, size, hash, nil)

	file, fileSize, err := store.Get(context.Background(), fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(size, gc.Equals, fileSize)
	c.Assert(s.readFile(c, file), gc.Equals, "hello")
}

func (s *S3ObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.session = NewMockSession(ctrl)
	s.client = &client{session: s.session}

	return ctrl
}

type client struct {
	session objectstore.Session
}

func (c *client) Session(ctx context.Context, f func(context.Context, objectstore.Session) error) error {
	return f(ctx, c.session)
}
