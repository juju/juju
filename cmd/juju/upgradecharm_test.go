// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"os"
	"path"
)

type UpgradeCharmErrorsSuite struct {
	jujutesting.RepoSuite
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
	testing.Charms.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak", "--repository=blah")
	c.Assert(err, ErrorMatches, `no repository found at ".*blah"`)
	// Reset JUJU_REPOSITORY explicitly, because repoSuite.SetUpTest
	// overwrites it (TearDownTest will revert it again).
	os.Setenv("JUJU_REPOSITORY", "")
	err = runUpgradeCharm(c, "riak", "--repository=")
	c.Assert(err, ErrorMatches, `charm not found in ".*": local:precise/riak`)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidService(c *C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(err, ErrorMatches, `service "phony" not found`)
}

func (s *UpgradeCharmErrorsSuite) TestCannotBumpRevisionWithBundle(c *C) {
	testing.Charms.BundlePath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	err = runUpgradeCharm(c, "riak")
	c.Assert(err, ErrorMatches, `cannot increment revision of charm "local:precise/riak-7": not a directory`)
}

func (s *UpgradeCharmErrorsSuite) deployService(c *C) {
	testing.Charms.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidSwitchURL(c *C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=blah")
	c.Assert(err, ErrorMatches, "charm not found: cs:precise/blah")
	err = runUpgradeCharm(c, "riak", "--switch=cs:missing/one")
	c.Assert(err, ErrorMatches, "charm not found: cs:missing/one")
	// TODO(dimitern): add tests with incompatible charms
}

func (s *UpgradeCharmErrorsSuite) TestSwitchAndRevisionFails(c *C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsSuite) TestInvalidRevision(c *C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--revision=blah")
	c.Assert(err, ErrorMatches, `invalid value "blah" for flag --revision: strconv.ParseInt: parsing "blah": invalid syntax`)
}

type UpgradeCharmSuccessSuite struct {
	jujutesting.RepoSuite
	path string
	riak *state.Service
}

var _ = Suite(&UpgradeCharmSuccessSuite{})

func (s *UpgradeCharmSuccessSuite) SetUpTest(c *C) {
	s.RepoSuite.SetUpTest(c)
	s.path = testing.Charms.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, IsNil)
	s.riak, err = s.State.Service("riak")
	c.Assert(err, IsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, 7)
	c.Assert(forced, Equals, false)
}

func (s *UpgradeCharmSuccessSuite) assertUpgraded(c *C, revision int, forced bool) *charm.URL {
	err := s.riak.Refresh()
	c.Assert(err, IsNil)
	ch, force, err := s.riak.Charm()
	c.Assert(err, IsNil)
	c.Assert(ch.Revision(), Equals, revision)
	c.Assert(force, Equals, forced)
	s.AssertCharmUploaded(c, ch.URL())
	return ch.URL()
}

func (s *UpgradeCharmSuccessSuite) assertLocalRevision(c *C, revision int, path string) {
	dir, err := charm.ReadDir(path)
	c.Assert(err, IsNil)
	c.Assert(dir.Revision(), Equals, revision)
}

func (s *UpgradeCharmSuccessSuite) TestBumpsRevisionWhenNecessary(c *C) {
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 8, false)
	s.assertLocalRevision(c, 8, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestDoesntBumpRevisionWhenNotNecessary(c *C) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 42, false)
	s.assertLocalRevision(c, 42, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestUpgradesWithBundle(c *C) {
	dir, err := charm.ReadDir(s.path)
	c.Assert(err, IsNil)
	dir.SetRevision(42)
	buf := &bytes.Buffer{}
	err = dir.BundleTo(buf)
	c.Assert(err, IsNil)
	bundlePath := path.Join(s.SeriesPath, "riak.charm")
	err = ioutil.WriteFile(bundlePath, buf.Bytes(), 0644)
	c.Assert(err, IsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 42, false)
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestForcedUpgrade(c *C) {
	err := runUpgradeCharm(c, "riak", "--force")
	c.Assert(err, IsNil)
	s.assertUpgraded(c, 8, true)
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
	myriakPath := testing.Charms.RenamedClonedDirPath(s.SeriesPath, "riak", "myriak")
	err := ioutil.WriteFile(path.Join(myriakPath, "metadata.yaml"), myriakMeta, 0644)
	c.Assert(err, IsNil)

	// Test with local repo and no explicit revsion.
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak")
	c.Assert(err, IsNil)
	curl := s.assertUpgraded(c, 7, false)
	c.Assert(curl.String(), Equals, "local:precise/myriak-7")
	s.assertLocalRevision(c, 7, myriakPath)

	// Try it again without revision - should be bumped.
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak")
	c.Assert(err, IsNil)
	curl = s.assertUpgraded(c, 8, false)
	c.Assert(curl.String(), Equals, "local:precise/myriak-8")
	s.assertLocalRevision(c, 8, myriakPath)

	// Now try the same with explicit revision - should fail.
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak-8")
	c.Assert(err, ErrorMatches, `already running specified charm "local:precise/myriak-8"`)

	// Change the revision to 42 and upgrade to it with explicit revision.
	err = ioutil.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, IsNil)
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak-42")
	c.Assert(err, IsNil)
	curl = s.assertUpgraded(c, 42, false)
	c.Assert(curl.String(), Equals, "local:precise/myriak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}
