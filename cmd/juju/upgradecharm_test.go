package main

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"os"
	"path"
)

type UpgradeCharmErrorsSuite struct {
	repoSuite
}

var _ = Suite(&UpgradeCharmErrorsSuite{})

func runUpgradeCharm(c *C, args ...string) error {
	_, err := testing.RunCommand(c, &UpgradeCharmCommand{}, args)
	return err
}

func (s *UpgradeCharmErrorsSuite) TestInvalidArgs(c *C) {
	err := runUpgradeCharm(c)
	c.Assert(err, ErrorMatches, "no service specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, ErrorMatches, `invalid service name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmErrorsSuite) TestWithInvalidRepository(c *C) {
	testing.Charms.ClonedDirPath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak", "--repository=blah")
	c.Assert(err, ErrorMatches, `no repository found at ".*blah"`)
	// Reset JUJU_REPOSITORY explicitly, because repoSuite.SetUpTest
	// overwrites it (TearDownTest will revert it again).
	os.Setenv("JUJU_REPOSITORY", "")
	err = runUpgradeCharm(c, "riak", "--repository=")
	c.Assert(err, ErrorMatches, `no charms found matching "local:precise/riak" in .*`)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidService(c *C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(err, ErrorMatches, `service "phony" not found`)
}

func (s *UpgradeCharmErrorsSuite) TestCannotBumpRevisionWithBundle(c *C) {
	testing.Charms.BundlePath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	err = runUpgradeCharm(c, "riak")
	c.Assert(err, ErrorMatches, `already running latest charm "local:precise/riak-7"`)
}

type UpgradeCharmSuccessSuite struct {
	repoSuite
	path string
	riak *state.Service
}

var _ = Suite(&UpgradeCharmSuccessSuite{})

func (s *UpgradeCharmSuccessSuite) SetUpTest(c *C) {
	s.repoSuite.SetUpTest(c)
	s.path = testing.Charms.ClonedDirPath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	s.riak, err = s.State.Service("riak")
	c.Assert(err, IsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 7)
	c.Assert(forced, Equals, false)
}

func (s *UpgradeCharmSuccessSuite) assertUpgraded(c *C, revision int) {
	err := s.riak.Refresh()
	c.Assert(err, IsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, revision)
	c.Assert(forced, Equals, false)
	s.assertCharmUploaded(c, ch.URL())
}

func (s *UpgradeCharmSuccessSuite) assertLocalRevision(c *C, revision int) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, revision)
}

func (s *UpgradeCharmSuccessSuite) TestBumpsRevisionWhenNecessary(c *C) {
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 8)
	s.assertLocalRevision(c, 8)
}

func (s *UpgradeCharmSuccessSuite) TestDoesntBumpRevisionWhenNotNecessary(c *C) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 42)
	s.assertLocalRevision(c, 42)
}

func (s *UpgradeCharmSuccessSuite) TestUpgradesWithBundle(c *C) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	dir.SetRevision(42)
	buf := &bytes.Buffer{}
	err = dir.BundleTo(buf)
	c.Assert(err, IsNil)
	bundlePath := path.Join(s.seriesPath, "riak.charm")
	err = ioutil.WriteFile(bundlePath, buf.Bytes(), 0644)
	c.Assert(err, IsNil)
	c.Logf("%q %q", bundlePath, s.seriesPath)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 42)
	s.assertLocalRevision(c, 7)
}
