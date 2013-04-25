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

func (s *UpgradeCharmErrorsSuite) TestInvalidSwitchURL(c *C) {
	testing.Charms.ClonedDirPath(s.seriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak", "--switch=blah")
	c.Assert(err, ErrorMatches, "charm not found: cs:precise/blah")
	err = runUpgradeCharm(c, "riak", "--switch=cs:missing/one-1")
	c.Assert(err, ErrorMatches, "charm not found: cs:missing/one")
	// TODO(dimitern): add tests with incompatible charms
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

func (s *UpgradeCharmSuccessSuite) assertUpgraded(c *C, revision int, forced bool, curl *charm.URL) {
	err := s.riak.Refresh()
	c.Assert(err, IsNil)
	ch, force, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, revision)
	c.Assert(force, Equals, forced)
	s.assertCharmUploaded(c, ch.URL())
	if curl != nil {
		c.Assert(ch.URL().String(), Equals, curl.String())
	}
}

func (s *UpgradeCharmSuccessSuite) assertLocalRevision(c *C, revision int, path string) {
	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, revision)
}

func (s *UpgradeCharmSuccessSuite) TestBumpsRevisionWhenNecessary(c *C) {
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 8, false, nil)
	s.assertLocalRevision(c, 8, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestDoesntBumpRevisionWhenNotNecessary(c *C) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 42, false, nil)
	s.assertLocalRevision(c, 42, s.path)
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
	s.assertUpgraded(c, 42, false, nil)
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestForcedUpgrade(c *C) {
	err := runUpgradeCharm(c, "riak", "--force")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 8, true, nil)
	s.assertLocalRevision(c, 8, s.path)
}

var myriakMeta = []byte(`
name: myriak
summary: "K/V storage engine"
description: "Scalable K/V Store in Erlang with Clocks :-)"
provides:
  endpoint:
    interface: http
  admin:
    interface: http
peers:
  ring:
    interface: riak
`)

func (s *UpgradeCharmSuccessSuite) TestSwitch(c *C) {
	myriakPath := testing.Charms.RenamedClonedDirPath(s.seriesPath, "riak", "myriak")
	err := ioutil.WriteFile(path.Join(myriakPath, "metadata.yaml"), myriakMeta, 0644)
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak", "--switch=local:myriak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 7, false, charm.MustParseURL("local:precise/myriak-7"))
	s.assertLocalRevision(c, 7, myriakPath)
}
