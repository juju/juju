// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package filetesting_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/tc"

	ft "github.com/juju/juju/internal/testhelpers/filetesting"
)

type EntrySuite struct {
	basePath string
}

var _ = tc.Suite(&EntrySuite{})

func (s *EntrySuite) SetUpTest(c *tc.C) {
	s.basePath = c.MkDir()
}

func (s *EntrySuite) join(path string) string {
	return filepath.Join(s.basePath, filepath.FromSlash(path))
}

func (s *EntrySuite) TestFileCreate(c *tc.C) {
	ft.File{"foobar", "hello", 0666}.Create(c, s.basePath)
	path := s.join("foobar")
	info, err := os.Lstat(path)
	c.Assert(err, tc.IsNil)
	c.Assert(info.Mode()&os.ModePerm, tc.Equals, os.FileMode(0666))
	c.Assert(info.Mode()&os.ModeType, tc.Equals, os.FileMode(0))
	data, err := ioutil.ReadFile(path)
	c.Assert(err, tc.IsNil)
	c.Assert(string(data), tc.Equals, "hello")
}

func (s *EntrySuite) TestFileCreateFailure(c *tc.C) {
	c.ExpectFailure("should fail to create file in missing dir")
	ft.File{"missing/foobar", "hello", 0644}.Create(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckSuccess(c *tc.C) {
	ft.File{"furble", "pingle", 0740}.Create(c, s.basePath)
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureBadPerm(c *tc.C) {
	ft.File{"furble", "pingle", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different perms")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureBadData(c *tc.C) {
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different content")
	ft.File{"furble", "wrongle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureNoExist(c *tc.C) {
	c.ExpectFailure("shouldn't find file that does not exist")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureSymlink(c *tc.C) {
	ft.Symlink{"link", "file"}.Create(c, s.basePath)
	ft.File{"file", "content", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept symlink, even if pointing to matching file")
	ft.File{"link", "content", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestFileCheckFailureDir(c *tc.C) {
	ft.Dir{"furble", 0740}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept dir")
	ft.File{"furble", "pingle", 0740}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCreate(c *tc.C) {
	ft.Dir{"path", 0750}.Create(c, s.basePath)
	info, err := os.Lstat(s.join("path"))
	c.Check(err, tc.IsNil)
	c.Check(info.Mode()&os.ModePerm, tc.Equals, os.FileMode(0750))
	c.Check(info.Mode()&os.ModeType, tc.Equals, os.ModeDir)
}

func (s *EntrySuite) TestDirCreateChmod(c *tc.C) {
	ft.Dir{"name", 0750}.Create(c, s.basePath)
	expect := ft.Dir{"name", 0755}.Create(c, s.basePath)
	expect.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCreateSubdir(c *tc.C) {
	subdir := ft.Dir{"some/path", 0750}.Create(c, s.basePath)
	subdir.Check(c, s.basePath)
	ft.Dir{"some", 0750}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCreateFailure(c *tc.C) {
	os.Chmod(s.basePath, 0444)
	defer os.Chmod(s.basePath, 0777)
	c.ExpectFailure("should fail to create file")
	ft.Dir{"foobar", 0750}.Create(c, s.basePath)
}

func (s *EntrySuite) TestDirCheck(c *tc.C) {
	ft.Dir{"fooble", 0751}.Create(c, s.basePath)
	ft.Dir{"fooble", 0751}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureNoExist(c *tc.C) {
	c.ExpectFailure("shouldn't find dir that does not exist")
	ft.Dir{"fooble", 0751}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureBadPerm(c *tc.C) {
	ft.Dir{"furble", 0740}.Check(c, s.basePath)
	c.ExpectFailure("shouldn't pass with different perms")
	ft.Dir{"furble", 0755}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureSymlink(c *tc.C) {
	ft.Symlink{"link", "dir"}.Create(c, s.basePath)
	ft.Dir{"dir", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept symlink, even if pointing to matching dir")
	ft.Dir{"link", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestDirCheckFailureFile(c *tc.C) {
	ft.File{"blah", "content", 0644}.Create(c, s.basePath)
	c.ExpectFailure("shouldn't accept file")
	ft.Dir{"blah", 0644}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCreate(c *tc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	target, err := os.Readlink(s.join("link"))
	c.Assert(err, tc.IsNil)
	c.Assert(target, tc.Equals, "target")
}

func (s *EntrySuite) TestSymlinkCreateFailure(c *tc.C) {
	c.ExpectFailure("should fail to create symlink in missing dir")
	ft.Symlink{"missing/link", "target"}.Create(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheck(c *tc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureNoExist(c *tc.C) {
	c.ExpectFailure("should not accept symlink that doesn't exist")
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureBadTarget(c *tc.C) {
	ft.Symlink{"link", "target"}.Create(c, s.basePath)
	c.ExpectFailure("should not accept different target")
	ft.Symlink{"link", "different"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureFile(c *tc.C) {
	ft.File{"link", "target", 0644}.Create(c, s.basePath)
	c.ExpectFailure("should not accept plain file")
	ft.Symlink{"link", "target"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestSymlinkCheckFailureDir(c *tc.C) {
	ft.Dir{"link", 0755}.Create(c, s.basePath)
	c.ExpectFailure("should not accept dir")
	ft.Symlink{"link", "different"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCreate(c *tc.C) {
	ft.File{"some-file", "content", 0644}.Create(c, s.basePath)
	ft.Removed{"some-file"}.Create(c, s.basePath)
	_, err := os.Lstat(s.join("some-file"))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *EntrySuite) TestRemovedCreateNothing(c *tc.C) {
	ft.Removed{"some-file"}.Create(c, s.basePath)
	_, err := os.Lstat(s.join("some-file"))
	c.Assert(err, tc.Satisfies, os.IsNotExist)
}

func (s *EntrySuite) TestRemovedCreateFailure(c *tc.C) {
	ft.File{"some-file", "content", 0644}.Create(c, s.basePath)
	os.Chmod(s.basePath, 0444)
	defer os.Chmod(s.basePath, 0777)
	c.ExpectFailure("should fail to remove file")
	ft.Removed{"some-file"}.Create(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCheck(c *tc.C) {
	ft.Removed{"some-file"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCheckParentNotDir(c *tc.C) {
	ft.File{"some-dir", "lol-not-a-file", 0644}.Create(c, s.basePath)
	ft.Removed{"some-dir/some-file"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCheckFailureFile(c *tc.C) {
	ft.File{"some-file", "", 0644}.Create(c, s.basePath)
	c.ExpectFailure("should not accept file")
	ft.Removed{"some-file"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCheckFailureDir(c *tc.C) {
	ft.Dir{"some-dir", 0755}.Create(c, s.basePath)
	c.ExpectFailure("should not accept dir")
	ft.Removed{"some-dir"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestRemovedCheckFailureSymlink(c *tc.C) {
	ft.Symlink{"some-link", "target"}.Create(c, s.basePath)
	c.ExpectFailure("should not accept symlink")
	ft.Removed{"some-link"}.Check(c, s.basePath)
}

func (s *EntrySuite) TestCreateCheckChainResults(c *tc.C) {
	for i, test := range (ft.Entries{
		ft.File{"some-file", "content", 0644},
		ft.Dir{"some-dir", 0750},
		ft.Symlink{"some-link", "target"},
		ft.Removed{"missing"},
	}) {
		c.Logf("test %d: %#v", i, test)
		chained := test.Create(c, s.basePath)
		chained = chained.Check(c, s.basePath)
		c.Assert(chained, tc.DeepEquals, test)
	}
}

func (s *EntrySuite) TestEntries(c *tc.C) {
	initial := ft.Entries{
		ft.File{"some-file", "content", 0600},
		ft.Dir{"some-dir", 0750},
		ft.Symlink{"some-link", "target"},
		ft.Removed{"missing"},
	}
	expectRemoveds := ft.Entries{
		ft.Removed{"some-file"},
		ft.Removed{"some-dir"},
		ft.Removed{"some-link"},
		ft.Removed{"missing"},
	}
	removeds := initial.AsRemoveds()
	c.Assert(removeds, tc.DeepEquals, expectRemoveds)

	expectPaths := []string{"some-file", "some-dir", "some-link", "missing"}
	c.Assert(initial.Paths(), tc.DeepEquals, expectPaths)
	c.Assert(removeds.Paths(), tc.DeepEquals, expectPaths)

	chainRemoveds := initial.Create(c, s.basePath).Check(c, s.basePath).AsRemoveds()
	c.Assert(chainRemoveds, tc.DeepEquals, expectRemoveds)
	chainRemoveds = chainRemoveds.Create(c, s.basePath).Check(c, s.basePath)
	c.Assert(chainRemoveds, tc.DeepEquals, expectRemoveds)
}

func (s *EntrySuite) TestEntriesCreateFailure(c *tc.C) {
	c.ExpectFailure("cannot create an entry")
	ft.Entries{
		ft.File{"good", "good", 0750},
		ft.File{"nodir/bad", "bad", 0640},
	}.Create(c, s.basePath)
}

func (s *EntrySuite) TestEntriesCheckFailure(c *tc.C) {
	goodFile := ft.File{"good", "good", 0751}.Create(c, s.basePath)
	c.ExpectFailure("entry does not exist")
	ft.Entries{
		goodFile,
		ft.File{"bad", "", 0750},
	}.Check(c, s.basePath)
}
