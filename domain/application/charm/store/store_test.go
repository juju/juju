// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	objectstoreerrors "github.com/juju/juju/internal/objectstore/errors"
)

type storeSuite struct {
	testing.IsolationSuite

	objectStore       *MockObjectStore
	objectStoreGetter *MockModelObjectStoreGetter
}

var _ = gc.Suite(&storeSuite{})

func (s *storeSuite) TestStore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, contentDigest := s.createTempFile(c, dir, "hello world")

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	var (
		uniqueName string
		contents   string
	)
	s.objectStore.EXPECT().
		PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), contentDigest.Size, contentDigest.SHA384).
		DoAndReturn(func(_ context.Context, name string, reader io.Reader, _ int64, _ string) (objectstore.UUID, error) {
			uniqueName = name

			data, err := io.ReadAll(reader)
			c.Assert(err, jc.ErrorIsNil)
			contents = string(data)

			return uuid, nil
		})

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	storeResult, err := storage.Store(context.Background(), path, contentDigest.Size, contentDigest.SHA384)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(storeResult.ObjectStoreUUID, gc.DeepEquals, uuid)
	c.Check(storeResult.UniqueName, gc.Equals, uniqueName)

	// Make sure the contents are the same and it's not been tampered with.
	c.Check(contents, gc.Equals, "hello world")
}

func (s *storeSuite) TestStoreFileClosed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, contentDigest := s.createTempFile(c, dir, "hello world")

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	var reader io.Reader
	s.objectStore.EXPECT().
		PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), contentDigest.Size, contentDigest.SHA384).
		DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ string) (objectstore.UUID, error) {
			reader = r

			return uuid, nil
		})

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.Store(context.Background(), path, contentDigest.Size, contentDigest.SHA384)
	c.Assert(err, jc.ErrorIsNil)

	// Attempt to read the contents of the read after it's been closed.
	_, err = io.ReadAll(reader)

	c.Assert(err, gc.ErrorMatches, ".*file already closed")
}

func (s *storeSuite) TestStoreFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.Store(context.Background(), filepath.Join(dir, "foo"), 12, "hash")
	c.Assert(err, jc.ErrorIs, ErrNotFound)
}

func (s *storeSuite) TestStoreFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, contentDigest := s.createTempFile(c, dir, "hello world")

	s.objectStore.EXPECT().
		PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), contentDigest.Size, contentDigest.SHA384).
		Return("", errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.Store(context.Background(), path, contentDigest.Size, contentDigest.SHA384)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestStoreFromReader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, contentDigest := s.createTempFile(c, dir, "hello world")
	reader, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	var (
		uniqueName string
		contents   string
	)
	s.objectStore.EXPECT().
		PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), contentDigest.Size, contentDigest.SHA384).
		DoAndReturn(func(_ context.Context, name string, reader io.Reader, _ int64, _ string) (objectstore.UUID, error) {
			uniqueName = name

			data, err := io.ReadAll(reader)
			c.Assert(err, jc.ErrorIsNil)
			contents = string(data)

			return uuid, nil
		})

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	storeResult, digest, err := storage.StoreFromReader(context.Background(), reader, contentDigest.SHA256[:7])
	c.Assert(err, jc.ErrorIsNil)

	c.Check(storeResult.ObjectStoreUUID, gc.DeepEquals, uuid)
	c.Check(storeResult.UniqueName, gc.Equals, uniqueName)

	c.Check(digest, gc.DeepEquals, contentDigest)

	// Make sure the contents are the same and it's not been tampered with.
	c.Check(contents, gc.Equals, "hello world")
}

func (s *storeSuite) TestStoreFromReaderEmptyReader(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	_, contentDigest := s.createTempFile(c, dir, "hello world")
	reader := io.NopCloser(strings.NewReader(""))

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, _, err := storage.StoreFromReader(context.Background(), reader, contentDigest.SHA256[:7])
	c.Assert(err, jc.ErrorIs, ErrCharmHashMismatch)
}

func (s *storeSuite) TestStoreFromReaderInvalidHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, _ := s.createTempFile(c, dir, "hello world")
	reader, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, _, err = storage.StoreFromReader(context.Background(), reader, "blah")
	c.Assert(err, jc.ErrorIs, ErrCharmHashMismatch)
}

func (s *storeSuite) TestGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	archive := io.NopCloser(strings.NewReader("archive-content"))
	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(archive, 0, nil)

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	reader, err := storage.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *storeSuite) TestGetFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(nil, 0, errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))

	_, err := storage.Get(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestGetNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(nil, 0, objectstoreerrors.ObjectNotFound)

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, ErrNotFound)
}

func (s *storeSuite) TestGetBySHA256Prefix(c *gc.C) {
	defer s.setupMocks(c).Finish()

	archive := io.NopCloser(strings.NewReader("archive-content"))
	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(archive, 0, nil)

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	reader, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, jc.ErrorIsNil)
	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *storeSuite) TestGetBySHA256PrefixFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(nil, 0, errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestGetBySHA256NotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(nil, 0, objectstoreerrors.ObjectNotFound)

	storage := NewCharmStore(s.objectStoreGetter, loggertesting.WrapCheckLog(c))
	_, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, jc.ErrorIs, ErrNotFound)
}

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil).AnyTimes()

	return ctrl
}

func (s *storeSuite) createTempFile(c *gc.C, dir, content string) (string, Digest) {
	path := filepath.Join(dir, "test")
	err := os.WriteFile(path, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)

	info, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)

	return path, Digest{
		SHA256: calculateSHA256(c, content),
		SHA384: calculateSHA384(c, content),
		Size:   info.Size(),
	}
}

func calculateSHA384(c *gc.C, content string) string {
	hash := sha512.New384()
	_, err := hash.Write([]byte(content))
	c.Assert(err, jc.ErrorIsNil)
	return hex.EncodeToString(hash.Sum(nil))
}

func calculateSHA256(c *gc.C, content string) string {
	hash := sha256.New()
	_, err := hash.Write([]byte(content))
	c.Assert(err, jc.ErrorIsNil)
	return hex.EncodeToString(hash.Sum(nil))
}
