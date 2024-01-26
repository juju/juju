// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	jujutesting "github.com/juju/juju/testing"
)

const (
	defaultBucketName = "juju-123"
)

type s3ObjectStoreSuite struct {
	baseSuite

	session *MockSession
	client  *client
}

var _ = gc.Suite(&s3ObjectStoreSuite{})

func (s *s3ObjectStoreSuite) TestGetMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, errors.NotFound).Times(2)

	_, _, err := store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *s3ObjectStoreSuite) TestGetMetadataFoundNoFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := "blah"

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 666,
	}, nil).Times(2)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash)).Return(nil, int64(0), "", errors.NotFoundf("not found")).Times(2)

	_, _, err := store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *s3ObjectStoreSuite) TestGetMetadataAndFileNotFoundThenFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	fileName := "foo"
	hash := "blah"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	s.expectFailure(fileName, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash)).Return(reader, size, hash, nil)

	file, fileSize, err := store.Get(context.Background(), fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(size, gc.Equals, fileSize)
	c.Assert(s.readFile(c, file), gc.Equals, "hello")
}

func (s *s3ObjectStoreSuite) TestGetMetadataAndFileFoundWithIncorrectSize(c *gc.C) {
	defer s.setupMocks(c).Finish()

	fileName := "foo"
	hash := "blah"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	// This size will be incorrect here, which will cause an error.

	s.expectFailure(fileName, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size + 1,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash)).Return(reader, size, hash, nil)

	_, _, err := store.Get(context.Background(), fileName)
	c.Assert(err, gc.ErrorMatches, `.*size mismatch.*`)
}

func (s *s3ObjectStoreSuite) TestPut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	base64Hash := s.calculateBase64Hash(c, content)
	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})

	err := store.Put(context.Background(), "foo", strings.NewReader(content), 12)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(receivedContent, gc.Equals, content)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	base64Hash := s.calculateBase64Hash(c, content)
	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})

	err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, hexHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(receivedContent, gc.Equals, content)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	fakeHash := fmt.Sprintf("%s0", hexHash)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, fakeHash)
	c.Assert(err, gc.ErrorMatches, `.*hash mismatch.*`)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHashFileAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	base64Hash := s.calculateBase64Hash(c, content)
	s.expectClaim(hexHash, 2)
	s.expectRelease(hexHash, 2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}).Return(nil).Times(2)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).Return(errors.AlreadyExistsf("already exists"))

	err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, hexHash)
	c.Assert(err, jc.ErrorIsNil)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, hexHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(receivedContent, gc.Equals, content)
}

func (s *s3ObjectStoreSuite) TestPutFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to cleaned up by the object store later on.

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	base64Hash := s.calculateBase64Hash(c, content)
	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).Return(nil)

	err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, hexHash)
	c.Assert(err, gc.ErrorMatches, `.*boom`)
}

func (s *s3ObjectStoreSuite) TestRemoveFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we don't return an error if the file does not exist.
	// We just want to ensure that we don't return an error after the metadata
	// is removed.

	content := "some content"
	hexHash := s.calculateHexHash(c, content)

	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	fileName := "foo"

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hexHash,
		Path: fileName,
		Size: 666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexHash)).Return(errors.NotFoundf("foo"))

	err := store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)

	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexHash)).Return(nil)

	err := store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestList(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	fileName := "foo"
	size := int64(666)

	store := s.newS3ObjectStore(c).(*s3ObjectStore)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	started := s.expectStartup()

	select {
	case <-started:
	case <-time.After(jujutesting.LongWait):
		c.Fatalf("timed out waiting for startup")
	}

	s.service.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		Hash: hexHash,
		Path: fileName,
		Size: size,
	}}, nil)
	s.session.EXPECT().ListObjects(gomock.Any(), defaultBucketName).Return([]string{hexHash}, nil)

	metadata, files, err := store.list(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, []objectstore.Metadata{{
		Hash: hexHash,
		Path: fileName,
		Size: size,
	}})
	c.Check(files, gc.DeepEquals, []string{hexHash})
}

func (s *s3ObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.session = NewMockSession(ctrl)
	s.client = &client{session: s.session}

	return ctrl
}

func (s *s3ObjectStoreSuite) expectStartup() chan struct{} {
	done := make(chan struct{})
	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).DoAndReturn(func(ctx context.Context, bucketName string) error {
		defer close(done)
		return nil
	})
	return done
}

func (s *s3ObjectStoreSuite) expectFailure(fileName string, err error) {
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{}, err)
}

func (s *s3ObjectStoreSuite) newS3ObjectStore(c *gc.C) TrackedObjectStore {
	store, err := NewS3ObjectStore(context.Background(), S3ObjectStoreConfig{
		RootBucket:      defaultBucketName,
		Namespace:       "inferi",
		RootDir:         c.MkDir(),
		Client:          s.client,
		MetadataService: s.service,
		Claimer:         s.claimer,
		Logger:          jujutesting.NewCheckLogger(c),
		Clock:           clock.WallClock,
	})
	c.Assert(err, gc.IsNil)
	return store
}

type client struct {
	session objectstore.Session
}

func (c *client) Session(ctx context.Context, f func(context.Context, objectstore.Session) error) error {
	return f(ctx, c.session)
}

func filePath(hash string) string {
	return fmt.Sprintf("inferi/%s", hash)
}
