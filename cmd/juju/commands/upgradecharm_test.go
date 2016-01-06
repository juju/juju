// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmstore.v5-unstable"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
)

type UpgradeCharmErrorsSuite struct {
	jujutesting.RepoSuite
	handler charmstore.HTTPCloseHandler
	srv     *httptest.Server
}

func (s *UpgradeCharmErrorsSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	// Set up the charm store testing server.
	handler, err := charmstore.NewServer(s.Session.DB("juju-testing"), nil, "", charmstore.ServerParams{
		AuthUsername: "test-user",
		AuthPassword: "test-password",
	}, charmstore.V4)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)

	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
	original := newCharmStoreClient
	s.PatchValue(&newCharmStoreClient, func(httpClient *http.Client) *csClient {
		csclient := original(httpClient)
		csclient.params.URL = s.srv.URL
		return csclient
	})
}

func (s *UpgradeCharmErrorsSuite) TearDownTest(c *gc.C) {
	s.handler.Close()
	s.srv.Close()
	s.RepoSuite.TearDownTest(c)
}

var _ = gc.Suite(&UpgradeCharmErrorsSuite{})

func runUpgradeCharm(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, newUpgradeCharmCommand(), args...)
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
	c.Assert(err, gc.ErrorMatches, `entity not found in ".*": local:trusty/riak`)
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
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:blah": charm or bundle not found`)
	err = runUpgradeCharm(c, "riak", "--switch=cs:missing/one")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:missing/one": charm not found`)
	// TODO(dimitern): add tests with incompatible charms
}

func (s *UpgradeCharmErrorsSuite) TestSwitchAndRevisionFails(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsSuite) TestPathAndRevisionFails(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--path=foo", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--path and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsSuite) TestSwitchAndPathFails(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--path=foo")
	c.Assert(err, gc.ErrorMatches, "--switch and --path are mutually exclusive")
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

func (s *UpgradeCharmSuccessSuite) TestForcedSeriesUpgrade(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(c.MkDir(), "multi-series")
	err := runDeploy(c, path, "multi-series", "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	service, err := s.State.Service("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	// Copy files from a charm supporting a different set of series
	// so we can try an upgrade requiring --force-series.
	for _, f := range []string{"metadata.yaml", "revision"} {
		err = utils.CopyFile(
			filepath.Join(path, f),
			filepath.Join(testcharms.Repo.CharmDirPath("multi-series2"), f))
		c.Assert(err, jc.ErrorIsNil)
	}
	err = runUpgradeCharm(c, "multi-series", "--path", path, "--force-series")
	c.Assert(err, jc.ErrorIsNil)

	err = service.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := service.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 8)
	c.Assert(force, gc.Equals, false)
	s.AssertCharmUploaded(c, ch.URL())
	c.Assert(ch.URL().String(), gc.Equals, "local:precise/multi-series2-8")
}

func (s *UpgradeCharmSuccessSuite) TestForcedUnitsUpgrade(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--force-units")
	c.Assert(err, jc.ErrorIsNil)
	s.assertUpgraded(c, 8, true)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockForcedUnitsUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	err := runUpgradeCharm(c, "riak", "--force-units")
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

func (s *UpgradeCharmSuccessSuite) TestCharmPath(c *gc.C) {
	myriakPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "riak")

	// Change the revision to 42 and upgrade to it with explicit revision.
	err := ioutil.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, 42, false)
	c.Assert(curl.String(), gc.Equals, "local:trusty/riak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}

func (s *UpgradeCharmSuccessSuite) TestCharmPathNoRevUpgrade(c *gc.C) {
	// Revision 7 is running to start with.
	myriakPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "riak")
	s.assertLocalRevision(c, 7, myriakPath)
	err := runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, 8, false)
	c.Assert(curl.String(), gc.Equals, "local:trusty/riak-8")
}

func (s *UpgradeCharmSuccessSuite) TestCharmPathDifferentNameFails(c *gc.C) {
	myriakPath := testcharms.Repo.RenamedClonedDirPath(s.SeriesPath, "riak", "myriak")
	err := runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade "riak" to "myriak"`)
}

type UpgradeCharmCharmStoreSuite struct {
	charmStoreSuite
}

var _ = gc.Suite(&UpgradeCharmCharmStoreSuite{})

var upgradeCharmAuthorizationTests = []struct {
	about        string
	uploadURL    string
	switchURL    string
	readPermUser string
	expectError  string
}{{
	about:     "public charm, success",
	uploadURL: "cs:~bob/trusty/wordpress1-10",
	switchURL: "cs:~bob/trusty/wordpress1",
}, {
	about:     "public charm, fully resolved, success",
	uploadURL: "cs:~bob/trusty/wordpress2-10",
	switchURL: "cs:~bob/trusty/wordpress2-10",
}, {
	about:        "non-public charm, success",
	uploadURL:    "cs:~bob/trusty/wordpress3-10",
	switchURL:    "cs:~bob/trusty/wordpress3",
	readPermUser: clientUserName,
}, {
	about:        "non-public charm, fully resolved, success",
	uploadURL:    "cs:~bob/trusty/wordpress4-10",
	switchURL:    "cs:~bob/trusty/wordpress4-10",
	readPermUser: clientUserName,
}, {
	about:        "non-public charm, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress5-10",
	switchURL:    "cs:~bob/trusty/wordpress5",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id&include=supported-series": unauthorized: access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	switchURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?include=id&include=supported-series": unauthorized: access denied for user "client-username"`,
}}

func (s *UpgradeCharmCharmStoreSuite) TestUpgradeCharmAuthorization(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "cs:~other/trusty/wordpress-0", "wordpress")
	err := runDeploy(c, "cs:~other/trusty/wordpress-0")
	c.Assert(err, jc.ErrorIsNil)
	for i, test := range upgradeCharmAuthorizationTests {
		c.Logf("test %d: %s", i, test.about)
		url, _ := testcharms.UploadCharm(c, s.client, test.uploadURL, "wordpress")
		if test.readPermUser != "" {
			s.changeReadPerm(c, url, test.readPermUser)
		}
		err := runUpgradeCharm(c, "wordpress", "--switch", test.switchURL)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
	}
}
