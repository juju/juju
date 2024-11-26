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

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/objectstore"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	"github.com/juju/juju/internal/errors"
)

type storeSuite struct {
	testing.IsolationSuite

	objectStore *MockObjectStore
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

	storage := NewCharmStore(s.objectStore)
	result, err := storage.Store(context.Background(), "foo", path, size, hash)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, gc.DeepEquals, uuid)
	c.Check(uniqueName, gc.Matches, "foo-.*")

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

	storage := NewCharmStore(s.objectStore)
	_, err := storage.Store(context.Background(), "foo", path, size, hash)
	c.Assert(err, jc.ErrorIsNil)

	// Attempt to read the contents of the read after it's been closed.
	_, err = io.ReadAll(reader)

	c.Assert(err, gc.ErrorMatches, ".*file already closed")
}

func (s *storeSuite) TestStoreFileNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	storage := NewCharmStore(s.objectStore)
	_, err := storage.Store(context.Background(), "foo", filepath.Join(dir, "foo"), 12, "hash")
	c.Assert(err, jc.ErrorIs, ErrNotFound)
}

func (s *storeSuite) TestStoreFailed(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	path, size, hash := s.createTempFile(c, dir, "hello world")

	s.objectStore.EXPECT().PutAndCheckHash(gomock.Any(), gomock.Any(), gomock.Any(), size, hash).Return("", errors.Errorf("boom"))

	storage := NewCharmStore(s.objectStore)
	_, err := storage.Store(context.Background(), "foo", path, size, hash)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *storeSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.objectStore = NewMockObjectStore(ctrl)

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
