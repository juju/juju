// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bzr_test

import (
	"os"
	"os/exec"
	"testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/bzr"
)

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&BzrSuite{})

type BzrSuite struct {
	b *bzr.Branch
}

func (s *BzrSuite) SetUpTest(c *C) {
	s.b = bzr.New(c.MkDir())
	c.Assert(s.b.Init(), IsNil)
}

func (s *BzrSuite) TestNewFindsRoot(c *C) {
	err := os.Mkdir(s.b.Join("dir"), 0755)
	c.Assert(err, IsNil)
	b := bzr.New(s.b.Join("dir"))
	c.Assert(b.Location(), Equals, s.b.Location())
}

func (s *BzrSuite) TestJoin(c *C) {
	path := bzr.New("lp:foo").Join("baz", "bar")
	c.Assert(path, Equals, "lp:foo/baz/bar")
}

func (s *BzrSuite) TestErrorHandling(c *C) {
	err := bzr.New("/non/existent/path").Init()
	c.Assert(err, ErrorMatches, `(?s)error running "bzr init":.*does not exist.*`)
}

func (s *BzrSuite) TestInit(c *C) {
	_, err := os.Stat(s.b.Join(".bzr"))
	c.Assert(err, IsNil)
}

func (s *BzrSuite) TestRevisionIdOnEmpty(c *C) {
	revid, err := s.b.RevisionId()
	c.Assert(err, ErrorMatches, "branch has no content")
	c.Assert(revid, Equals, "")
}

func (s *BzrSuite) TestCommit(c *C) {
	f, err := os.Create(s.b.Join("myfile"))
	c.Assert(err, IsNil)
	f.Close()
	err = s.b.Add("myfile")
	c.Assert(err, IsNil)
	err = s.b.Commit("my log message")
	c.Assert(err, IsNil)

	revid, err := s.b.RevisionId()
	c.Assert(err, IsNil)

	cmd := exec.Command("bzr", "log", "--long", "--show-ids", "-v", s.b.Location())
	output, err := cmd.CombinedOutput()
	c.Assert(err, IsNil)
	c.Assert(string(output), Matches, "(?s).*revision-id: "+revid+"\n.*message:\n.*my log message\n.*added:\n.*myfile .*")
}

func (s *BzrSuite) TestPush(c *C) {
	b1 := bzr.New(c.MkDir())
	b2 := bzr.New(c.MkDir())
	b3 := bzr.New(c.MkDir())
	c.Assert(b1.Init(), IsNil)
	c.Assert(b2.Init(), IsNil)
	c.Assert(b3.Init(), IsNil)

	// Create and add b1/file to the branch.
	f, err := os.Create(b1.Join("file"))
	c.Assert(err, IsNil)
	f.Close()
	err = b1.Add("file")
	c.Assert(err, IsNil)
	err = b1.Commit("added file")
	c.Assert(err, IsNil)

	// Push file to b2.
	err = b1.Push(&bzr.PushAttr{Location: b2.Location()})
	c.Assert(err, IsNil)

	// Push location should be set to b2.
	location, err := b1.PushLocation()
	c.Assert(err, IsNil)
	c.Assert(location, Equals, b2.Location())

	// Now push it to b3.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location()})
	c.Assert(err, IsNil)

	// Push location is still set to b2.
	location, err = b1.PushLocation()
	c.Assert(err, IsNil)
	c.Assert(location, Equals, b2.Location())

	// Push it again, this time with the remember flag set.
	err = b1.Push(&bzr.PushAttr{Location: b3.Location(), Remember: true})
	c.Assert(err, IsNil)

	// Now the push location has shifted to b3.
	location, err = b1.PushLocation()
	c.Assert(err, IsNil)
	c.Assert(location, Equals, b3.Location())

	// Both b2 and b3 should have the file.
	_, err = os.Stat(b2.Join("file"))
	c.Assert(err, IsNil)
	_, err = os.Stat(b3.Join("file"))
	c.Assert(err, IsNil)
}

func (s *BzrSuite) TestCheckClean(c *C) {
	err := s.b.CheckClean()
	c.Assert(err, IsNil)

	// Create and add b1/file to the branch.
	f, err := os.Create(s.b.Join("file"))
	c.Assert(err, IsNil)
	f.Close()

	err = s.b.CheckClean()
	c.Assert(err, ErrorMatches, `branch is not clean \(bzr status\)`)
}
