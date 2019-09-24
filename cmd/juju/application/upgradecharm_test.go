// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	csclientparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/charmstore.v5"
	"gopkg.in/juju/names.v3"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
	"gopkg.in/macaroon.v2-unstable"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	jujucharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type UpgradeCharmSuite struct {
	testing.IsolationSuite
	testing.Stub

	deployResources   resourceadapters.DeployResourcesFunc
	resolveCharm      ResolveCharmFunc
	resolvedCharmURL  *charm.URL
	apiConnection     mockAPIConnection
	charmAdder        mockCharmAdder
	charmClient       mockCharmClient
	charmAPIClient    mockCharmAPIClient
	modelConfigGetter mockModelConfigGetter
	resourceLister    mockResourceLister
	cmd               cmd.Command
}

var _ = gc.Suite(&UpgradeCharmSuite{})

func (s *UpgradeCharmSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()

	// Create persistent cookies in a temporary location.
	cookieFile := filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookieFile)

	s.deployResources = func(
		applicationID string,
		chID jujucharmstore.CharmID,
		csMac *macaroon.Macaroon,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
	) (ids map[string]string, err error) {
		s.AddCall("DeployResources", applicationID, chID, csMac, filesAndRevisions, resources, conn)
		return nil, s.NextErr()
	}

	s.resolveCharm = func(
		resolveWithChannel func(*charm.URL, csclientparams.Channel) (*charm.URL, csclientparams.Channel, []string, error),
		url *charm.URL,
		preferredChannel csclientparams.Channel,
	) (*charm.URL, csclientparams.Channel, []string, error) {
		s.AddCall("ResolveCharm", resolveWithChannel, url, preferredChannel)
		if err := s.NextErr(); err != nil {
			return nil, csclientparams.NoChannel, nil, err
		}
		return s.resolvedCharmURL, csclientparams.StableChannel, []string{"quantal"}, nil
	}

	currentCharmURL := charm.MustParseURL("cs:quantal/foo-1")
	latestCharmURL := charm.MustParseURL("cs:quantal/foo-2")
	s.resolvedCharmURL = latestCharmURL

	s.apiConnection = mockAPIConnection{
		bestFacadeVersion: 2,
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
	s.charmAdder = mockCharmAdder{}
	s.charmClient = mockCharmClient{
		charmInfo: &charms.CharmInfo{
			Meta: &charm.Meta{},
		},
	}
	s.charmAPIClient = mockCharmAPIClient{charmURL: currentCharmURL}
	s.modelConfigGetter = mockModelConfigGetter{}
	s.resourceLister = mockResourceLister{}

	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models:       map[string]jujuclient.ModelDetails{"admin/bar": {ActiveBranch: model.GenerationMaster}},
	}
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.AddCall("OpenAPI")
		return &s.apiConnection, nil
	}

	s.cmd = NewUpgradeCharmCommandForTest(
		store,
		apiOpen,
		s.deployResources,
		s.resolveCharm,
		func(conn api.Connection, bakeryClient *httpbakery.Client, csURL string, channel csclientparams.Channel) CharmAdder {
			s.AddCall("NewCharmAdder", conn, bakeryClient, csURL, channel)
			s.PopNoErr()
			return &s.charmAdder
		},
		func(conn base.APICallCloser) CharmClient {
			s.AddCall("NewCharmClient", conn)
			s.PopNoErr()
			return &s.charmClient
		},
		func(conn base.APICallCloser) CharmAPIClient {
			s.AddCall("NewCharmAPIClient", conn)
			s.PopNoErr()
			return &s.charmAPIClient
		},
		func(conn base.APICallCloser) ModelConfigGetter {
			s.AddCall("NewModelConfigGetter", conn)
			return &s.modelConfigGetter
		},
		func(conn base.APICallCloser) (ResourceLister, error) {
			s.AddCall("NewResourceLister", conn)
			return &s.resourceLister, s.NextErr()
		},
		func(conn base.APICallCloser) (string, error) {
			s.AddCall("CharmStoreURLGetter", conn)
			return "testing.api.charmstore", s.NextErr()
		},
	)
}

func (s *UpgradeCharmSuite) runUpgradeCharm(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.cmd, args...)
}

func (s *UpgradeCharmSuite) TestStorageConstraints(c *gc.C) {
	_, err := s.runUpgradeCharm(c, "foo", "--storage", "bar=baz")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.StableChannel,
		},
		StorageConstraints: map[string]storage.Constraints{
			"bar": {Pool: "baz", Count: 1},
		},
	})
}

func (s *UpgradeCharmSuite) TestUseConfiguredCharmStoreURL(c *gc.C) {
	_, err := s.runUpgradeCharm(c, "foo")
	c.Assert(err, jc.ErrorIsNil)
	var csURL string
	for _, call := range s.Calls() {
		if call.FuncName == "NewCharmAdder" {
			csURL = call.Args[2].(string)
			break
		}
	}
	c.Assert(csURL, gc.Equals, "testing.api.charmstore")
}

func (s *UpgradeCharmSuite) TestStorageConstraintsMinFacadeVersion(c *gc.C) {
	s.apiConnection.bestFacadeVersion = 1
	_, err := s.runUpgradeCharm(c, "foo", "--storage", "bar=baz")
	c.Assert(err, gc.ErrorMatches,
		"updating storage constraints at upgrade-charm time is not supported by server version 1.2.3")
}

func (s *UpgradeCharmSuite) TestStorageConstraintsMinFacadeVersionNoServerVersion(c *gc.C) {
	s.apiConnection.bestFacadeVersion = 1
	s.apiConnection.serverVersion = nil
	_, err := s.runUpgradeCharm(c, "foo", "--storage", "bar=baz")
	c.Assert(err, gc.ErrorMatches,
		"updating storage constraints at upgrade-charm time is not supported by this server")
}

func (s *UpgradeCharmSuite) TestConfigSettings(c *gc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runUpgradeCharm(c, "foo", "--config", configFile)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.StableChannel,
		},
		ConfigSettingsYAML: "foo:{}",
	})
}

func (s *UpgradeCharmSuite) TestConfigSettingsMinFacadeVersion(c *gc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := ioutil.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	s.apiConnection.bestFacadeVersion = 1
	_, err = s.runUpgradeCharm(c, "foo", "--config", configFile)
	c.Assert(err, gc.ErrorMatches,
		"updating config at upgrade-charm time is not supported by server version 1.2.3")
}

type UpgradeCharmErrorsStateSuite struct {
	jujutesting.RepoSuite
	handler charmstore.HTTPCloseHandler
	srv     *httptest.Server
}

func (s *UpgradeCharmErrorsStateSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	// Set up the charm store testing server.
	handler, err := charmstore.NewServer(s.Session.DB("juju-testing"), nil, "", charmstore.ServerParams{
		AuthUsername: "test-user",
		AuthPassword: "test-password",
	}, charmstore.V5)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.srv = httptest.NewServer(handler)
	s.AddCleanup(func(*gc.C) {
		s.handler.Close()
		s.srv.Close()
	})

	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
	s.PatchValue(&getCharmStoreAPIURL, func(base.APICallCloser) (string, error) {
		return s.srv.URL, nil
	})
}

var _ = gc.Suite(&UpgradeCharmErrorsStateSuite{})

func runUpgradeCharm(c *gc.C, args ...string) error {
	_, err := cmdtesting.RunCommand(c, NewUpgradeCharmCommand(), args...)
	return err
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidArgs(c *gc.C) {
	err := runUpgradeCharm(c)
	c.Assert(err, gc.ErrorMatches, "no application specified")
	err = runUpgradeCharm(c, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	err = runUpgradeCharm(c, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidApplication(c *gc.C) {
	err := runUpgradeCharm(c, "phony")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "phony" not found`,
		Code:    "not found",
	})
}

func (s *UpgradeCharmErrorsStateSuite) deployApplication(c *gc.C) {
	ch := testcharms.RepoWithSeries("bionic").ClonedDirPath(s.CharmsPath, "riak")
	err := runDeploy(c, ch, "riak", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidSwitchURL(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak", "--switch=blah")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:blah": charm or bundle not found`)
	err = runUpgradeCharm(c, "riak", "--switch=cs:missing/one")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:missing/one": charm not found`)
	// TODO(dimitern): add tests with incompatible charms
}

func (s *UpgradeCharmErrorsStateSuite) TestNoPathFails(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak")
	c.Assert(err, gc.ErrorMatches, "upgrading a local charm requires either --path or --switch")
}

func (s *UpgradeCharmErrorsStateSuite) TestSwitchAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestPathAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak", "--path=foo", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--path and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestSwitchAndPathFails(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak", "--switch=riak", "--path=foo")
	c.Assert(err, gc.ErrorMatches, "--switch and --path are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidRevision(c *gc.C) {
	s.deployApplication(c)
	err := runUpgradeCharm(c, "riak", "--revision=blah")
	c.Assert(err, gc.ErrorMatches, `invalid value "blah" for option --revision: strconv.(ParseInt|Atoi): parsing "blah": invalid syntax`)
}

type BaseUpgradeCharmStateSuite struct{}

type UpgradeCharmSuccessStateSuite struct {
	BaseUpgradeCharmStateSuite
	jujutesting.RepoSuite
	coretesting.CmdBlockHelper
	path string
	riak *state.Application
}

func (s *BaseUpgradeCharmStateSuite) assertUpgraded(c *gc.C, riak *state.Application, revision int, forced bool) *charm.URL {
	err := riak.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, revision)
	c.Assert(force, gc.Equals, forced)
	return ch.URL()
}

var _ = gc.Suite(&UpgradeCharmSuccessStateSuite{})

func (s *UpgradeCharmSuccessStateSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	s.path = testcharms.RepoWithSeries("bionic").ClonedDirPath(s.CharmsPath, "riak")
	err := runDeploy(c, s.path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:bionic/riak-7")
	s.riak, _ = s.RepoSuite.AssertApplication(c, "riak", curl, 1, 1)

	_, forced, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(forced, jc.IsFalse)

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *UpgradeCharmSuccessStateSuite) assertLocalRevision(c *gc.C, revision int, path string) {
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, revision)
}

func (s *UpgradeCharmSuccessStateSuite) TestLocalRevisionUnchanged(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	s.AssertCharmUploaded(c, curl)
	// Even though the remote revision is bumped, the local one should
	// be unchanged.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessStateSuite) TestBlockUpgradeCharm(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeCharm")
	err := runUpgradeCharm(c, "riak", "--path", s.path)
	s.AssertBlocked(c, err, ".*TestBlockUpgradeCharm.*")
}

func (s *UpgradeCharmSuccessStateSuite) TestRespectsLocalRevisionWhenPossible(c *gc.C) {
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

func (s *UpgradeCharmSuccessStateSuite) TestForcedSeriesUpgrade(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
	err := runDeploy(c, path, "multi-series", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	unit := units[0]
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, make([]error, len(units)))

	// Overwrite the metadata.yaml to change the supported series.
	metadataPath := filepath.Join(path, "metadata.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metadata.yaml for overwriting"))
	}
	defer file.Close()

	metadata := strings.Join(
		[]string{
			`name: multi-series`,
			`summary: "That's a dummy charm with multi-series."`,
			`description: |`,
			`    This is a longer description which`,
			`    potentially contains multiple lines.`,
			`series:`,
			`    - trusty`,
			`    - wily`,
			`    - bionic`,
		},
		"\n",
	)
	if _, err := file.WriteString(metadata); err != nil {
		c.Fatal(errors.Annotate(err, "cannot write to metadata.yaml"))
	}

	err = runUpgradeCharm(c, "multi-series", "--path", path, "--force-series")
	c.Assert(err, jc.ErrorIsNil)

	err = application.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	ch, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch.Revision(), gc.Equals, 2)
	c.Check(force, gc.Equals, false)
}

func (s *UpgradeCharmSuccessStateSuite) TestForcedLXDProfileUpgrade(c *gc.C) {
	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "lxd-profile-alt")
	err := runDeploy(c, path, "lxd-profile-alt", "--to", "lxd")
	c.Assert(err, jc.ErrorIsNil)
	application, err := s.State.Application("lxd-profile-alt")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 0)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	unit := units[0]

	container, err := s.State.AddMachineInsideNewMachine(
		state.MachineTemplate{
			Series: "bionic",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		state.MachineTemplate{ // parent
			Series: "bionic",
			Jobs:   []state.MachineJob{state.JobHostUnits},
		},
		instance.LXD,
	)
	c.Assert(err, jc.ErrorIsNil)

	err = unit.AssignToMachine(container)
	c.Assert(err, jc.ErrorIsNil)

	// Overwrite the lxd-profile.yaml to change the supported series.
	lxdProfilePath := filepath.Join(path, "lxd-profile.yaml")
	file, err := os.OpenFile(lxdProfilePath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open lxd-profile.yaml for overwriting"))
	}
	defer file.Close()

	lxdProfile := `
description: lxd profile for testing
config:
  security.nesting: "true"
  security.privileged: "true"
  linux.kernel_modules: openvswitch,nbd,ip_tables,ip6_tables
  environment.http_proxy: ""
  boot.autostart.delay: 1
devices: {}
`
	if _, err := file.WriteString(lxdProfile); err != nil {
		c.Fatal(errors.Annotate(err, "cannot write to lxd-profile.yaml"))
	}

	err = runUpgradeCharm(c, "lxd-profile-alt", "--path", path)
	c.Assert(err, gc.ErrorMatches, `invalid lxd-profile.yaml: contains config value "boot.autostart.delay"`)
}

func (s *UpgradeCharmSuccessStateSuite) TestInitWithResources(c *gc.C) {
	testcharms.RepoWithSeries("bionic").CharmArchivePath(s.CharmsPath, "dummy")
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

	err = cmdtesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *UpgradeCharmSuccessStateSuite) TestForcedUnitsUpgrade(c *gc.C) {
	err := runUpgradeCharm(c, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessStateSuite) TestBlockForcedUnitsUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	err := runUpgradeCharm(c, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessStateSuite) TestCharmPath(c *gc.C) {
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")

	// Change the revision to 42 and upgrade to it with explicit revision.
	err := ioutil.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 42, false)
	c.Assert(curl.String(), gc.Equals, "local:bionic/riak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}

func (s *UpgradeCharmSuccessStateSuite) TestCharmPathNoRevUpgrade(c *gc.C) {
	// Revision 7 is running to start with.
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	s.assertLocalRevision(c, 7, myriakPath)
	err := runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	c.Assert(curl.String(), gc.Equals, "local:bionic/riak-8")
}

func (s *UpgradeCharmSuccessStateSuite) TestCharmPathDifferentNameFails(c *gc.C) {
	myriakPath := testcharms.RepoWithSeries("bionic").RenamedClonedDirPath(s.CharmsPath, "riak", "myriak")
	metadataPath := filepath.Join(myriakPath, "metadata.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metadata.yaml"))
	}
	defer file.Close()

	// Overwrite the metadata.yaml to contain a new name.
	newMetadata := strings.Join([]string{`name: myriak`, `summary: ""`, `description: ""`}, "\n")
	if _, err := file.WriteString(newMetadata); err != nil {
		c.Fatal("cannot write to metadata.yaml")
	}
	err = runUpgradeCharm(c, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade "riak" to "myriak"`)
}

type UpgradeCharmCharmStoreStateSuite struct {
	BaseUpgradeCharmStateSuite
	legacyCharmStoreSuite
}

var _ = gc.Suite(&UpgradeCharmCharmStoreStateSuite{})

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
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress5": cannot get "/~bob/trusty/wordpress5/meta/any\?channel=stable&include=id&include=supported-series&include=published": access denied for user "client-username"`,
}, {
	about:        "non-public charm, fully resolved, access denied",
	uploadURL:    "cs:~bob/trusty/wordpress6-47",
	switchURL:    "cs:~bob/trusty/wordpress6-47",
	readPermUser: "bob",
	expectError:  `cannot resolve charm URL "cs:~bob/trusty/wordpress6-47": cannot get "/~bob/trusty/wordpress6-47/meta/any\?channel=stable&include=id&include=supported-series&include=published": access denied for user "client-username"`,
}}

func (s *UpgradeCharmCharmStoreStateSuite) TestUpgradeCharmAuthorization(c *gc.C) {
	testcharms.UploadCharmWithSeries(c, s.client, "cs:~other/trusty/wordpress-0", "wordpress", "bionic")
	err := runDeploy(c, "cs:~other/trusty/wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	riak, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(forced, jc.IsFalse)

	unit, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil})

	c.Assert(err, jc.ErrorIsNil)
	for i, test := range upgradeCharmAuthorizationTests {
		c.Logf("test %d: %s", i, test.about)
		url, _ := testcharms.UploadCharmWithSeries(c, s.client, test.uploadURL, "wordpress", "bionic")
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

func (s *UpgradeCharmCharmStoreStateSuite) TestSwitch(c *gc.C) {
	testcharms.UploadCharmWithSeries(c, s.client, "cs:~other/trusty/riak-0", "riak", "bionic")
	testcharms.UploadCharmWithSeries(c, s.client, "cs:~other/trusty/anotherriak-7", "riak", "bionic")
	err := runDeploy(c, "cs:~other/trusty/riak-0")
	c.Assert(err, jc.ErrorIsNil)

	riak, err := s.State.Application("riak")
	c.Assert(err, jc.ErrorIsNil)
	ch, forced, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(forced, jc.IsFalse)

	unit, err := s.State.Unit("riak/0")
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil})

	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, riak, 7, false)
	c.Assert(curl.String(), gc.Equals, "cs:~other/trusty/anotherriak-7")

	// Now try the same with explicit revision - should fail.
	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak-7")
	c.Assert(err, gc.ErrorMatches, `already running specified charm "cs:~other/trusty/anotherriak-7"`)

	// Change the revision to 42 and upgrade to it with explicit revision.
	testcharms.UploadCharmWithSeries(c, s.client, "cs:~other/trusty/anotherriak-42", "riak", "bionic")
	err = runUpgradeCharm(c, "riak", "--switch=cs:~other/trusty/anotherriak-42")
	c.Assert(err, jc.ErrorIsNil)
	curl = s.assertUpgraded(c, riak, 42, false)
	c.Assert(curl.String(), gc.Equals, "cs:~other/trusty/anotherriak-42")
}

func (s *UpgradeCharmCharmStoreStateSuite) TestUpgradeCharmWithChannel(c *gc.C) {
	id, ch := testcharms.UploadCharmWithSeries(c, s.client, "cs:~client-username/trusty/wordpress-0", "wordpress", "bionic")
	err := runDeploy(c, "cs:~client-username/trusty/wordpress-0")
	c.Assert(err, jc.ErrorIsNil)

	unit, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil})

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
	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"wordpress": {charm: "cs:~client-username/trusty/wordpress-1", config: ch.Config().DefaultSettings()},
	})
}

func (s *UpgradeCharmCharmStoreStateSuite) TestUpgradeCharmShouldRespectDeployedChannelByDefault(c *gc.C) {
	id, ch := testcharms.UploadCharmWithSeries(c, s.client, "cs:~client-username/trusty/wordpress-0", "wordpress", "bionic")

	// publish charm to beta channel
	id.Revision = 1
	err := s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)
	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.BetaChannel}, nil)
	c.Assert(err, gc.IsNil)

	// deploy from beta channel
	err = runDeploy(c, "cs:~client-username/trusty/wordpress-1")
	c.Assert(err, jc.ErrorIsNil)

	unit, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, []error{nil})

	// publish revision 2 to stable channel
	id.Revision = 2
	err = s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)
	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.BetaChannel}, nil)
	c.Assert(err, gc.IsNil)

	// publish revision 3 to beta channel
	id.Revision = 3
	err = s.client.UploadCharmWithRevision(id, ch, -1)
	c.Assert(err, gc.IsNil)
	err = s.client.Publish(id, []csclientparams.Channel{csclientparams.StableChannel}, nil)
	c.Assert(err, gc.IsNil)

	// running upgrade charm without specifying a channel should use the
	// beta channel by default, not the stable channel, since we originally deployed
	// from beta
	err = runUpgradeCharm(c, "wordpress")
	c.Assert(err, gc.IsNil)

	s.assertApplicationsDeployed(c, map[string]applicationInfo{
		"wordpress": {charm: "cs:~client-username/trusty/wordpress-2", config: ch.Config().DefaultSettings()},
	})
}

func (s *UpgradeCharmCharmStoreStateSuite) TestUpgradeWithTermsNotSigned(c *gc.C) {
	id, ch := testcharms.UploadCharmWithSeries(c, s.client, "bionic/terms1-1", "terms1", "bionic")
	err := runDeploy(c, "bionic/terms1")
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
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	err = runUpgradeCharm(c, "terms1")
	c.Assert(err, gc.ErrorMatches, expectedError)
}

type mockAPIConnection struct {
	api.Connection
	bestFacadeVersion int
	serverVersion     *version.Number
}

func (m *mockAPIConnection) Addr() string {
	return "0.1.2.3:1234"
}

func (m *mockAPIConnection) IPAddr() string {
	return "0.1.2.3:1234"
}

func (m *mockAPIConnection) AuthTag() names.Tag {
	return names.NewUserTag("testuser")
}

func (m *mockAPIConnection) PublicDNSName() string {
	return ""
}

func (m *mockAPIConnection) APIHostPorts() []network.MachineHostPorts {
	hp, _ := network.ParseMachineHostPort(m.Addr())
	return []network.MachineHostPorts{{*hp}}
}

func (m *mockAPIConnection) BestFacadeVersion(name string) int {
	return m.bestFacadeVersion
}

func (m *mockAPIConnection) ServerVersion() (version.Number, bool) {
	if m.serverVersion != nil {
		return *m.serverVersion, true
	}
	return version.Number{}, false
}

func (*mockAPIConnection) Close() error {
	return nil
}

type mockCharmAdder struct {
	CharmAdder
	testing.Stub
}

func (m *mockCharmAdder) AddCharm(curl *charm.URL, channel csclientparams.Channel, force bool) error {
	m.MethodCall(m, "AddCharm", curl, channel, force)
	return m.NextErr()
}

type mockCharmClient struct {
	CharmClient
	testing.Stub
	charmInfo *charms.CharmInfo
}

func (m *mockCharmClient) CharmInfo(curl string) (*charms.CharmInfo, error) {
	m.MethodCall(m, "CharmInfo", curl)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.charmInfo, nil
}

type mockCharmAPIClient struct {
	CharmAPIClient
	testing.Stub
	charmURL *charm.URL
}

func (m *mockCharmAPIClient) GetCharmURL(branchName, appName string) (*charm.URL, error) {
	m.MethodCall(m, "GetCharmURL", branchName, appName)
	return m.charmURL, m.NextErr()
}

func (m *mockCharmAPIClient) SetCharm(branchName string, cfg application.SetCharmConfig) error {
	m.MethodCall(m, "SetCharm", branchName, cfg)
	return m.NextErr()
}

func (m *mockCharmAPIClient) Get(branchName, applicationName string) (*params.ApplicationGetResults, error) {
	m.MethodCall(m, "Get", applicationName)
	return &params.ApplicationGetResults{}, m.NextErr()
}

type mockModelConfigGetter struct {
	ModelConfigGetter
	testing.Stub
}

func (m *mockModelConfigGetter) ModelGet() (map[string]interface{}, error) {
	m.MethodCall(m, "ModelGet")
	return coretesting.FakeConfig(), m.NextErr()
}

type mockResourceLister struct {
	ResourceLister
	testing.Stub
}
