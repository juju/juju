// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"bytes"
	"io/ioutil"
	"os"
	"path"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5-unstable"
	"gopkg.in/juju/charm.v5-unstable/charmrepo"
	charmtesting "gopkg.in/juju/charm.v5-unstable/testing"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type UpgradeCharmErrorsSuite struct {
	jujutesting.RepoSuite
}

func (s *UpgradeCharmErrorsSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	mockstore := charmtesting.NewMockStore(c, testcharms.Repo, map[string]int{})
	s.AddCleanup(func(*gc.C) { mockstore.Close() })
	s.PatchValue(&charmrepo.LegacyStore, &charmrepo.LegacyCharmStore{
		BaseURL: mockstore.Address(),
	})
}

var _ = gc.Suite(&UpgradeCharmErrorsSuite{})

func runUpgradeCharm(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, envcmd.Wrap(&UpgradeCharmCommand{}), args...)
	return err
}

func (s *UpgradeCharmErrorsSuite) TestInvalidArgs(c *gc.C) {
	err := runUpgradeCharm(c)
	c.Assert(err, gc.ErrorMatches, "no service specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid service name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmErrorsSuite) TestWithInvalidRepository(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)

	err = runUpgradeCharm(c, "riak", "--repository=blah")
	c.Assert(err, gc.ErrorMatches, `no repository found at ".*blah"`)
	// Reset JUJU_REPOSITORY explicitly, because repoSuite.SetUpTest
	// overwrites it (TearDownTest will revert it again).
	os.Setenv("JUJU_REPOSITORY", "")
	err = runUpgradeCharm(c, "riak", "--repository=")
	c.Assert(err, gc.ErrorMatches, `charm not found in ".*": local:trusty/riak`)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidService(c *gc.C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(err, gc.ErrorMatches, `service "phony" not found`)
}

func (s *UpgradeCharmErrorsSuite) deployService(c *gc.C) {
	testcharms.Repo.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidSwitchURL(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=blah")
	c.Assert(err, gc.ErrorMatches, "charm not found: cs:trusty/blah")
	err = runUpgradeCharm(c, "riak", "--switch=cs:missing/one")
	c.Assert(err, gc.ErrorMatches, "charm not found: cs:missing/one")
	// TODO(dimitern): add tests with incompatible charms
}

func (s *UpgradeCharmErrorsSuite) TestSwitchAndRevisionFails(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsSuite) TestInvalidRevision(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--revision=blah")
	c.Assert(err, gc.ErrorMatches, `invalid value "blah" for flag --revision: strconv.ParseInt: parsing "blah": invalid syntax`)
}

type UpgradeCharmSuccessSuite struct {
	jujutesting.RepoSuite
	CmdBlockHelper
	path string
	riak *state.Service
}

var _ = gc.Suite(&UpgradeCharmSuccessSuite{})

func (s *UpgradeCharmSuccessSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.path = testcharms.Repo.ClonedDirPath(s.SeriesPath, "riak")
	err := runDeploy(c, "local:riak", "riak")
	c.Assert(err, jc.ErrorIsNil)
	s.riak, err = s.State.Service("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(forced, jc.IsFalse)

	s.CmdBlockHelper = NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *UpgradeCharmSuccessSuite) assertUpgraded(c *gc.C, revision int, forced bool) *charm.URL {
	err := s.riak.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, revision)
	c.Assert(force, gc.Equals, forced)
	s.AssertCharmUploaded(c, ch.URL())
	return ch.URL()
}

func (s *UpgradeCharmSuccessSuite) assertLocalRevision(c *gc.C, revision int, path string) {
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, revision)
}

func (s *UpgradeCharmSuccessSuite) TestLocalRevisionUnchanged(c *gc.C) {
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 8, false)
	// Even though the remote revision is bumped, the local one should
	// be unchanged.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockUpgradeCharm(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeCharm")
	err := runUpgradeCharm(c, "riak")
	s.AssertBlocked(c, err, ".*TestBlockUpgradeCharm.*")
}

func (s *UpgradeCharmSuccessSuite) TestRespectsLocalRevisionWhenPossible(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 42, false)
	s.assertLocalRevision(c, 42, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestUpgradesWithBundle(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	dir.SetRevision(42)
	buf := &bytes.Buffer{}
	err = dir.ArchiveTo(buf)
	c.Assert(err, jc.ErrorIsNil)
	bundlePath := path.Join(s.SeriesPath, "riak.charm")
	err = ioutil.WriteFile(bundlePath, buf.Bytes(), 0644)
	c.Assert(err, jc.ErrorIsNil)

	err = runUpgradeCharm(c, "riak")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 42, false)
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockUpgradesWithBundle(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	dir.SetRevision(42)
	buf := &bytes.Buffer{}
	err = dir.ArchiveTo(buf)
	c.Assert(err, jc.ErrorIsNil)
	bundlePath := path.Join(s.SeriesPath, "riak.charm")
	err = ioutil.WriteFile(bundlePath, buf.Bytes(), 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradesWithBundle")
	err = runUpgradeCharm(c, "riak")
	s.AssertBlocked(c, err, ".*TestBlockUpgradesWithBundle.*")
}

func (s *UpgradeCharmSuccessSuite) TestForcedUpgrade(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 8, true)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockForcedUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	err := runUpgradeCharm(c, "riak", "--force")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 8, true)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
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

func (s *UpgradeCharmSuccessSuite) TestSwitch(c *gc.C) {
	myriakPath := testcharms.Repo.RenamedClonedDirPath(s.SeriesPath, "riak", "myriak")
	err := ioutil.WriteFile(path.Join(myriakPath, "metadata.yaml"), myriakMeta, 0644)
	c.Assert(err, jc.ErrorIsNil)

	// Test with local repo and no explicit revsion.
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak")
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, 7, false)
	c.Assert(curl.String(), gc.Equals, "local:trusty/myriak-7")
	s.assertLocalRevision(c, 7, myriakPath)

	// Now try the same with explicit revision - should fail.
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak-7")
	c.Assert(err, gc.ErrorMatches, `already running specified charm "local:trusty/myriak-7"`)

	// Change the revision to 42 and upgrade to it with explicit revision.
	err = ioutil.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runUpgradeCharm(c, "riak", "--switch=local:myriak-42")
	c.Assert(err, jc.ErrorIsNil)
	curl = s.assertUpgraded(c, 42, false)
	c.Assert(curl.String(), gc.Equals, "local:trusty/myriak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}
