// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/testing"
)

const (
	defaultBucketName = "juju-123"
)

type s3ObjectStoreSuite struct {
	baseSuite

	states                 chan string
	session                *MockSession
	hashFileSystemAccessor *MockHashFileSystemAccessor
	client                 *client
}

var _ = gc.Suite(&s3ObjectStoreSuite{})

func (s *s3ObjectStoreSuite) TestGetMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, errors.NotFound).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *s3ObjectStoreSuite) TestGetMetadataFoundNoFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := "blah"

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 666,
	}, nil).Times(2)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash)).Return(nil, int64(0), "", errors.NotFoundf("not found")).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

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

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

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

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
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

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
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

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	err := store.PutAndCheckHash(context.Background(), "foo", strings.NewReader(content), 12, hexHash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(receivedContent, gc.Equals, content)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	fakeHash := fmt.Sprintf("%s0", hexHash)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
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

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexHash), gomock.Any(), base64Hash).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

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

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hexHash,
		Path: fileName,
		Size: 666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexHash)).Return(errors.NotFoundf("foo"))

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	err := store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)

	s.expectClaim(hexHash, 1)
	s.expectRelease(hexHash, 1)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: hexHash,
		Path: "foo",
		Size: 12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexHash)).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	err := store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestList(c *gc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexHash(c, content)
	fileName := "foo"
	size := int64(666)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		Hash: hexHash,
		Path: fileName,
		Size: size,
	}}, nil)
	s.session.EXPECT().ListObjects(gomock.Any(), defaultBucketName).Return([]string{hexHash}, nil)

	store := s.newS3ObjectStore(c).(*s3ObjectStore)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	metadata, files, err := store.list(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(metadata, gc.DeepEquals, []objectstore.Metadata{{
		Hash: hexHash,
		Path: fileName,
		Size: size,
	}})
	c.Check(files, gc.DeepEquals, []string{hexHash})
}

func (s *s3ObjectStoreSuite) TestDrainFilesWithNoFiles(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.expectListMetadata([]objectstore.Metadata{})

	store := s.newDrainingS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c)
	s.expectDrain(c)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestDrainFiles(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can drain files from the object store.
	// We expect that the draining tests will be covered by the drainFile
	// tests.

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	s.expectListMetadata([]objectstore.Metadata{{
		Hash: "foo",
		Path: "foo",
		Size: 12,
	}})
	s.expectHashToExistError("foo", errors.NotFound)

	store := s.newDrainingS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c)
	s.expectFileDrained(c, "foo")
	s.expectDrain(c)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestDrainFilesWithError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can drain files from the object store.
	// We expect that the draining tests will be covered by the drainFile
	// tests.
	// The drain state shouldn't be reached if there's an error.

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.expectListMetadata([]objectstore.Metadata{{
		Hash: "foo",
		Path: "foo",
		Size: 12,
	}})
	done := s.expectHashToExistError("foo", errors.Errorf("boom"))

	store := s.newDrainingS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c)
	s.expectFileDrained(c, "foo")

	// Note: the drained state is never reached because of the error.

	select {
	case <-done:
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for drain")
	}

	err := workertest.CheckKill(c, store)
	c.Assert(err, gc.ErrorMatches, `.*boom.*`)
}

func (s *s3ObjectStoreSuite) TestDrainFileDoesNotExist(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := &s3ObjectStore{
		fileSystemAccessor: s.hashFileSystemAccessor,
		allowDraining:      true,
	}

	s.expectHashToExistError("foo", errors.NotFound)

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestDrainFileObjectError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := &s3ObjectStore{
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.Errorf("boom"))

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, gc.ErrorMatches, `.*boom.*`)
}

func (s *s3ObjectStoreSuite) TestDrainFileObjectAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			logger: testing.NewCheckLogger(c),
		},
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	s.expectHashToExist("foo")
	s.expectObjectExists("foo")

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestDrainFileObjectGetHashReturnsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test what happens when the object does not exist in the object store
	// and doesn't exist in the hash file system.
	// In this case we should just return nil.

	store := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			logger: testing.NewCheckLogger(c),
		},
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHashError("foo", errors.NotFoundf("not found"))

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *s3ObjectStoreSuite) TestDrainFileSizeDoNotMatch(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test what happens when the size of the file in the object store
	// does not match the size of the file in the hash file system.
	// In this case we should just return nil, otherwise we'll end up
	// crashing the worker.

	store := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			logger: testing.NewCheckLogger(c),
		},
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(666)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reader.Closed(), jc.IsTrue)
}

func (s *s3ObjectStoreSuite) TestDrainFilePut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	store := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			logger: testing.NewCheckLogger(c),
		},
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(12)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)
	s.expectHashPut(c, "foo", "KQ9JPET11j0Gs3TQpavSkvrji5LKsvrl7+/hsOk0f1Y=", "some content")
	s.expectDeleteHash("foo")

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reader.Closed(), jc.IsTrue)
}

func (s *s3ObjectStoreSuite) TestDrainFileDeleteError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can handle an error when we try to delete the hash
	// from the file system.
	// In this case we should just return nil, otherwise we'll end up
	// crashing the worker.

	store := &s3ObjectStore{
		baseObjectStore: baseObjectStore{
			logger: testing.NewCheckLogger(c),
		},
		rootBucket:         defaultBucketName,
		namespace:          "inferi",
		fileSystemAccessor: s.hashFileSystemAccessor,
		client:             s.client,
		allowDraining:      true,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(12)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)
	s.expectHashPut(c, "foo", "KQ9JPET11j0Gs3TQpavSkvrji5LKsvrl7+/hsOk0f1Y=", "some content")
	s.expectDeleteHashError("foo", errors.Errorf("boom"))

	err := store.drainFile(context.Background(), "/path", "foo", 12)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(reader.Closed(), jc.IsTrue)
}

func (s *s3ObjectStoreSuite) TestComputeS3Hash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can compute the hash without having to perform
	// an intermediary step. This will use the Seeker interface, to rewind
	// the reader to the start of the file.

	content := "some content"
	expectedHash := s.calculateBase64Hash(c, content)

	store := &s3ObjectStore{}

	reader, hash, err := store.computeS3Hash(strings.NewReader(content))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, expectedHash)

	bytes, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(bytes), gc.Equals, content)
}

func (s *s3ObjectStoreSuite) TestComputeS3HashNoSeekerReader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can compute the hash even if we don't have a Seeker
	// interface. Hopefully this won't be the case for most paths, but
	// we require that the reader is rewound to the start of the file.

	content := "some content"
	expectedHash := s.calculateBase64Hash(c, content)

	store := &s3ObjectStore{}

	reader, hash, err := store.computeS3Hash(blockSeek{Reader: strings.NewReader(content)})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(hash, gc.Equals, expectedHash)

	bytes, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(bytes), gc.Equals, content)
}

func (s *s3ObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.states = make(chan string, 1)
	s.session = NewMockSession(ctrl)
	s.hashFileSystemAccessor = NewMockHashFileSystemAccessor(ctrl)
	s.client = &client{session: s.session}

	return ctrl
}

func (s *s3ObjectStoreSuite) expectStartup(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *s3ObjectStoreSuite) expectDrain(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateDrained)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for drained")
	}
}

func (s *s3ObjectStoreSuite) expectFileDrained(c *gc.C, hash string) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, fmt.Sprintf(stateFileDrained, hash))
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for file drained")
	}
}

func (s *s3ObjectStoreSuite) expectFailure(fileName string, err error) {
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{}, err)
}

func (s *s3ObjectStoreSuite) expectListMetadata(metadata []objectstore.Metadata) {
	s.service.EXPECT().ListMetadata(gomock.Any()).Return(metadata, nil)
}

func (s *s3ObjectStoreSuite) expectHashToExist(hash string) {
	s.hashFileSystemAccessor.EXPECT().HashExists(gomock.Any(), hash).Return(nil)
}

func (s *s3ObjectStoreSuite) expectHashToExistError(hash string, err error) <-chan struct{} {
	ch := make(chan struct{})
	s.hashFileSystemAccessor.EXPECT().HashExists(gomock.Any(), hash).DoAndReturn(func(ctx context.Context, hash string) error {
		defer close(ch)
		return err
	})
	return ch
}

func (s *s3ObjectStoreSuite) expectDeleteHash(hash string) {
	s.hashFileSystemAccessor.EXPECT().DeleteByHash(gomock.Any(), hash).Return(nil)
}

func (s *s3ObjectStoreSuite) expectDeleteHashError(hash string, err error) {
	s.hashFileSystemAccessor.EXPECT().DeleteByHash(gomock.Any(), hash).Return(err)
}

func (s *s3ObjectStoreSuite) expectObjectExists(hash string) {
	s.session.EXPECT().ObjectExists(gomock.Any(), defaultBucketName, filePath(hash)).Return(nil)
}

func (s *s3ObjectStoreSuite) expectObjectExistsError(hash string, err error) {
	s.session.EXPECT().ObjectExists(gomock.Any(), defaultBucketName, filePath(hash)).Return(err)
}

func (s *s3ObjectStoreSuite) expectGetByHash(hash string, reader io.ReadCloser, size int64) {
	s.hashFileSystemAccessor.EXPECT().GetByHash(gomock.Any(), hash).Return(reader, size, nil)
}

func (s *s3ObjectStoreSuite) expectGetByHashError(hash string, err error) {
	s.hashFileSystemAccessor.EXPECT().GetByHash(gomock.Any(), hash).Return(nil, int64(0), err)
}

func (s *s3ObjectStoreSuite) expectHashPut(c *gc.C, hash, s3Hash, content string) {
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hash), gomock.Any(), s3Hash).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		bytes, err := io.ReadAll(body)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(bytes), gc.Equals, content)
		return nil
	})
}

func (s *s3ObjectStoreSuite) newS3ObjectStore(c *gc.C) TrackedObjectStore {
	return s.newS3ObjectStoreConfig(c, false)
}

func (s *s3ObjectStoreSuite) newDrainingS3ObjectStore(c *gc.C) TrackedObjectStore {
	return s.newS3ObjectStoreConfig(c, true)
}

func (s *s3ObjectStoreSuite) newS3ObjectStoreConfig(c *gc.C, allowDraining bool) TrackedObjectStore {
	store, err := newS3ObjectStore(S3ObjectStoreConfig{
		RootBucket:             defaultBucketName,
		Namespace:              "inferi",
		RootDir:                c.MkDir(),
		Client:                 s.client,
		MetadataService:        s.service,
		Claimer:                s.claimer,
		Logger:                 testing.NewCheckLogger(c),
		Clock:                  clock.WallClock,
		HashFileSystemAccessor: s.hashFileSystemAccessor,
		AllowDraining:          allowDraining,
	}, s.states)
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

type blockSeek struct {
	io.Reader
}

type readCloser struct {
	io.Reader

	mu     sync.Mutex
	closed bool
}

func (r *readCloser) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

func (r *readCloser) Closed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}
