// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/goleak"

	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type hashFileStoreSuite struct {
	baseSuite
}

func TestHashFileStoreSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &hashFileStoreSuite{})
}

func (s *hashFileStoreSuite) TestHashExistsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))
	err = accessor.HashExists(c.Context(), "hash")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *hashFileStoreSuite) TestHashExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, tc.ErrorIsNil)

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))
	err = accessor.HashExists(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *hashFileStoreSuite) TestGetByHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	file, err := os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, tc.ErrorIsNil)

	_, err = fmt.Fprintln(file, "inferi")
	c.Assert(err, tc.ErrorIsNil)

	// Note this will include the new line character. This is on purpose and
	// is baked into the implementation.

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))
	reader, size, err := accessor.GetByHash(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, int64(7))

	bytes, err := io.ReadAll(reader)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(bytes), tc.Equals, "inferi\n")
}

func (s *hashFileStoreSuite) TestGetByHashNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))
	_, _, err = accessor.GetByHash(c.Context(), "foo")
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *hashFileStoreSuite) TestDeleteByHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Create(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, tc.ErrorIsNil)

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))

	err = accessor.DeleteByHash(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)

	_, err = os.Stat(filepath.Join(s.namespaceFilePath(dir), "foo"))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *hashFileStoreSuite) TestDeleteByHashNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()
	err := os.MkdirAll(s.namespaceFilePath(dir), 0755)
	c.Assert(err, tc.ErrorIsNil)

	accessor := NewHashFileStore("namespace", dir, loggertesting.WrapCheckLog(c))

	err = accessor.DeleteByHash(c.Context(), "foo")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *hashFileStoreSuite) namespaceFilePath(dir string) string {
	return filepath.Join(dir, "objectstore", "namespace")
}
