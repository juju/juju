// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/testing"
)

const (
	defaultBucketName = "juju-123"
)

type s3ObjectStoreSuite struct {
	baseSuite

	states  chan string
	session *MockSession
	client  *client
}

func TestS3ObjectStoreSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &s3ObjectStoreSuite{})
}

func (s *s3ObjectStoreSuite) TestGetMetadataNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, domainobjectstoreerrors.ErrNotFound).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHANotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), "0263829").Return(objectstore.Metadata{}, domainobjectstoreerrors.ErrNotFound).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.GetBySHA256Prefix(c.Context(), "0263829")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataFoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash256 := "blah-256"
	hash384 := "blah-512-384"

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(nil, int64(0), "", errors.NotFoundf("not found")).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256FoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hash384 := "blah-512-384"

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(nil, int64(0), "", errors.NotFoundf("not found")).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256PrefixFoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hashPrefix := hash256[:7]
	hash384 := "blah-512-384"

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(nil, int64(0), "", errors.NotFoundf("not found")).Times(2)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataAndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	fileName := "foo"
	hash256 := "blah-256"
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	s.expectFailure(fileName, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	file, fileSize, err := store.Get(c.Context(), fileName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "hello")

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256AndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	fileName := "foo"
	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadataBySHA256 twice. We optimistically call
	// it once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	s.expectBySHA256Failure(hash256, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	file, fileSize, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "hello")

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256PrefixAndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	fileName := "foo"
	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hashPrefix := hash256[:7]
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadataBySHA256Prefix twice. We optimistically
	// call it once, and if it fails, we call it again. We're exercising the
	// code serialization code here.

	s.expectBySHA256PrefixFailure(hashPrefix, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	file, fileSize, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "hello")

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataAndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fileName := "foo"
	hash256 := "blah-256"
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	// This size will be incorrect here, which will cause an error.

	s.expectFailure(fileName, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size + 1,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.Get(c.Context(), fileName)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256AndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fileName := "foo"
	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	// This size will be incorrect here, which will cause an error.

	s.expectBySHA256Failure(hash256, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size + 1,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestGetMetadataBySHA256PrefixAndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	fileName := "foo"
	hash256 := "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f"
	hashPrefix := hash256[:7]
	hash384 := "blah-512-384"
	size := int64(666)
	reader := io.NopCloser(bytes.NewBufferString("hello"))

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	// We expect that we call GetMetadata twice. We optimistically call it
	// once, and if it fails, we call it again. We're exercising the code
	// serialization code here.

	// This size will be incorrect here, which will cause an error.

	s.expectBySHA256PrefixFailure(hashPrefix, errors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA256: hash256,
		SHA384: hash384,
		Path:   fileName,
		Size:   size + 1,
	}, nil)
	s.session.EXPECT().GetObject(gomock.Any(), defaultBucketName, filePath(hash384)).Return(reader, size, hash384, nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, _, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPut(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)
	base64SHA256 := s.calculateBase64SHA256(c, content)
	s.expectClaim(hexSHA384, 1)
	s.expectRelease(hexSHA384, 1)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexSHA384), gomock.Any(), base64SHA256).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	received, err := store.Put(c.Context(), "foo", strings.NewReader(content), 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid.Validate(), tc.ErrorIsNil)
	c.Check(received, tc.Equals, uuid)

	c.Check(receivedContent, tc.Equals, content)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)
	base64SHA256 := s.calculateBase64SHA256(c, content)
	s.expectClaim(hexSHA384, 1)
	s.expectRelease(hexSHA384, 1)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexSHA384), gomock.Any(), base64SHA256).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	uuid, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader(content), 12, hexSHA384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid.Validate(), tc.ErrorIsNil)

	c.Check(receivedContent, tc.Equals, content)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	fakeHash := fmt.Sprintf("%s0", hexSHA384)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader(content), 12, fakeHash)
	c.Assert(err, tc.ErrorMatches, `.*hash mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPutAndCheckHashFileAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)
	base64SHA256 := s.calculateBase64SHA256(c, content)
	s.expectClaim(hexSHA384, 2)
	s.expectRelease(hexSHA384, 2)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil).Times(2)

	var receivedContent string
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexSHA384), gomock.Any(), base64SHA256).DoAndReturn(func(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
		receivedContent = s.readFile(c, io.NopCloser(body))
		return nil
	})
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexSHA384), gomock.Any(), base64SHA256).Return(errors.AlreadyExistsf("already exists"))

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	uuid0, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader(content), 12, hexSHA384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid0.Validate(), tc.ErrorIsNil)

	uuid1, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader(content), 12, hexSHA384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid1.Validate(), tc.ErrorIsNil)

	c.Check(receivedContent, tc.Equals, content)

	c.Check(uuid0, tc.Equals, uuid1)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPutFileOnMetadataFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to cleaned up by the object store later on.

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)
	base64SHA256 := s.calculateBase64SHA256(c, content)
	s.expectClaim(hexSHA384, 1)
	s.expectRelease(hexSHA384, 1)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, errors.Errorf("boom"))
	s.session.EXPECT().PutObject(gomock.Any(), defaultBucketName, filePath(hexSHA384), gomock.Any(), base64SHA256).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	_, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader(content), 12, hexSHA384)
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestRemoveFileNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we don't return an error if the file does not exist.
	// We just want to ensure that we don't return an error after the metadata
	// is removed.

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)

	s.expectClaim(hexSHA384, 1)
	s.expectRelease(hexSHA384, 1)

	fileName := "foo"

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   fileName,
		Size:   666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexSHA384)).Return(errors.NotFoundf("foo"))

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	err := store.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)

	s.expectClaim(hexSHA384, 1)
	s.expectRelease(hexSHA384, 1)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   "foo",
		Size:   12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)
	s.session.EXPECT().DeleteObject(gomock.Any(), defaultBucketName, filePath(hexSHA384)).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	err := store.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexSHA384 := s.calculateHexSHA384(c, content)
	hexSHA256 := s.calculateHexSHA256(c, content)

	fileName := "foo"
	size := int64(666)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)
	s.service.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   fileName,
		Size:   size,
	}}, nil)
	s.session.EXPECT().ListObjects(gomock.Any(), defaultBucketName).Return([]string{hexSHA384}, nil)

	store := s.newS3ObjectStore(c).(*s3ObjectStore)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	metadata, files, err := store.list(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadata, tc.DeepEquals, []objectstore.Metadata{{
		SHA384: hexSHA384,
		SHA256: hexSHA256,
		Path:   fileName,
		Size:   size,
	}})
	c.Check(files, tc.DeepEquals, []string{hexSHA384})

	workertest.CleanKill(c, store)
}

func (s *s3ObjectStoreSuite) TestPersistTmpFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	content := "some content"
	hexHash := s.calculateHexSHA384(c, content)
	base64Hash := s.calculateBase64SHA256(c, content)

	s.session.EXPECT().CreateBucket(gomock.Any(), defaultBucketName).Return(nil)

	store := s.newS3ObjectStore(c)
	defer workertest.DirtyKill(c, store)

	// Ensure we've started up before we start the test.
	s.expectStartup(c)

	dir := c.MkDir()
	filePath := filepath.Join(dir, "foo.txt")
	err := os.WriteFile(filePath, []byte(content), 0644)
	c.Assert(err, tc.ErrorIsNil)

	err = store.(*s3ObjectStore).persistTmpFile(c.Context(), filePath, hexHash, base64Hash, 42)
	c.Assert(err, tc.ErrorMatches, `size mismatch for ".*foo.txt".*`)
}

func (s *s3ObjectStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.states = make(chan string, 3)

	s.session = NewMockSession(ctrl)
	s.client = &client{session: s.session}

	c.Cleanup(func() {
		s.states = nil

		s.session = nil
		s.client = nil
	})

	return ctrl
}

func (s *s3ObjectStoreSuite) expectStartup(c *tc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, tc.Equals, stateStarted)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *s3ObjectStoreSuite) expectFailure(fileName string, err error) {
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{}, err)
}

func (s *s3ObjectStoreSuite) expectBySHA256Failure(sha string, err error) {
	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), sha).Return(objectstore.Metadata{}, err)
}

func (s *s3ObjectStoreSuite) expectBySHA256PrefixFailure(sha string, err error) {
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), sha).Return(objectstore.Metadata{}, err)
}

func (s *s3ObjectStoreSuite) newS3ObjectStore(c *tc.C) TrackedObjectStore {
	store, err := newS3ObjectStore(S3ObjectStoreConfig{
		RootBucket:      defaultBucketName,
		Namespace:       "inferi",
		RootDir:         c.MkDir(),
		Client:          s.client,
		MetadataService: s.service,
		Claimer:         s.claimer,
		Logger:          loggertesting.WrapCheckLog(c),
		Clock:           clock.WallClock,
	}, s.states)
	c.Assert(err, tc.IsNil)
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
