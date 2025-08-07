// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	stdtesting "testing"
	time "time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	objectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/testing"
)

const (
	defaultBucketName = "juju-123"
)

type drainerSuite struct {
	baseSuite
}

func TestDrainer(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &drainerSuite{})
}

func (s *drainerSuite) TestDrainFilesWithNoFiles(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.s3Session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.expectListMetadata([]objectstore.Metadata{})

	done, store := s.newDrainerWorker(c)
	defer workertest.DirtyKill(c, store)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for drain")
	}

	workertest.CleanKill(c, store)
}

func (s *drainerSuite) TestDrainFiles(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can drain files from the object store.
	// We expect that the draining tests will be covered by the drainFile
	// tests.

	s.s3Session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	s.expectListMetadata([]objectstore.Metadata{{
		SHA384: "foo",
		SHA256: "foo",
		Path:   "foo",
		Size:   12,
	}})
	s.expectHashToExistError("foo", errors.NotFound)

	done, store := s.newDrainerWorker(c)
	defer workertest.DirtyKill(c, store)

	select {
	case <-done:
	case <-c.Context().Done():
		c.Fatalf("timed out waiting for drain")
	}

	workertest.CleanKill(c, store)
}

func (s *drainerSuite) TestDrainFilesWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can drain files from the object store.
	// We expect that the draining tests will be covered by the drainFile
	// tests.
	// The drain state shouldn't be reached if there's an error.

	s.s3Session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.expectListMetadata([]objectstore.Metadata{{
		SHA384: "foo",
		SHA256: "foo",
		Path:   "foo",
		Size:   12,
	}})
	done := s.expectHashToExistError("foo", errors.Errorf("boom"))

	_, store := s.newDrainerWorker(c)
	defer workertest.DirtyKill(c, store)

	// Note: the drained state is never reached because of the error.

	select {
	case <-done:
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for drain")
	}

	err := workertest.CheckKill(c, store)
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)
}

func (s *drainerSuite) TestDrainFileDoesNotExist(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := &drainWorker{
		fileSystem: s.hashFileSystemAccessor,
	}

	s.expectHashToExistError("foo", errors.NotFound)

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainerSuite) TestDrainFileObjectError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.Errorf("boom"))

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorMatches, `.*boom.*`)
}

func (s *drainerSuite) TestDrainFileObjectAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	s.expectHashToExist("foo")
	s.expectObjectExists("foo")

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainerSuite) TestDrainFileObjectGetHashReturnsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test what happens when the object does not exist in the object store
	// and doesn't exist in the hash file system.
	// In this case we should just return nil.

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHashError("foo", errors.NotFoundf("not found"))

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *drainerSuite) TestDrainFileSizeDoNotMatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test what happens when the size of the file in the object store
	// does not match the size of the file in the hash file system.
	// In this case we should just return nil, otherwise we'll end up
	// crashing the worker.

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(666)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader.Closed(), tc.IsTrue)
}

func (s *drainerSuite) TestDrainFilePut(c *tc.C) {
	defer s.setupMocks(c).Finish()

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(12)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)
	s.expectHashPut(c, "foo", "KQ9JPET11j0Gs3TQpavSkvrji5LKsvrl7+/hsOk0f1Y=", "some content")
	s.expectDeleteHash("foo")

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader.Closed(), tc.IsTrue)
}

func (s *drainerSuite) TestDrainFileDeleteError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can handle an error when we try to delete the hash
	// from the file system.
	// In this case we should just return nil, otherwise we'll end up
	// crashing the worker.

	store := &drainWorker{
		rootBucket: defaultBucketName,
		namespace:  "inferi",
		fileSystem: s.hashFileSystemAccessor,
		client:     &client{s3Session: s.s3Session},
		logger:     s.logger,
	}

	reader := &readCloser{Reader: strings.NewReader("some content")}
	size := int64(12)

	s.expectHashToExist("foo")
	s.expectObjectExistsError("foo", errors.NotFoundf("not found"))
	s.expectGetByHash("foo", reader, size)
	s.expectHashPut(c, "foo", "KQ9JPET11j0Gs3TQpavSkvrji5LKsvrl7+/hsOk0f1Y=", "some content")
	s.expectDeleteHashError("foo", errors.Errorf("boom"))

	err := store.drainFile(c.Context(), "/path", "foo", 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(reader.Closed(), tc.IsTrue)
}

func (s *drainerSuite) TestComputeS3Hash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can compute the hash without having to perform
	// an intermediary step. This will use the Seeker interface, to rewind
	// the reader to the start of the file.

	content := "some content"
	expectedHash := s.calculateBase64SHA256(c, content)

	store := &drainWorker{}

	reader, hash, err := store.computeS3Hash(strings.NewReader(content))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, expectedHash)

	bytes, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(bytes), tc.Equals, content)
}

func (s *drainerSuite) TestComputeS3HashNoSeekerReader(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we can compute the hash even if we don't have a Seeker
	// interface. Hopefully this won't be the case for most paths, but
	// we require that the reader is rewound to the start of the file.

	content := "some content"
	expectedHash := s.calculateBase64SHA256(c, content)

	store := &drainWorker{}

	reader, hash, err := store.computeS3Hash(blockSeek{Reader: strings.NewReader(content)})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(hash, tc.Equals, expectedHash)

	bytes, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(bytes), tc.Equals, content)
}

func (s *drainerSuite) newDrainerWorker(c *tc.C) (<-chan string, worker.Worker) {
	ch := make(chan string, 1)
	store := NewDrainWorker(
		ch,
		s.hashFileSystemAccessor,
		&client{s3Session: s.s3Session},
		s.objectStoreMetadata,
		defaultBucketName,
		"inferi",
		func(m objectstore.Metadata) string {
			return m.SHA384
		},
		s.logger,
	)
	return ch, store
}

func (s *drainerSuite) expectListMetadata(metadata []objectstore.Metadata) {
	s.objectStoreMetadata.EXPECT().ListMetadata(gomock.Any()).Return(metadata, nil)
}

func (s *drainerSuite) expectHashToExist(hash string) {
	s.hashFileSystemAccessor.EXPECT().HashExists(gomock.Any(), hash).Return(nil)
}

func (s *drainerSuite) expectHashToExistError(hash string, err error) <-chan struct{} {
	ch := make(chan struct{})
	s.hashFileSystemAccessor.EXPECT().HashExists(gomock.Any(), hash).DoAndReturn(func(ctx context.Context, hash string) error {
		close(ch)
		return err
	})
	return ch
}

func (s *drainerSuite) expectDeleteHash(hash string) {
	s.hashFileSystemAccessor.EXPECT().DeleteByHash(gomock.Any(), hash).Return(nil)
}

func (s *drainerSuite) expectDeleteHashError(hash string, err error) {
	s.hashFileSystemAccessor.EXPECT().DeleteByHash(gomock.Any(), hash).Return(err)
}

func (s *drainerSuite) expectObjectExists(hash string) {
	s.s3Session.EXPECT().ObjectExists(gomock.Any(), defaultBucketName, filePath(hash)).Return(nil)
}

func (s *drainerSuite) expectObjectExistsError(hash string, err error) {
	s.s3Session.EXPECT().ObjectExists(gomock.Any(), defaultBucketName, filePath(hash)).Return(err)
}

func (s *drainerSuite) expectGetByHash(hash string, reader io.ReadCloser, size int64) {
	s.hashFileSystemAccessor.EXPECT().GetByHash(gomock.Any(), hash).Return(reader, size, nil)
}

func (s *drainerSuite) expectGetByHashError(hash string, err error) {
	s.hashFileSystemAccessor.EXPECT().GetByHash(gomock.Any(), hash).Return(nil, int64(0), err)
}

func (s *drainerSuite) expectHashPut(c *tc.C, hash, s3Hash, content string) {
	s.s3Session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hash), gomock.Any(), s3Hash).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		bytes, err := io.ReadAll(body)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(string(bytes), tc.Equals, content)
		return nil
	})
}

func (s *drainerSuite) calculateBase64SHA256(c *tc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, tc.ErrorIsNil)
	return base64.StdEncoding.EncodeToString(hasher.Sum(nil))
}

func filePath(hash string) string {
	return fmt.Sprintf("inferi/%s", hash)
}

type client struct {
	s3Session objectstore.Session
}

func (c *client) Session(ctx context.Context, f func(context.Context, objectstore.Session) error) error {
	return f(ctx, c.s3Session)
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
