// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package bzr_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	stdtesting "testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/bzr"
	"github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&BzrSuite{})

type BzrSuite struct {
	testing.BaseSuite
	b *bzr.Branch
}

const bzr_config = `[DEFAULT]
email = testing <test@example.com>
`

func (s *BzrSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	bzrdir := c.MkDir()
	s.PatchEnvironment("BZR_HOME", bzrdir)
	err := os.MkdirAll(filepath.Join(bzrdir, bzrHome), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(
		filepath.Join(bzrdir, bzrHome, "bazaar.conf"),
		[]byte(bzr_config), 0644)
	c.Assert(err, jc.ErrorIsNil)
	s.b = bzr.New(c.MkDir())
	c.Assert(s.b.Init(), gc.IsNil)
}

func (s *BzrSuite) TestNewFindsRoot(c *gc.C) {
	err := os.Mkdir(s.b.Join("dir"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	b := bzr.New(s.b.Join("dir"))
	// When bzr has to search for the root, it will expand any symlinks it
	// found along the way.
	path, err := filepath.EvalSymlinks(s.b.Location())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(b.Location(), jc.SamePath, path)
}

func (s *BzrSuite) TestJoin(c *gc.C) {
	path := bzr.New("lp:foo").Join("baz", "bar")
	c.Assert(path, gc.Equals, "lp:foo/baz/bar")
}

func (s *BzrSuite) TestErrorHandling(c *gc.C) {
	err := bzr.New("/non/existent/path").Init()
	c.Assert(err, gc.ErrorMatches, `(?s)error running "bzr init":.*does not exist.*`)
}

func (s *BzrSuite) TestInit(c *gc.C) {
	_, err := os.Stat(s.b.Join(".bzr"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BzrSuite) TestRevisionIdOnEmpty(c *gc.C) {
	revid, err := s.b.RevisionId()
	c.Assert(err, gc.ErrorMatches, "branch has no content")
	c.Assert(revid, gc.Equals, "")
}

func (s *BzrSuite) TestCommit(c *gc.C) {
	f, err := os.Create(s.b.Join("myfile"))
	c.Assert(err, jc.ErrorIsNil)
	f.Close()
	err = s.b.Add("myfile")
	c.Assert(err, jc.ErrorIsNil)
	err = s.b.Commit("my log message")
	c.Assert(err, jc.ErrorIsNil)

	revid, err := s.b.RevisionId()
	c.Assert(err, jc.ErrorIsNil)

	cmd := exec.Command("bzr", "log", "--long", "--show-ids", "-v", s.b.Location())
	output, err := cmd.CombinedOutput()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(output), gc.Matches, "(?s).*revision-id: "+revid+"\n.*message:\n.*my log message\n.*added:\n.*myfile .*")
}

func (s *BzrSuite) TestPush(c *gc.C) {
	b1 := bzr.New(c.MkDir())
	b2 := bzr.New(c.MkDir())
	b3 := bzr.New(c.MkDir())
	c.Assert(b1.Init(), gc.IsNil)
	c.Assert(b2.Init(), gc.IsNil)
	c.Assert(b3.Init(), gc.IsNil)

	// Create and add b1/file to the branch.
	f, err := os.Create(b1.Join("file"))
	c.Assert(err, jc.ErrorIsNil)
	f.Close()
	err = b1.Add("file")
	c.Assert(err, jc.ErrorIsNil)
	err = b1.Commit("added file")
	c.Assert(err, jc.ErrorIsNil)

	// Push file to b2.
	err = b1.Push(&bzr.PushAttr{Location: b2.Location()})
	c.Assert(err, jc.ErrorIsNil)

	// Push location should be set to b2.
	location, err := b1.PushLocation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(location, jc.SamePath, b2.Location())

	// Now push it to b3.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location()})
	c.Assert(err, jc.ErrorIsNil)

	// Push location is still set to b2.
	location, err = b1.PushLocation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(location, jc.SamePath, b2.Location())

	// Push it again, this time with the remember flag set.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location(), Remember: true})
	c.Assert(err, jc.ErrorIsNil)

	// Now the push location has shifted to b3.
	location, err = b1.PushLocation()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(location, jc.SamePath, b3.Location())

	// Both b2 and b3 should have the file.
	_, err = os.Stat(b2.Join("file"))
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(b3.Join("file"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BzrSuite) TestCheckClean(c *gc.C) {
	err := s.b.CheckClean()
	c.Assert(err, jc.ErrorIsNil)

	// Create and add b1/file to the branch.
	f, err := os.Create(s.b.Join("file"))
	c.Assert(err, jc.ErrorIsNil)
	f.Close()

	err = s.b.CheckClean()
	c.Assert(err, gc.ErrorMatches, `branch is not clean \(bzr status\)`)
}
