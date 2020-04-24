// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	csclientparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	jujucharmstore "github.com/juju/juju/charmstore"
	"github.com/juju/juju/cmd/juju/common"
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

type BaseUpgradeCharmSuite struct {
	testing.IsolationSuite
	testing.Stub

	deployResources   resourceadapters.DeployResourcesFunc
	fakeAPI           *fakeDeployAPI
	resolveCharm      ResolveCharmFunc
	resolvedCharmURL  *charm.URL
	resolvedChannel   csclientparams.Channel
	apiConnection     mockAPIConnection
	charmAdder        mockCharmAdder
	charmClient       mockCharmClient
	charmAPIClient    mockCharmAPIClient
	modelConfigGetter mockModelConfigGetter
	resourceLister    mockResourceLister
	spacesClient      mockSpacesClient
}

func (s *BaseUpgradeCharmSuite) runUpgradeCharm(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.upgradeCommand(), args...)
}

type UpgradeCharmSuite struct {
	BaseUpgradeCharmSuite
}

var _ = gc.Suite(&UpgradeCharmSuite{})

func (s *BaseUpgradeCharmSuite) SetUpTest(c *gc.C) {
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

	s.resolvedChannel = csclientparams.StableChannel
	s.resolveCharm = func(
		resolveWithChannel func(*charm.URL, csclientparams.Channel) (*charm.URL, csclientparams.Channel, []string, error),
		url *charm.URL,
		preferredChannel csclientparams.Channel,
	) (*charm.URL, csclientparams.Channel, []string, error) {
		s.AddCall("ResolveCharm", url, preferredChannel)
		if err := s.NextErr(); err != nil {
			return nil, csclientparams.NoChannel, nil, err
		}

		if s.resolvedChannel != "" {
			preferredChannel = s.resolvedChannel
		}
		return s.resolvedCharmURL, s.resolvedChannel, []string{"quantal"}, nil
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
	s.charmAPIClient = mockCharmAPIClient{
		charmURL: currentCharmURL,
		bindings: map[string]string{
			"": network.AlphaSpaceName,
		},
	}
	s.modelConfigGetter = newMockModelConfigGetter()
	s.resourceLister = mockResourceLister{}
	s.spacesClient = mockSpacesClient{
		spaceList: []params.Space{
			{Id: network.AlphaSpaceId, Name: network.AlphaSpaceName}, // default
			{Id: "1", Name: "sp1"},
		},
	}
}

func (s *BaseUpgradeCharmSuite) upgradeCommand() cmd.Command {
	store := jujuclient.NewMemStore()
	store.CurrentControllerName = "foo"
	store.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	store.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models: map[string]jujuclient.ModelDetails{
			"admin/bar": {ActiveBranch: model.GenerationMaster},
		},
	}
	store.Accounts["foo"] = jujuclient.AccountDetails{
		User: "admin", Password: "hunter2",
	}
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.AddCall("OpenAPI")
		return &s.apiConnection, nil
	}

	cmd := NewUpgradeCharmCommandForTest(
		store,
		apiOpen,
		s.deployResources,
		s.resolveCharm,
		func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csclientparams.Channel,
		) charmrepoForDeploy {
			s.AddCall("NewCharmStore", csURL)
			return s.fakeAPI
		},
		func(conn api.Connection) CharmAdder {
			s.AddCall("NewCharmAdder", conn)
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
		func(conn base.APICallCloser) (ResourceLister, error) {
			s.AddCall("NewResourceLister", conn)
			return &s.resourceLister, s.NextErr()
		},
		func(conn base.APICallCloser) (string, error) {
			s.AddCall("CharmStoreURLGetter", conn)
			return "testing.api.charmstore", s.NextErr()
		},
		func(conn base.APICallCloser) SpacesAPI {
			s.AddCall("NewSpacesClient", conn)
			return &s.spacesClient
		},
	)
	return cmd
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
		if call.FuncName == "NewCharmStore" {
			csURL = call.Args[0].(string)
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

func (s *UpgradeCharmSuite) TestUpgradeWithBindDefaults(c *gc.C) {
	s.charmAPIClient.bindings = map[string]string{
		"": "testing",
	}

	s.testUpgradeWithBind(c, map[string]string{
		"ep1": "sp1",
		"ep2": "testing",
	})
}

func (s *UpgradeCharmSuite) testUpgradeWithBind(c *gc.C, expectedBindings map[string]string) {
	s.apiConnection = mockAPIConnection{
		bestFacadeVersion: 11,
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}

	s.charmClient.charmInfo.Meta.ExtraBindings = map[string]charm.ExtraBinding{
		"ep1": {Name: "ep1"},
		"ep2": {Name: "ep2"},
	}

	_, err := s.runUpgradeCharm(c, "foo", "--bind", "ep1=sp1")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")
	s.spacesClient.CheckCallNames(c, "ListSpaces")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.StableChannel,
		},
		EndpointBindings: expectedBindings,
	})
}

func (s *UpgradeCharmSuite) TestUpgradeWithBindAndUnknownEndpoint(c *gc.C) {
	s.apiConnection = mockAPIConnection{
		bestFacadeVersion: 11,
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}

	s.charmClient.charmInfo.Meta.ExtraBindings = map[string]charm.ExtraBinding{
		"ep1": {Name: "ep1"},
	}

	_, err := s.runUpgradeCharm(c, "foo", "--bind", "unknown=sp1")
	c.Assert(err, gc.ErrorMatches, `endpoint "unknown" not found`)
}

type UpgradeCharmErrorsStateSuite struct {
	jujutesting.RepoSuite

	fakeAPI *fakeDeployAPI
	cmd     cmd.Command
}

var _ = gc.Suite(&UpgradeCharmErrorsStateSuite{})

func (s *UpgradeCharmErrorsStateSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	cfgAttrs := map[string]interface{}{
		"name": "name",
		"uuid": coretesting.ModelTag.Id(),
		"type": "foo",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfgAttrs)
	s.cmd = NewUpgradeCharmCommandForStateTest(
		func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csclientparams.Channel,
		) charmrepoForDeploy {
			return s.fakeAPI
		},
		func(conn api.Connection) CharmAdder {
			return s.fakeAPI
		},
		func(conn base.APICallCloser) CharmClient {
			return s.fakeAPI
		},
		resourceadapters.DeployResources,
		nil,
	)
}

func (s *UpgradeCharmErrorsStateSuite) runUpgradeCharm(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidArgs(c *gc.C) {
	_, err := s.runUpgradeCharm(c, s.cmd)
	c.Assert(err, gc.ErrorMatches, "no application specified")
	_, err = s.runUpgradeCharm(c, s.cmd, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	_, err = s.runUpgradeCharm(c, s.cmd, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidApplication(c *gc.C) {
	_, err := s.runUpgradeCharm(c, s.cmd, "phony")
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: `application "phony" not found`,
		Code:    "not found",
	})
}

func (s *UpgradeCharmErrorsStateSuite) deployApplication(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "riak")
	curl := charm.MustParseURL("local:bionic/riak-7")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, "bionic", charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := runDeploy(c, charmDir.Path, "riak", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidSwitchURL(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--switch=missing")
	c.Assert(err, gc.ErrorMatches, `cannot resolve URL "cs:missing": charm or bundle not found`)
}

func (s *UpgradeCharmErrorsStateSuite) TestNoPathFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak")
	c.Assert(err, gc.ErrorMatches, "upgrading a local charm requires either --path or --switch")
}

func (s *UpgradeCharmErrorsStateSuite) TestSwitchAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestPathAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--path=foo", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--path and --revision are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestSwitchAndPathFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--switch=riak", "--path=foo")
	c.Assert(err, gc.ErrorMatches, "--switch and --path are mutually exclusive")
}

func (s *UpgradeCharmErrorsStateSuite) TestInvalidRevision(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--revision=blah")
	c.Assert(err, gc.ErrorMatches, `invalid value "blah" for option --revision: strconv.(ParseInt|Atoi): parsing "blah": invalid syntax`)
}

type UpgradeCharmSuccessStateSuite struct {
	jujutesting.RepoSuite
	coretesting.CmdBlockHelper
	path string
	riak *state.Application

	fakeAPI     *fakeDeployAPI
	charmClient mockCharmClient
	cmd         cmd.Command
}

var _ = gc.Suite(&UpgradeCharmSuccessStateSuite{})

func (s *UpgradeCharmSuccessStateSuite) assertUpgraded(c *gc.C, riak *state.Application, revision int, forced bool) *charm.URL {
	err := riak.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, revision)
	c.Assert(force, gc.Equals, forced)
	return ch.URL()
}

func (s *UpgradeCharmSuccessStateSuite) runUpgradeCharm(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *UpgradeCharmSuccessStateSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	s.charmClient = mockCharmClient{}
	s.cmd = NewUpgradeCharmCommandForStateTest(
		func(
			bakeryClient *httpbakery.Client,
			csURL string,
			channel csclientparams.Channel,
		) charmrepoForDeploy {
			return s.fakeAPI
		},
		func(conn api.Connection) CharmAdder {
			return &apiClient{Client: conn.Client()}
		},
		func(conn base.APICallCloser) CharmClient {
			return &s.charmClient
		},
		resourceadapters.DeployResources,
		nil,
	)
	s.path = testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	err := runDeploy(c, s.path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	curl := charm.MustParseURL("local:bionic/riak-7")
	s.riak, _ = s.RepoSuite.AssertApplication(c, "riak", curl, 1, 1)

	_, forced, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(forced, jc.IsFalse)

	s.charmClient.charmInfo = &charms.CharmInfo{
		URL:  "local:riak",
		Meta: &charm.Meta{},
	}

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
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	s.AssertCharmUploaded(c, curl)
	// Even though the remote revision is bumped, the local one should
	// be unchanged.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuite) TestUpgradeWithChannel(c *gc.C) {
	s.resolvedChannel = csclientparams.BetaChannel
	_, err := s.runUpgradeCharm(c, "foo", "--channel=beta")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, csclientparams.BetaChannel, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.BetaChannel,
		},
	})
}

func (s *UpgradeCharmSuite) TestUpgradeCharmShouldRespectDeployedChannelByDefault(c *gc.C) {
	s.resolvedChannel = csclientparams.BetaChannel
	_, err := s.runUpgradeCharm(c, "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, csclientparams.BetaChannel, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.BetaChannel,
		},
	})
}

func (s *UpgradeCharmSuite) TestSwitch(c *gc.C) {
	_, err := s.runUpgradeCharm(c, "foo", "--switch=cs:~other/trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)

	s.charmClient.CheckCallNames(c, "CharmInfo")
	s.charmClient.CheckCall(c, 0, "CharmInfo", s.resolvedCharmURL.String())
	s.charmAdder.CheckCallNames(c, "AddCharm")
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, csclientparams.StableChannel, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURL", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: jujucharmstore.CharmID{
			URL:     s.resolvedCharmURL,
			Channel: csclientparams.StableChannel,
		},
	})
	var curl *charm.URL
	for _, call := range s.Calls() {
		if call.FuncName == "ResolveCharm" {
			curl = call.Args[0].(*charm.URL)
			break
		}
	}
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, "cs:~other/trusty/anotherriak")
}

func (s *UpgradeCharmSuite) TestSwitchSameURL(c *gc.C) {
	s.charmAPIClient.charmURL = s.resolvedCharmURL
	_, err := s.runUpgradeCharm(c, "foo", "--switch="+s.resolvedCharmURL.String())
	c.Assert(err, gc.ErrorMatches, `already running specified charm "cs:quantal/foo-2"`)
}

func (s *UpgradeCharmSuite) TestSwitchDifferentRevision(c *gc.C) {
	curlCopy := *s.resolvedCharmURL
	s.charmAPIClient.charmURL = &curlCopy
	s.resolvedCharmURL.Revision++
	_, err := s.runUpgradeCharm(c, "riak", "--switch="+s.resolvedCharmURL.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeCharmSuite) TestUpgradeWithTermsNotSigned(c *gc.C) {
	termsRequiredError := &common.TermsRequiredError{Terms: []string{"term/1", "term/2"}}
	s.charmAdder.SetErrors(termsRequiredError)
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	_, err := s.runUpgradeCharm(c, "terms1")
	c.Assert(err, gc.ErrorMatches, expectedError)
}
func (s *UpgradeCharmSuccessStateSuite) TestBlockUpgradeCharm(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockUpgradeCharm")
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--path", s.path)
	s.AssertBlocked(c, err, ".*TestBlockUpgradeCharm.*")
}

func (s *UpgradeCharmSuccessStateSuite) TestRespectsLocalRevisionWhenPossible(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)

	s.charmClient.charmInfo = &charms.CharmInfo{
		URL:      "local:riak",
		Meta:     dir.Meta(),
		Revision: dir.Revision(),
	}
	_, err = s.runUpgradeCharm(c, s.cmd, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 42, false)
	s.AssertCharmUploaded(c, curl)
	s.assertLocalRevision(c, 42, s.path)
}

func (s *UpgradeCharmSuccessStateSuite) TestForcedSeriesUpgrade(c *gc.C) {
	repoPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
	err := runDeploy(c, repoPath, "multi-series", "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("multi-series")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)
	unit := units[0]
	tags := []names.UnitTag{unit.UnitTag()}
	errs, err := s.APIState.UnitAssigner().AssignUnits(tags)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errs, gc.DeepEquals, make([]error, len(units)))

	// Overwrite the metadata.yaml to change the supported series.
	metadataPath := filepath.Join(repoPath, "metadata.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metadata.yaml for overwriting"))
	}
	defer func() { _ = file.Close() }()

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

	s.charmClient.charmInfo = &charms.CharmInfo{
		URL:      ch.URL().String(),
		Meta:     ch.Meta(),
		Revision: ch.Revision(),
	}
	_, err = s.runUpgradeCharm(c, s.cmd, "multi-series", "--path", repoPath, "--force-series")
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch.Revision(), gc.Equals, 2)
	c.Check(force, gc.Equals, false)
}

func (s *UpgradeCharmSuccessStateSuite) TestForcedLXDProfileUpgrade(c *gc.C) {
	repoPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "lxd-profile-alt")
	err := runDeploy(c, repoPath, "lxd-profile-alt", "--to", "lxd")
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("lxd-profile-alt")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, 0)

	units, err := app.AllUnits()
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
	lxdProfilePath := filepath.Join(repoPath, "lxd-profile.yaml")
	file, err := os.OpenFile(lxdProfilePath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open lxd-profile.yaml for overwriting"))
	}
	defer func() { _ = file.Close() }()

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

	_, err = s.runUpgradeCharm(c, s.cmd, "lxd-profile-alt", "--path", repoPath)
	c.Assert(err, gc.ErrorMatches, `invalid lxd-profile.yaml: contains config value "boot.autostart.delay"`)
}

func (s *UpgradeCharmSuccessStateSuite) TestInitWithResources(c *gc.C) {
	testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "dummy")
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
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *UpgradeCharmSuccessStateSuite) TestBlockForcedUnitsUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--force-units", "--path", s.path)
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
	_, err = s.runUpgradeCharm(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 42, false)
	c.Assert(curl.String(), gc.Equals, "local:bionic/riak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}

func (s *UpgradeCharmSuccessStateSuite) TestCharmPathNoRevUpgrade(c *gc.C) {
	// Revision 7 is running to start with.
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	s.assertLocalRevision(c, 7, myriakPath)
	_, err := s.runUpgradeCharm(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl := s.assertUpgraded(c, s.riak, 8, false)
	c.Assert(curl.String(), gc.Equals, "local:bionic/riak-8")
}

func (s *UpgradeCharmSuccessStateSuite) TestCharmPathDifferentNameFails(c *gc.C) {
	myriakPath := testcharms.RepoWithSeries("bionic").RenamedClonedDirPath(c.MkDir(), "riak", "myriak")
	metadataPath := filepath.Join(myriakPath, "metadata.yaml")
	file, err := os.OpenFile(metadataPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open metadata.yaml"))
	}
	defer func() { _ = file.Close() }()

	// Overwrite the metadata.yaml to contain a new name.
	newMetadata := strings.Join([]string{`name: myriak`, `summary: ""`, `description: ""`}, "\n")
	if _, err := file.WriteString(newMetadata); err != nil {
		c.Fatal("cannot write to metadata.yaml")
	}
	_, err = s.runUpgradeCharm(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, `cannot upgrade "riak" to "myriak"`)
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

func (*mockAPIConnection) ControllerAccess() string {
	return "admin"
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

func (m *mockCharmAdder) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	m.MethodCall(m, "AddLocalCharm", curl, ch, force)
	return curl, m.NextErr()
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

	bindings map[string]string
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
	return &params.ApplicationGetResults{
		EndpointBindings: m.bindings,
	}, m.NextErr()
}

func newMockModelConfigGetter() mockModelConfigGetter {
	return mockModelConfigGetter{cfg: coretesting.FakeConfig()}
}

type mockModelConfigGetter struct {
	ModelConfigGetter
	testing.Stub

	cfg map[string]interface{}
}

func (m *mockModelConfigGetter) ModelGet() (map[string]interface{}, error) {
	m.MethodCall(m, "ModelGet")
	return m.cfg, m.NextErr()
}

func (m *mockModelConfigGetter) SetDefaultSpace(name string) {
	m.cfg["default-space"] = name
}

type mockResourceLister struct {
	ResourceLister
	testing.Stub
}

type mockSpacesClient struct {
	SpacesAPI
	testing.Stub

	spaceList []params.Space
}

func (m *mockSpacesClient) ListSpaces() ([]params.Space, error) {
	m.MethodCall(m, "ListSpaces")
	return m.spaceList, m.NextErr()
}
