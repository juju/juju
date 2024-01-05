// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	jujutesting "github.com/juju/juju/testing"
)

type FileObjectStoreSuite struct {
	testing.IsolationSuite

	service *MockObjectStoreMetadata
	locker  *MockLocker
}

var _ = gc.Suite(&FileObjectStoreSuite{})

func (s *FileObjectStoreSuite) TestGetMetadataNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	store, err := NewFileObjectStore(context.Background(), "inferi", path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{}, errors.NotFound)

	_, _, err = store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *FileObjectStoreSuite) TestGetMetadataFoundNoFile(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	store, err := NewFileObjectStore(context.Background(), "inferi", path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: "blah",
		Path: "foo",
		Size: 666,
	}, nil)

	_, _, err = store.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, os.ErrNotExist)
}

func (s *FileObjectStoreSuite) TestGetMetadataAndFileFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash := s.createFile(c, filepath.Join(path, namespace), fileName, "some content")

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size,
	}, nil)

	file, fileSize, err := store.Get(context.Background(), fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(size, gc.Equals, fileSize)
	c.Assert(s.readFile(c, file), gc.Equals, "some content")
}

func (s *FileObjectStoreSuite) TestGetMetadataAndFileFoundWithIncorrectSize(c *gc.C) {
	defer s.setupMocks(c).Finish()

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"
	size, hash := s.createFile(c, filepath.Join(path, namespace), fileName, "some content")

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: hash,
		Path: fileName,
		Size: size + 1,
	}, nil)

	_, _, err = store.Get(context.Background(), fileName)
	c.Assert(err, gc.ErrorMatches, `.*size mismatch.*`)
}

func (s *FileObjectStoreSuite) TestPut(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 1)
	s.expectUnlock(hash, 1)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) TestPutFileAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 2)
	s.expectUnlock(hash, 2)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil).Times(2)

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) TestPutCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to be cleaned by the object store later on.

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 1)
	s.expectUnlock(hash, 1)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, namespace, hash)
}

func (s *FileObjectStoreSuite) TestPutDoesNotCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 2)
	s.expectUnlock(hash, 2)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, namespace, hash)
}

func (s *FileObjectStoreSuite) TestPutAndCheckHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 1)
	s.expectUnlock(hash, 1)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) TestPutAndCheckHashWithInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	fakeHash := fmt.Sprintf("%s0", hash)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, fakeHash)
	c.Assert(err, gc.ErrorMatches, `.*hash mismatch.*`)
}

func (s *FileObjectStoreSuite) TestPutAndCheckHashFileAlreadyExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 2)
	s.expectUnlock(hash, 2)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil).Times(2)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, jc.ErrorIsNil)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) TestPutAndCheckHashCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// If the file is not referenced by another metadata entry, then the file
	// should be left to cleaned up by the object store later on.

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 1)
	s.expectUnlock(hash, 1)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, namespace, hash)
}

func (s *FileObjectStoreSuite) TestPutAndCheckHashDoesNotCleansUpFileOnMetadataFailure(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// The file should not be cleaned up if the file is referenced by another
	// metadata entry. In this case we need to ensure that the file is not
	// cleaned up if the metadata service returns an error.

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 2)
	s.expectUnlock(hash, 2)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: s.calculateHash(c, "some content"),
		Path: "foo",
		Size: 12,
	}).Return(nil)

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, jc.ErrorIsNil)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(errors.Errorf("boom"))

	err = store.PutAndCheckHash(context.Background(), "foo", strings.NewReader("some content"), 12, hash)
	c.Assert(err, gc.ErrorMatches, `.*boom`)

	s.expectFileDoesExist(c, path, namespace, hash)
}

func (s *FileObjectStoreSuite) TestRemoveFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Test that we don't return an error if the file does not exist.
	// We just want to ensure that we don't return an error after the metadata
	// is removed.

	path := c.MkDir()

	namespace := "inferi"
	fileName := "foo"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().GetMetadata(gomock.Any(), fileName).Return(objectstore.Metadata{
		Hash: "blah",
		Path: fileName,
		Size: 666,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	err = store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) TestRemove(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hash := s.calculateHash(c, "some content")
	s.expectLock(hash, 2)
	s.expectUnlock(hash, 2)

	path := c.MkDir()

	namespace := "inferi"

	store, err := NewFileObjectStore(context.Background(), namespace, path, s.service, s.locker, jujutesting.NewCheckLogger(c))
	c.Assert(err, gc.IsNil)
	defer workertest.DirtyKill(c, store)

	s.service.EXPECT().PutMetadata(gomock.Any(), objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}).Return(nil)

	err = store.Put(context.Background(), "foo", strings.NewReader("some content"), 12)
	c.Assert(err, jc.ErrorIsNil)

	s.expectFileDoesExist(c, path, namespace, hash)

	s.service.EXPECT().GetMetadata(gomock.Any(), "foo").Return(objectstore.Metadata{
		Hash: hash,
		Path: "foo",
		Size: 12,
	}, nil)

	s.service.EXPECT().RemoveMetadata(gomock.Any(), "foo").Return(nil)

	err = store.Remove(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.expectFileDoesNotExist(c, path, namespace, hash)
}

func (s *FileObjectStoreSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.service = NewMockObjectStoreMetadata(ctrl)
	s.locker = NewMockLocker(ctrl)

	return ctrl
}

func (s *FileObjectStoreSuite) createFile(c *gc.C, path, name, contents string) (int64, string) {
	// Ensure the directory exists.
	err := os.MkdirAll(path, 0755)
	c.Assert(err, jc.ErrorIsNil)

	// Create a file in a temporary directory.
	dir := c.MkDir()

	f, err := os.Create(filepath.Join(dir, name))
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()

	// Create a hash of the contents when writing the file. The hash will
	// be used as the file name on disk.
	hasher := sha256.New()
	size, err := io.Copy(f, io.TeeReader(strings.NewReader(contents), hasher))
	c.Assert(err, jc.ErrorIsNil)

	info, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)

	if info.Size() != size {
		c.Fatalf("file size %d does not match expected size %d", info.Size(), size)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	err = os.Rename(filepath.Join(dir, name), filepath.Join(path, hash))
	c.Assert(err, jc.ErrorIsNil)

	return info.Size(), hash
}

func (s *FileObjectStoreSuite) readFile(c *gc.C, reader io.ReadCloser) string {
	defer reader.Close()

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	return string(content)
}

func (s *FileObjectStoreSuite) calculateHash(c *gc.C, contents string) string {
	hasher := sha256.New()
	_, err := io.Copy(hasher, strings.NewReader(contents))
	c.Assert(err, jc.ErrorIsNil)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (s *FileObjectStoreSuite) expectFileDoesNotExist(c *gc.C, path, namespace, hash string) {
	_, err := os.Stat(filepath.Join(path, namespace, hash))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
}

func (s *FileObjectStoreSuite) expectFileDoesExist(c *gc.C, path, namespace, hash string) {
	_, err := os.Stat(filepath.Join(path, namespace, hash))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *FileObjectStoreSuite) expectLock(hash string, num int) {
	s.locker.EXPECT().Lock(gomock.Any(), hash).Return(nil).Times(num)
}

func (s *FileObjectStoreSuite) expectUnlock(hash string, num int) {
	s.locker.EXPECT().Unlock(gomock.Any(), hash).Return(nil).Times(num)
}
