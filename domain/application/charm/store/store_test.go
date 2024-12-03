// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"crypto/sha256"
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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
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
	path, size, hash := s.createTempFile(c, dir, "hello world")

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	var (
		uniqueName string
		contents   string
	)
	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), size, hash).DoAndReturn(func(_ context.Context, name string, reader io.Reader, _ int64, _ string) (objectstore.UUID, error) {
		uniqueName = name

		data, err := io.ReadAll(reader)
		c.Assert(err, jc.ErrorIsNil)
		contents = string(data)

		return uuid, nil
	})

	storage := NewCharmStore(s.objectStoreGetter)
	storagePath, result, err := storage.Store(context.Background(), path, size, hash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, gc.DeepEquals, uuid)
	c.Check(uniqueName, gc.Equals, storagePath)

	// Make sure the contents are the same and it's not been tampered with.
	c.Check(contents, gc.Equals, "hello world")
}

func (s *storeSuite) TestStoreFileClosed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, size, hash := s.createTempFile(c, dir, "hello world")

	uuid := objectstoretesting.GenObjectStoreUUID(c)

	var reader io.Reader
	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), size, hash).DoAndReturn(func(_ context.Context, _ string, r io.Reader, _ int64, _ string) (objectstore.UUID, error) {
		reader = r

		return uuid, nil
	})

	storage := NewCharmStore(s.objectStoreGetter)
	_, _, err := storage.Store(context.Background(), path, size, hash)
	c.Assert(err, jc.ErrorIsNil)

	// Attempt to read the contents of the read after it's been closed.
	_, err = io.ReadAll(reader)

	c.Assert(err, gc.ErrorMatches, ".*file already closed")
}

func (s *storeSuite) TestStoreFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	storage := NewCharmStore(s.objectStoreGetter)
	_, _, err := storage.Store(context.Background(), filepath.Join(dir, "foo"), 12, "hash")
	c.Assert(err, jc.ErrorIs, ErrNotFound)
}

func (s *storeSuite) TestStoreFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, size, hash := s.createTempFile(c, dir, "hello world")

	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), size, hash).Return("", errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter)
	_, _, err := storage.Store(context.Background(), path, size, hash)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestGet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	archive := io.NopCloser(strings.NewReader("archive-content"))
	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(archive, 0, nil)

	storage := NewCharmStore(s.objectStoreGetter)
	reader, err := storage.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIsNil)

	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *storeSuite) TestGetFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(nil, 0, errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter)

	_, err := storage.Get(context.Background(), "foo")
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestGetNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().Get(gomock.Any(), "foo").Return(nil, 0, objectstoreerrors.ObjectNotFound)

	storage := NewCharmStore(s.objectStoreGetter)
	_, err := storage.Get(context.Background(), "foo")
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *storeSuite) TestGetBySHA256Prefix(c *gc.C) {
	defer s.setupMocks(c).Finish()

	archive := io.NopCloser(strings.NewReader("archive-content"))
	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(archive, 0, nil)

	storage := NewCharmStore(s.objectStoreGetter)
	reader, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, jc.ErrorIsNil)
	content, err := io.ReadAll(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(content), gc.Equals, "archive-content")
}

func (s *storeSuite) TestGetBySHA256PrefixFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(nil, 0, errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStoreGetter)
	_, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) TestGetBySHA256NotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.objectStore.EXPECT().GetBySHA256Prefix(gomock.Any(), "02638299").Return(nil, 0, objectstoreerrors.ObjectNotFound)

	storage := NewCharmStore(s.objectStoreGetter)
	_, err := storage.GetBySHA256Prefix(context.Background(), "02638299")
	c.Assert(err, jc.ErrorIs, applicationerrors.CharmNotFound)
}

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStore = NewMockObjectStore(ctrl)
	s.objectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	s.objectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.objectStore, nil).AnyTimes()

	return ctrl
}

func (s *storeSuite) createTempFile(c *gc.C, dir, content string) (string, int64, string) {
	path := filepath.Join(dir, "test")
	err := os.WriteFile(path, []byte(content), 0644)
	c.Assert(err, jc.ErrorIsNil)

	info, err := os.Stat(path)
	c.Assert(err, jc.ErrorIsNil)

	return path, info.Size(), calculateHash(content)
}

func calculateHash(content string) string {
	hash := sha256.New()
	hash.Write([]byte(content))
	return hex.EncodeToString(hash.Sum(nil))
}
