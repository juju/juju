// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	watcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domainobjectstoreerrors "github.com/juju/juju/domain/objectstore/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
	"github.com/juju/juju/internal/objectstore/remote"
)

type fileObjectStoreSuite struct {
	baseSuite

	remote *MockRemoteRetriever
}

func TestFileObjectStoreSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &fileObjectStoreSuite{})
}

func (s *fileObjectStoreSuite) TestGetMetadataNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA256: "026382989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f",
	}, domainobjectstoreerrors.ErrNotFound).Times(2)
	s.remote.EXPECT().Retrieve(gomock.Any(), "026382989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f").Return(nil, -1, remote.BlobNotFound)

	store := s.newFileObjectStore(c, c.MkDir())
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHANotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), "0263829").Return(objectstore.Metadata{
		SHA256: "026382989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f",
	}, domainobjectstoreerrors.ErrNotFound).Times(2)
	s.remote.EXPECT().Retrieve(gomock.Any(), "026382989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f").Return(nil, -1, remote.BlobNotFound)

	store := s.newFileObjectStore(c, c.MkDir())
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.GetBySHA256Prefix(c.Context(), "0263829")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataFoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA384: "blah",
		SHA256: "blah",
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)

	s.remote.EXPECT().Retrieve(gomock.Any(), "blah").
		Return(nil, -1, jujuerrors.NotFoundf("not found"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256FoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f").Return(objectstore.Metadata{
		SHA384: "blah",
		SHA256: "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f",
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.GetBySHA256(c.Context(), "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256PrefixFoundNoFile(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), "0263829").Return(objectstore.Metadata{
		SHA384: "blah",
		SHA256: "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f",
		Path:   "foo",
		Size:   666,
	}, nil).Times(2)

	s.remote.EXPECT().Retrieve(gomock.Any(), "0263829989b6fd954f72baaf2fc64bc2e2f01d692d4de72986ea808f6e99813f").
		Return(nil, -1, jujuerrors.NotFoundf("not found"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.GetBySHA256Prefix(c.Context(), "0263829")
	c.Assert(err, tc.ErrorIs, objectstoreerrors.ObjectNotFound)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.Get(c.Context(), fileName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, fileSize)
	c.Check(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataFoundNoFileRemoteFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	content := bytes.NewBufferString("some content")
	size := int64(content.Len())

	hash384 := "66b3707eaed3f7f4c6f084e4ba7aaa95f0412c3d9fd91475fc454b93ed8b7cd9d33cc1821e517b52d338f8d8d6908cb9"
	hash256 := "290f493c44f5d63d06b374d0a5abd292fae38b92cab2fae5efefe1b0e9347f56"

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}, nil).Times(2)

	s.remote.EXPECT().Retrieve(gomock.Any(), hash256).
		Return(io.NopCloser(content), size, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	s.expectClaim(hash384, 1)

	s.expectRelease(hash384, 1)

	file, fileSize, err := store.Get(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, fileSize)
	c.Check(s.readFile(c, file), tc.Equals, "some content")

	// The file has been claimed and released.
	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256AndFileFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256PrefixAndFileFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")
	hashPrefix := hash256[:7]

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256PrefixFoundNoFileRemoteFallback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	ch := s.expectWatch()

	content := bytes.NewBufferString("some content")
	size := int64(content.Len())

	hash384 := "66b3707eaed3f7f4c6f084e4ba7aaa95f0412c3d9fd91475fc454b93ed8b7cd9d33cc1821e517b52d338f8d8d6908cb9"
	hash256 := "290f493c44f5d63d06b374d0a5abd292fae38b92cab2fae5efefe1b0e9347f56"
	hashPrefix := hash256[:7]

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}, nil).Times(2)

	s.remote.EXPECT().Retrieve(gomock.Any(), hash256).
		Return(io.NopCloser(content), size, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	s.expectClaim(hash384, 1)

	s.expectRelease(hash384, 1)

	file, fileSize, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, fileSize)
	c.Check(s.readFile(c, file), tc.Equals, "some content")

	// The file has been claimed and released.
	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, jujuerrors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.Get(c.Context(), fileName)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256AndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, jujuerrors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256PrefixAndFileNotFoundThenFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Attempt to read the file before it exists. This should fail.
	// Then attempt to read the file after it exists. This should succeed.

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")
	hashPrefix := hash256[:7]

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, jujuerrors.NotFoundf("not found"))
	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	file, fileSize, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(size, tc.Equals, fileSize)
	c.Assert(s.readFile(c, file), tc.Equals, "some content")

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataAndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size + 1,
	}, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.Get(c.Context(), fileName)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256AndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.service.EXPECT().GetMetadataBySHA256(gomock.Any(), hash256).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size + 1,
	}, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.GetBySHA256(c.Context(), hash256)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestGetMetadataBySHA256PrefixAndFileFoundWithIncorrectSize(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")
	hashPrefix := hash256[:7]

	s.service.EXPECT().GetMetadataBySHA256Prefix(gomock.Any(), hashPrefix).Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size + 1,
	}, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, _, err := store.GetBySHA256Prefix(c.Context(), hashPrefix)
	c.Assert(err, tc.ErrorMatches, `.*size mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPut(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 1)
	s.expectRelease(hash384, 1)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	received, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(received.Validate(), tc.ErrorIsNil)
	c.Check(received, tc.Equals, uuid)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutFileAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 2)
	s.expectRelease(hash384, 2)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	uuid0, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid0.Validate(), tc.ErrorIsNil)

	uuid1, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid1.Validate(), tc.ErrorIsNil)

	c.Check(uuid0, tc.Equals, uuid1)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutCleansUpFileOnMetadataFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to be cleaned by the object store later on.

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 1)
	s.expectRelease(hash384, 1)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, jujuerrors.Errorf("boom"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutDoesNotCleansUpFileOnMetadataFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 2)
	s.expectRelease(hash384, 2)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, jujuerrors.Errorf("boom"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)

	_, err = store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 1)
	s.expectRelease(hash384, 1)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	uuid, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid.Validate(), tc.ErrorIsNil)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")

	fakeHash := fmt.Sprintf("%s0", hash384)

	path := c.MkDir()

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, fakeHash)
	c.Assert(err, tc.ErrorMatches, `.*hash mismatch.*`)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashFileAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 2)
	s.expectRelease(hash384, 2)

	path := c.MkDir()

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return(uuid, nil).Times(2)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	uuid0, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid0.Validate(), tc.ErrorIsNil)

	uuid1, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(uuid1.Validate(), tc.ErrorIsNil)

	c.Check(uuid0, tc.Equals, uuid1)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashCleansUpFileOnMetadataFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to cleaned up by the object store later on.

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 1)
	s.expectRelease(hash384, 1)

	path := c.MkDir()

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return("", jujuerrors.Errorf("boom"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestPutAndCheckHashDoesNotCleansUpFileOnMetadataFailure(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 2)
	s.expectRelease(hash384, 2)

	path := c.MkDir()

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return("", nil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return("", jujuerrors.Errorf("boom"))

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorIsNil)

	_, err = store.PutAndCheckHash(c.Context(), "foo", strings.NewReader("some content"), 12, hash384)
	c.Assert(err, tc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestRemoveFileNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we don't return an error if the file does not exist.
	// We just want to ensure that we don't return an error after the metadata
	// is removed.

	ch := s.expectWatch()

	s.expectClaim("blah", 1)
	s.expectRelease("blah", 1)

	path := c.MkDir()

	fileName := "foo"

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		SHA384: "blah",
		SHA256: "blah",
		Path:   fileName,
		Size:   666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	err := store.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestRemove(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 2)
	s.expectRelease(hash384, 2)

	path := c.MkDir()

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return("", nil)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)

	s.expectFileDoesExist(c, path, hash384)

	err = store.Remove(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	s.expectFileDoesNotExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestRemoveAll(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	hash384 := s.calculateHexSHA384(c, "some content")
	hash256 := s.calculateHexSHA256(c, "some content")

	s.expectClaim(hash384, 1)
	s.expectRelease(hash384, 1)

	path := c.MkDir()

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		SHA384: hash384,
		SHA256: hash256,
		Path:   "foo",
		Size:   12,
	}).Return("", nil)

	store := s.newFileObjectStore(c, path)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	_, err := store.Put(c.Context(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, tc.ErrorIsNil)

	s.expectFileDoesExist(c, path, hash384)

	store.Kill()

	err = store.RemoveAll(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	s.expectFileDoesNotExist(c, path, hash384)

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) TestList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	ch := s.expectWatch()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash384, hash256 := s.createFile(c, s.filePath(path, namespace), fileName, "some content")

	s.createDirectory(c, s.filePath(path, namespace), "other")

	s.service.EXPECT().ListMetadata(gomock.Any()).Return([]objectstore.Metadata{{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}}, nil)

	store := s.newFileObjectStore(c, path).(*fileObjectStore)
	defer workertest.DirtyKill(c, store)

	s.expectStartup(c, ch)

	metadata, files, err := store.list(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(metadata, tc.DeepEquals, []objectstore.Metadata{{
		SHA384: hash384,
		SHA256: hash256,
		Path:   fileName,
		Size:   size,
	}})
	c.Check(files, tc.DeepEquals, []string{hash384})

	workertest.CleanKill(c, store)
}

func (s *fileObjectStoreSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.remote = NewMockRemoteRetriever(ctrl)

	c.Cleanup(func() {
		s.remote = nil
	})

	return ctrl
}

func (s *fileObjectStoreSuite) expectFileDoesNotExist(c *tc.C, path, hash string) {
	_, err := os.Stat(filepath.Join(path, defaultFileDirectory, "inferi", hash))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *fileObjectStoreSuite) expectFileDoesExist(c *tc.C, path, hash string) {
	_, err := os.Stat(filepath.Join(path, defaultFileDirectory, "inferi", hash))
	c.Assert(err, tc.ErrorIsNil)
}

func (s *fileObjectStoreSuite) expectClaim(hash string, num int) {
	s.claimer.EXPECT().Claim(gomock.Any(), hash).Return(s.claimExtender, nil).Times(num)
	s.claimExtender.EXPECT().Extend(gomock.Any()).Return(nil).AnyTimes()
	s.claimExtender.EXPECT().Duration().Return(time.Second).AnyTimes()
}

func (s *fileObjectStoreSuite) expectRelease(hash string, num int) {
	s.claimer.EXPECT().Release(gomock.Any(), hash).Return(nil).Times(num)
}

func (s *fileObjectStoreSuite) expectWatch() chan struct{} {
	ch := make(chan struct{})
	s.service.EXPECT().Watch().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		defer close(ch)

		return watchertest.NewMockStringsWatcher(make(<-chan []string)), nil
	})

	return ch
}

func (s *fileObjectStoreSuite) expectStartup(c *tc.C, ch chan struct{}) {
	select {
	case <-ch:
	case <-c.Context().Done():
		c.Fatalf("context cancelled before watcher was called")
	}
}

func (s *fileObjectStoreSuite) filePath(path, namespace string) string {
	return filepath.Join(path, defaultFileDirectory, namespace)
}

func (s *fileObjectStoreSuite) newFileObjectStore(c *tc.C, path string) TrackedObjectStore {
	store, err := NewFileObjectStore(FileObjectStoreConfig{
		Namespace:       "inferi",
		RootDir:         path,
		MetadataService: s.service,
		Claimer:         s.claimer,
		Logger:          loggertesting.WrapCheckLog(c),
		Clock:           clock.WallClock,
		RemoteRetriever: s.remote,
	})
	c.Assert(err, tc.IsNil)

	return store
}

func (s *fileObjectStoreSuite) createDirectory(c *tc.C, path, name string) {
	err := os.MkdirAll(filepath.Join(path, name), 0755)
	c.Assert(err, tc.ErrorIsNil)
}
