// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csclientparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc"
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
	}, charmstore.V5)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)

	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
	s.PatchValue(&newCharmStoreClient, func(bakeryClient *httpbakery.Client) *csclient.Client {
		return csclient.New(csclient.Params{
			URL:          s.srv.URL,
			BakeryClient: bakeryClient,
		})
	})
}

func (s *UpgradeCharmErrorsSuite) TearDownTest(c *gc.C) {
	s.handler.Close()
	s.srv.Close()
	s.RepoSuite.TearDownTest(c)
}

var _ = gc.Suite(&UpgradeCharmErrorsSuite{})

func runUpgradeCharm(c *gc.C, args ...string) error {
	_, err := testing.RunCommand(c, NewUpgradeCharmCommand(), args...)
	return err
}

func (s *UpgradeCharmErrorsSuite) TestInvalidArgs(c *gc.C) {
	err := runUpgradeCharm(c)
	c.Assert(err, gc.ErrorMatches, "no application specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmErrorsSuite) TestInvalidService(c *gc.C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "phony" not found`,
		Code:    "not found",
	})
}

func (s *UpgradeCharmErrorsSuite) deployService(c *gc.C) {
	ch := testcharms.Repo.ClonedDirPath(s.CharmsPath, "riak")
	err := runDeploy(c, ch, "riak", "--series", "quantal")
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

func (s *UpgradeCharmErrorsSuite) TestNoPathFails(c *gc.C) {
	s.deployService(c)
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, gc.ErrorMatches, "upgrading a local charm requires either --path or --switch")
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

type BaseUpgradeCharmSuite struct{}

type UpgradeCharmSuccessSuite struct {
	BaseUpgradeCharmSuite
	jujutesting.RepoSuite
	testing.CmdBlockHelper
	path string
	riak *state.Application
}

func (s *BaseUpgradeCharmSuite) assertUpgraded(c *gc.C, riak *state.Application, revision int, forced bool) *charm.URL {
	err := riak.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, revision)
	c.Assert(force, gc.Equals, forced)
	return ch.URL()
}

var _ = gc.Suite(&UpgradeCharmSuccessSuite{})

func (s *UpgradeCharmSuccessSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.path = testcharms.Repo.ClonedDirPath(s.CharmsPath, "riak")
	err := runDeploy(c, s.path, "--series", "quantal")
	c.Assert(err, jc.ErrorIsNil)
	s.riak, err = s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(forced, jc.IsFalse)

	s.CmdBlockHelper = testing.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *UpgradeCharmSuccessSuite) assertLocalRevision(c *gc.C, revision int, path string) {
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, revision)
}

func (s *UpgradeCharmSuccessSuite) TestLocalRevisionUnchanged(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	s.AssertCharmUploaded(c, curl)
	// Even though the remote revision is bumped, the local one should
	// be unchanged.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockUpgradeCharm(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeCharm")
	err := runUpgradeCharm(c, "riak", "--path", s.path)
	s.AssertBlocked(c, err, ".*TestBlockUpgradeCharm.*")
}

func (s *UpgradeCharmSuccessSuite) TestRespectsLocalRevisionWhenPossible(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)

	err = runUpgradeCharm(c, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 42, false)
	s.AssertCharmUploaded(c, curl)
	s.assertLocalRevision(c, 42, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestForcedSeriesUpgrade(c *gc.C) {
	path := testcharms.Repo.ClonedDirPath(c.MkDir(), "multi-series")
	err := runDeploy(c, path, "multi-series", "--series", "precise")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := application.Charm()
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

	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 8)
	c.Assert(force, gc.Equals, false)
	s.AssertCharmUploaded(c, ch.URL())
	c.Assert(ch.URL().String(), gc.Equals, "local:precise/multi-series2-8")
}

func (s *UpgradeCharmSuccessSuite) TestInitWithResources(c *gc.C) {
	testcharms.Repo.CharmArchivePath(s.CharmsPath, "dummy")
	dir := c.MkDir()

	foopath := path.Join(dir, "foo")
	barpath := path.Join(dir, "bar")
	err := ioutil.WriteFile(foopath, []byte("foo"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(barpath, []byte("bar"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := upgradeCharmCommand{}
	args := []string{"dummy", "--resource", res1, "--resource", res2}

	err = testing.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *UpgradeCharmSuccessSuite) TestForcedUnitsUpgrade(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestBlockForcedUnitsUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	err := runUpgradeCharm(c, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessSuite) TestCharmPath(c *gc.C) {
	myriakPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "riak")

	// Change the revision to 42 and upgrade to it with explicit revision.
	err := ioutil.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 42, false)
	c.Assert(curl.String(), gc.Equals, "local:quantal/riak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}

func (s *UpgradeCharmSuccessSuite) TestCharmPathNoRevUpgrade(c *gc.C) {
	// Revision 7 is running to start with.
	myriakPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "riak")
	s.assertLocalRevision(c, 7, myriakPath)
	err := runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	c.Assert(curl.String(), gc.Equals, "local:quantal/riak-8")
}

func (s *UpgradeCharmSuccessSuite) TestCharmPathDifferentNameFails(c *gc.C) {
	myriakPath := testcharms.Repo.RenamedClonedDirPath(s.CharmsPath, "riak", "myriak")
	err := runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade "riak" to "myriak"`)
}

type UpgradeCharmCharmStoreSuite struct {
	BaseUpgradeCharmSuite
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
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?include=id&include=supported-series&include=published": unauthorized: access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	switchURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?include=id&include=supported-series&include=published": unauthorized: access denied for user "client-username"`,
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

func (s *UpgradeCharmCharmStoreSuite) TestSwitch(c *gc.C) {
	testcharms.UploadCharm(c, s.client, "cs:~other/trusty/riak-0", "riak")
	testcharms.UploadCharm(c, s.client, "cs:~other/trusty/anotherriak-7", "riak")
	err := runDeploy(c, "cs:~other/trusty/riak-0")
	c.Assert(err, jc.ErrorIsNil)

	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(forced, jc.IsFalse)

	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, riak, 7, false)
	c.Assert(curl.String(), gc.Equals, "cs:~other/trusty/anotherriak-7")

	// Now try the same with explicit revision - should fail.
	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak-7")
	c.Assert(err, gc.ErrorMatches, `already running specified charm "cs:~other/trusty/anotherriak-7"`)

	// Change the revision to 42 and upgrade to it with explicit revision.
	testcharms.UploadCharm(c, s.client, "cs:~other/trusty/anotherriak-42", "riak")
	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak-42")
	c.Assert(err, jc.ErrorIsNil)
	curl = s.assertUpgraded(c, riak, 42, false)
	c.Assert(curl.String(), gc.Equals, "cs:~other/trusty/anotherriak-42")
}

func (s *UpgradeCharmCharmStoreSuite) TestUpgradeCharmWithChannel(c *gc.C) {
	id, ch := testcharms.UploadCharm(c, s.client, "cs:~client-username/trusty/wordpress-0", "wordpress")
	err := runDeploy(c, "cs:~client-username/trusty/wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	// Upload a new revision of the charm, but publish it
	// only to the beta channel.

	id.Revision = 1
	err = s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)

	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.BetaChannel}, nil)
	c.Assert(err, gc.IsNil)

	err = runUpgradeCharm(c, "wordpress", "--channel", "beta")
	c.Assert(err, gc.IsNil)

	s.assertCharmsUploaded(c, "cs:~client-username/trusty/wordpress-0", "cs:~client-username/trusty/wordpress-1")
	s.assertApplicationsDeployed(c, map[string]serviceInfo{
		"wordpress": {charm: "cs:~client-username/trusty/wordpress-1"},
	})
}

func (s *UpgradeCharmCharmStoreSuite) TestUpgradeWithTermsNotSigned(c *gc.C) {
	id, ch := testcharms.UploadCharm(c, s.client, "quantal/terms1-1", "terms1")
	err := runDeploy(c, "quantal/terms1")
	c.Assert(err, jc.ErrorIsNil)
	id.Revision = id.Revision + 1
	err = s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)
	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.StableChannel}, nil)
	c.Assert(err, gc.IsNil)
	s.termsDischargerError = &httpbakery.Error{
		Message: "term agreement required: term/1 term/2",
		Code:    "term agreement required",
	}
	expectedError := `Declined: please agree to the following terms term/1 term/2. Try: "juju agree term/1 term/2"`
	err = runUpgradeCharm(c, "terms1")
	c.Assert(err, gc.ErrorMatches, expectedError)
}
