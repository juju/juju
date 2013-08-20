// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bzr_test

import (
	"os"
	"os/exec"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/bzr"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&BzrSuite{})

type BzrSuite struct {
	b *bzr.Branch
}

func (s *BzrSuite) SetUpTest(c *gc.C) {
	s.b = bzr.New(c.MkDir())
	c.Assert(s.b.Init(), gc.IsNil)
}

func (s *BzrSuite) TestNewFindsRoot(c *gc.C) {
	err := os.Mkdir(s.b.Join("dir"), 0755)
	c.Assert(err, gc.IsNil)
	b := bzr.New(s.b.Join("dir"))
	c.Assert(b.Location(), gc.Equals, s.b.Location())
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
	c.Assert(err, gc.IsNil)
}

func (s *BzrSuite) TestRevisionIdOnEmpty(c *gc.C) {
	revid, err := s.b.RevisionId()
	c.Assert(err, gc.ErrorMatches, "branch has no content")
	c.Assert(revid, gc.Equals, "")
}

func (s *BzrSuite) TestCommit(c *gc.C) {
	f, err := os.Create(s.b.Join("myfile"))
	c.Assert(err, gc.IsNil)
	f.Close()
	err = s.b.Add("myfile")
	c.Assert(err, gc.IsNil)
	err = s.b.Commit("my log message")
	c.Assert(err, gc.IsNil)

	revid, err := s.b.RevisionId()
	c.Assert(err, gc.IsNil)

	cmd := exec.Command("bzr", "log", "--long", "--show-ids", "-v", s.b.Location())
	output, err := cmd.CombinedOutput()
	c.Assert(err, gc.IsNil)
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
	c.Assert(err, gc.IsNil)
	f.Close()
	err = b1.Add("file")
	c.Assert(err, gc.IsNil)
	err = b1.Commit("added file")
	c.Assert(err, gc.IsNil)

	// Push file to b2.
	err = b1.Push(&bzr.PushAttr{Location: b2.Location()})
	c.Assert(err, gc.IsNil)

	// Push location should be set to b2.
	location, err := b1.PushLocation()
	c.Assert(err, gc.IsNil)
	c.Assert(location, gc.Equals, b2.Location())

	// Now push it to b3.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location()})
	c.Assert(err, gc.IsNil)

	// Push location is still set to b2.
	location, err = b1.PushLocation()
	c.Assert(err, gc.IsNil)
	c.Assert(location, gc.Equals, b2.Location())

	// Push it again, this time with the remember flag set.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location(), Remember: true})
	c.Assert(err, gc.IsNil)

	// Now the push location has shifted to b3.
	location, err = b1.PushLocation()
	c.Assert(err, gc.IsNil)
	c.Assert(location, gc.Equals, b3.Location())

	// Both b2 and b3 should have the file.
	_, err = os.Stat(b2.Join("file"))
	c.Assert(err, gc.IsNil)
	_, err = os.Stat(b3.Join("file"))
	c.Assert(err, gc.IsNil)
}

func (s *BzrSuite) TestCheckClean(c *gc.C) {
	err := s.b.CheckClean()
	c.Assert(err, gc.IsNil)

	// Create and add b1/file to the branch.
	f, err := os.Create(s.b.Join("file"))
	c.Assert(err, gc.IsNil)
	f.Close()

	err = s.b.CheckClean()
	c.Assert(err, gc.ErrorMatches, `branch is not clean \(bzr status\)`)
}
