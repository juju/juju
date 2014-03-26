// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	ft "launchpad.net/juju-core/testing/filetesting"
	"launchpad.net/juju-core/testing/testbase"
)

type EntrySuite struct {
	testbase.LoggingSuite
	basePath string
}

var _ = gc.Suite(&EntrySuite{})

func (s *EntrySuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.basePath = c.MkDir()
}

func (s *EntrySuite) join(path string) string {
	return filepath.Join(s.basePath, filepath.FromSlash(path))
}

func (s *EntrySuite) TestFileCreate(c *gc.C) {
	ft.File{"foobar", "hello", 0644}.Create(c, s.basePath)
	path := s.join("foobar")
	info, err := os.Lstat(path)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode()&os.ModePerm, gc.Equals, os.FileMode(0644))
	c.Assert(info.Mode()&os.ModeType, gc.Equals, os.FileMode(0))
	data, err := ioutil.ReadFile(path)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "hello")
}

func (s *EntrySuite) TestFileCreateFailure(c *gc.C) {
	os.Chmod(s.basePath, 0444)
	c.ExpectFailure("should fail to create file")
	ft.File{"foobar", "hello", 0644}.Create(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckSuccess(c *gc.C) {
	ft.File{"furble", "pingle", 0740}.Create(c, s.basePath)
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureBadPerm(c *gc.C) {
	ft.File{"furble", "pingle", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different perms")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureBadData(c *gc.C) {
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different content")
	ft.File{"furble", "wrongle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureNoExist(c *gc.C) {
	c.ExpectFailure("shouldn't find file that does not exist")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureSymlink(c *gc.C) {
	ft.Symlink{"link", "file"}.Create(c, s.basePath)
	ft.File{"file", "content", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept symlink, even if pointing to matching file")
	ft.File{"link", "content", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureDir(c *gc.C) {
	ft.Dir{"furble", 0740}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept dir")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCreate(c *gc.C) {
	ft.Dir{"some/path", 0750}.Create(c, s.basePath)
	info, err := os.Lstat(s.join("some/path"))
	c.Check(err, gc.IsNil)
	c.Check(info.Mode()&os.ModePerm, gc.Equals, os.FileMode(0750))
	c.Check(info.Mode()&os.ModeType, gc.Equals, os.ModeDir)

	info, err = os.Lstat(s.join("some"))
	c.Check(err, gc.IsNil)
	c.Check(info.Mode()&os.ModePerm, gc.Equals, os.FileMode(0750))
	c.Check(info.Mode()&os.ModeType, gc.Equals, os.ModeDir)
}

func (s *EntrySuite) TestDirCreateFailure(c *gc.C) {
	os.Chmod(s.basePath, 0444)
	c.ExpectFailure("should fail to create file")
	ft.Dir{"foobar", 0750}.Create(c, s.basePath)
}

func (s *EntrySuite) TestDirCheck(c *gc.C) {
	ft.Dir{"fooble", 0751}.Create(c, s.basePath)
	ft.Dir{"fooble", 0751}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureNoExist(c *gc.C) {
	c.ExpectFailure("shouldn't find dir that does not exist")
	ft.Dir{"fooble", 0751}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureBadPerm(c *gc.C) {
	ft.Dir{"furble", 0740}.Check(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different perms")
	ft.Dir{"furble", 0755}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureSymlink(c *gc.C) {
	ft.Symlink{"link", "dir"}.Create(c, s.basePath)
	ft.Dir{"dir", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept symlink, even if pointing to matching dir")
	ft.Dir{"link", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureFile(c *gc.C) {
	ft.File{"blah", "content", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept file")
	ft.Dir{"blah", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCreate(c *gc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	target, err := os.Readlink(s.join("link"))
	c.Assert(err, gc.IsNil)
	c.Assert(target, gc.Equals, "target")
}

func (s *EntrySuite) TestSymlinkCreateFailure(c *gc.C) {
	os.Chmod(s.basePath, 0444)
	c.ExpectFailure("should fail to create symlink")
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheck(c *gc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureNoExist(c *gc.C) {
	c.ExpectFailure("should not accept symlink that doesn't exist")
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureBadTarget(c *gc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	c.ExpectFailure("should not accept different target")
	ft.Symlink{"link", "different"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureFile(c *gc.C) {
	ft.File{"link", "target", 0644}.Create(c, s.basePath)
	c.ExpectFailure("should not accept plain file")
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureDir(c *gc.C) {
	ft.Dir{"link", 0755}.Create(c, s.basePath)
	c.ExpectFailure("should not accept dir")
	ft.Symlink{"link", "different"}.Check(c, s.basePath)
}
