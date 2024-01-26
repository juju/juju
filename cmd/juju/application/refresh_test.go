// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/unitassigner"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreresouces "github.com/juju/juju/core/resources"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
)

type BaseRefreshSuite struct {
	testing.IsolationSuite
	testing.Stub

	deployResources      deployer.DeployResourcesFunc
	resolveCharm         mockCharmResolver
	resolvedCharmURL     *charm.URL
	resolvedChannel      charm.Risk
	apiConnection        mockAPIConnection
	charmAdder           mockCharmAdder
	charmClient          mockCharmClient
	charmAPIClient       mockCharmRefreshClient
	modelConfigGetter    mockModelConfigGetter
	resourceLister       mockResourceLister
	spacesClient         mockSpacesClient
	downloadBundleClient mockDownloadBundleClient

	testPlatform corecharm.Platform
	testBase     corebase.Base
}

func (s *BaseRefreshSuite) runRefresh(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.refreshCommand(), args...)
}

type RefreshSuite struct {
	BaseRefreshSuite
}

var _ = gc.Suite(&RefreshSuite{})

func (s *RefreshSuite) SetUpTest(c *gc.C) {
	s.BaseRefreshSuite.SetUpSuite(c)
	s.BaseRefreshSuite.setup(c, charm.MustParseURL("ch:quantal/foo-1"), charm.MustParseURL("ch:quantal/foo-2"))
}

func (s *BaseRefreshSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()
}

func (s *BaseRefreshSuite) setup(c *gc.C, currentCharmURL, latestCharmURL *charm.URL) {
	// Create persistent cookies in a temporary location.
	cookieFile := filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookieFile)

	var err error
	s.testBase, err = corebase.GetBaseFromSeries(currentCharmURL.Series)
	c.Assert(err, jc.ErrorIsNil)
	s.testPlatform = corecharm.MustParsePlatform(fmt.Sprintf("%s/%s/%s", arch.DefaultArchitecture, s.testBase.OS, s.testBase.Channel))

	s.deployResources = func(
		applicationID string,
		chID resources.CharmID,
		filesAndRevisions map[string]string,
		resources map[string]charmresource.Meta,
		conn base.APICallCloser,
		filesystem modelcmd.Filesystem,
	) (ids map[string]string, err error) {
		s.AddCall("DeployResources", applicationID, chID, filesAndRevisions, resources)
		ids = make(map[string]string)
		for _, r := range resources {
			ids[r.Name] = r.Name + "Id"
		}
		return ids, s.NextErr()
	}

	s.resolvedChannel = charm.Stable
	s.resolveCharm = mockCharmResolver{
		resolveFunc: func(url *charm.URL, preferredOrigin commoncharm.Origin, _ bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error) {
			s.AddCall("ResolveCharm", url, preferredOrigin)
			if err := s.NextErr(); err != nil {
				return nil, commoncharm.Origin{}, nil, err
			}

			if s.resolvedChannel != "" {
				preferredOrigin.Risk = string(s.resolvedChannel)
			}
			return s.resolvedCharmURL, preferredOrigin, []corebase.Base{corebase.MustParseBaseFromString("ubuntu@12.10")}, nil
		},
	}

	s.resolvedCharmURL = latestCharmURL

	s.apiConnection = mockAPIConnection{
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}
	s.charmAdder = mockCharmAdder{}
	s.charmClient = mockCharmClient{
		charmInfo: &apicommoncharms.CharmInfo{
			Meta: &charm.Meta{},
		},
	}
	risk := "stable"
	if charm.Local.Matches(currentCharmURL.Schema) {
		// A local charm must never have a track, risk,
		// nor branch.
		risk = ""
	}
	s.charmAPIClient = mockCharmRefreshClient{
		charmURL: currentCharmURL,
		bindings: map[string]string{
			"": network.AlphaSpaceName,
		},
		charmOrigin: commoncharm.Origin{
			ID:           "testing",
			Source:       schemaToOriginScource(currentCharmURL.Schema),
			Risk:         risk,
			Architecture: arch.DefaultArchitecture,
			Base:         s.testBase,
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
	s.downloadBundleClient = mockDownloadBundleClient{
		bundle: nil,
	}
}

func schemaToOriginScource(schema string) commoncharm.OriginSource {
	switch {
	case charm.Local.Matches(schema):
		return commoncharm.OriginLocal
	}
	return commoncharm.OriginCharmHub
}

func (s *BaseRefreshSuite) refreshCommand() cmd.Command {
	memStore := jujuclient.NewMemStore()
	memStore.CurrentControllerName = "foo"
	memStore.Controllers["foo"] = jujuclient.ControllerDetails{
		APIEndpoints: []string{"0.1.2.3:1234"},
	}
	memStore.Models["foo"] = &jujuclient.ControllerModels{
		CurrentModel: "admin/bar",
		Models: map[string]jujuclient.ModelDetails{
			"admin/bar": {ActiveBranch: model.GenerationMaster},
		},
	}
	memStore.Accounts["foo"] = jujuclient.AccountDetails{
		User: "admin", Password: "hunter2",
	}
	apiOpen := func(*api.Info, api.DialOpts) (api.Connection, error) {
		s.AddCall("OpenAPI")
		return &s.apiConnection, nil
	}

	cmd := NewRefreshCommandForTest(
		memStore,
		apiOpen,
		s.deployResources,
		func(base.APICallCloser, store.DownloadBundleClient) CharmResolver {
			s.AddCall("NewCharmResolver")
			return &s.resolveCharm
		},
		func(conn api.Connection) (store.CharmAdder, error) {
			s.AddCall("NewCharmAdder", conn)
			s.PopNoErr()
			return &s.charmAdder, nil
		},
		func(conn base.APICallCloser) utils.CharmClient {
			s.AddCall("NewCharmClient", conn)
			s.PopNoErr()
			return &s.charmClient
		},
		func(conn base.APICallCloser) CharmRefreshClient {
			s.AddCall("NewCharmAPIClient", conn)
			s.PopNoErr()
			return &s.charmAPIClient
		},
		func(conn base.APICallCloser) (utils.ResourceLister, error) {
			s.AddCall("NewResourceLister", conn)
			return &s.resourceLister, s.NextErr()
		},
		func(conn base.APICallCloser) SpacesAPI {
			s.AddCall("NewSpacesClient", conn)
			return &s.spacesClient
		},
		func(conn base.APICallCloser) ModelConfigClient {
			s.AddCall("ModelConfigClient", conn)
			return &s.modelConfigGetter
		},
		func(curl string) (store.DownloadBundleClient, error) {
			s.AddCall("NewCharmHubClient", curl)
			return &s.downloadBundleClient, nil
		},
	)
	return cmd
}

func (s *RefreshSuite) TestStorageConstraints(c *gc.C) {
	_, err := s.runRefresh(c, "foo", "--storage", "bar=baz")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		StorageConstraints: map[string]storage.Constraints{
			"bar": {Pool: "baz", Count: 1},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestConfigSettings(c *gc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runRefresh(c, "foo", "--config", configFile)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettingsYAML: "foo:{}",
		ConfigSettings:     map[string]string{},
		EndpointBindings:   map[string]string{},
	})
}

func (s *RefreshSuite) TestConfigSettingsWithTrust(c *gc.C) {
	_, err := s.runRefresh(c, "foo", "--trust", "--config", "foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{"trust": "true", "foo": "bar"},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestConfigSettingsWithTrustFalse(c *gc.C) {
	_, err := s.runRefresh(c, "foo", "--trust=false", "--config", "foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{"trust": "false", "foo": "bar"},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestConfigSettingsWithKeyValuesAndFile(c *gc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runRefresh(c, "foo", "--trust", "--config", "foo=bar", "--config", configFile)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettingsYAML: "foo:{}",
		ConfigSettings:     map[string]string{"trust": "true", "foo": "bar"},
		EndpointBindings:   map[string]string{},
	})
}

func (s *RefreshSuite) TestUpgradeWithBindDefaults(c *gc.C) {
	s.charmAPIClient.bindings = map[string]string{
		"": "testing",
	}

	s.testUpgradeWithBind(c, map[string]string{
		"ep1": "sp1",
		"ep2": "testing",
	})
}

func (s *RefreshSuite) testUpgradeWithBind(c *gc.C, expectedBindings map[string]string) {
	s.apiConnection = mockAPIConnection{
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

	_, err := s.runRefresh(c, "foo", "--bind", "ep1=sp1")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.spacesClient.CheckCallNames(c, "ListSpaces")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: expectedBindings,
	})
}

func (s *RefreshSuite) TestUpgradeWithBindAndUnknownEndpoint(c *gc.C) {
	s.apiConnection = mockAPIConnection{
		serverVersion: &version.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}

	s.charmClient.charmInfo.Meta.ExtraBindings = map[string]charm.ExtraBinding{
		"ep1": {Name: "ep1"},
	}

	_, err := s.runRefresh(c, "foo", "--bind", "unknown=sp1")
	c.Assert(err, gc.ErrorMatches, `endpoint "unknown" not found`)
}

type RefreshErrorsStateSuite struct {
	jujutesting.RepoSuite

	fakeAPI *fakeDeployAPI
	cmd     cmd.Command
}

var _ = gc.Suite(&RefreshErrorsStateSuite{})

func (s *RefreshErrorsStateSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpSuite(c)
	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04"}, corebase.ParseBaseFromString)
	})
}

func (s *RefreshErrorsStateSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	cfgAttrs := map[string]interface{}{
		"name":           "name",
		"uuid":           coretesting.ModelTag.Id(),
		"type":           "foo",
		"secret-backend": "auto",
	}
	s.fakeAPI = vanillaFakeModelAPI(cfgAttrs)
	s.cmd = NewRefreshCommandForStateTest(
		func(conn api.Connection) (store.CharmAdder, error) {
			return s.fakeAPI, nil
		},
		func(conn base.APICallCloser) utils.CharmClient {
			return s.fakeAPI
		},
		deployer.DeployResources,
		nil,
	)
}

func (s *RefreshErrorsStateSuite) runRefresh(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *RefreshErrorsStateSuite) TestInvalidArgs(c *gc.C) {
	_, err := s.runRefresh(c, s.cmd)
	c.Assert(err, gc.ErrorMatches, "no application specified")
	_, err = s.runRefresh(c, s.cmd, "invalid:name")
	c.Assert(err, gc.ErrorMatches, `invalid application name "invalid:name"`)
	_, err = s.runRefresh(c, s.cmd, "foo", "bar")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *RefreshErrorsStateSuite) TestInvalidApplication(c *gc.C) {
	_, err := s.runRefresh(c, s.cmd, "phony")
	c.Assert(errors.Cause(err), gc.ErrorMatches, `application "phony" not found`)
}

func (s *RefreshErrorsStateSuite) deployApplication(c *gc.C) {
	charmDir := testcharms.RepoWithSeries("bionic").ClonedDir(c.MkDir(), "riak")
	curl := charm.MustParseURL("local:bionic/riak-7")
	withLocalCharmDeployable(s.fakeAPI, curl, charmDir, false)
	withCharmDeployable(s.fakeAPI, curl, corebase.MustParseBaseFromString("ubuntu@18.04"), charmDir.Meta(), charmDir.Metrics(), false, false, 1, nil, nil)

	err := runDeploy(c, charmDir.Path, "riak", "--base", "ubuntu@18.04")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshErrorsStateSuite) TestNoPathFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runRefresh(c, s.cmd, "riak")
	c.Assert(err, gc.ErrorMatches, "refreshing a local charm requires either --path or --switch")
}

func (s *RefreshErrorsStateSuite) TestSwitchAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runRefresh(c, s.cmd, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *RefreshErrorsStateSuite) TestPathAndRevisionFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runRefresh(c, s.cmd, "riak", "--path=foo", "--revision=2")
	c.Assert(err, gc.ErrorMatches, "--path and --revision are mutually exclusive")
}

func (s *RefreshErrorsStateSuite) TestSwitchAndPathFails(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runRefresh(c, s.cmd, "riak", "--switch=riak", "--path=foo")
	c.Assert(err, gc.ErrorMatches, "--switch and --path are mutually exclusive")
}

func (s *RefreshErrorsStateSuite) TestInvalidRevision(c *gc.C) {
	s.deployApplication(c)
	_, err := s.runRefresh(c, s.cmd, "riak", "--revision=blah")
	c.Assert(err, gc.ErrorMatches, `invalid value "blah" for option --revision: strconv.(ParseInt|Atoi): parsing "blah": invalid syntax`)
}

type RefreshSuccessStateSuite struct {
	jujutesting.RepoSuite
	coretesting.CmdBlockHelper
	path string
	riak *state.Application

	charmClient mockCharmClient
	cmd         cmd.Command
}

var _ = gc.Suite(&RefreshSuccessStateSuite{})

func (s *RefreshSuccessStateSuite) SetUpSuite(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpSuite(c)
	// TODO remove this patch once we removed all the old bases from tests in current package.
	s.PatchValue(&deployer.SupportedJujuBases, func(time.Time, corebase.Base, string) ([]corebase.Base, error) {
		return transform.SliceOrErr([]string{"ubuntu@22.04", "ubuntu@20.04", "ubuntu@18.04"}, corebase.ParseBaseFromString)
	})

}

func (s *RefreshSuccessStateSuite) assertUpgraded(c *gc.C, riak *state.Application, revision int, forced bool) (string, *state.CharmOrigin) {
	err := riak.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Revision(), gc.Equals, revision)
	c.Assert(force, gc.Equals, forced)
	return ch.URL(), riak.CharmOrigin()
}

func (s *RefreshSuccessStateSuite) runRefresh(c *gc.C, cmd cmd.Command, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, cmd, args...)
}

func (s *RefreshSuccessStateSuite) SetUpTest(c *gc.C) {
	if runtime.GOOS == "darwin" {
		c.Skip("Mongo failures on macOS")
	}
	s.RepoSuite.SetUpTest(c)

	s.charmClient = mockCharmClient{}
	s.cmd = NewRefreshCommandForStateTest(
		newCharmAdder,
		func(conn base.APICallCloser) utils.CharmClient {
			return &s.charmClient
		},
		deployer.DeployResources,
		nil,
	)
	s.path = testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	err := runDeploy(c, s.path, "--series", "bionic")
	c.Assert(err, jc.ErrorIsNil)
	curl := "local:bionic/riak-7"
	s.riak, _ = s.RepoSuite.AssertApplication(c, "riak", curl, 1, 1)

	_, forced, err := s.riak.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(forced, jc.IsFalse)

	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL:  "local:riak",
		Meta: &charm.Meta{},
	}

	s.CmdBlockHelper = coretesting.NewCmdBlockHelper(s.APIState)
	c.Assert(s.CmdBlockHelper, gc.NotNil)
	s.AddCleanup(func(*gc.C) { s.CmdBlockHelper.Close() })
}

func (s *RefreshSuccessStateSuite) assertLocalRevision(c *gc.C, revision int, path string) {
	dir, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir.Revision(), gc.Equals, revision)
}

func (s *RefreshSuccessStateSuite) TestLocalRevisionUnchanged(c *gc.C) {
	_, err := s.runRefresh(c, s.cmd, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 8, false)
	s.AssertCharmUploaded(c, curl)
	// Even though the remote revision is bumped, the local one should
	// be unchanged.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *RefreshSuite) TestUpgradeWithChannel(c *gc.C) {
	s.resolvedChannel = charm.Beta
	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	origin, _ := utils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Beta}, corecharm.Platform{
		Architecture: arch.DefaultArchitecture,
	})
	origin.ID = "testing"
	origin.Revision = (*int)(nil)
	origin.Architecture = arch.DefaultArchitecture
	origin.Base = s.testBase
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "beta",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestUpgradeWithChannelNoNewCharmURL(c *gc.C) {
	// Test setting a new charm channel, without an actual
	// charm upgrade needed.
	s.resolvedChannel = charm.Beta
	s.resolvedCharmURL = charm.MustParseURL("ch:quantal/foo-1")

	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "beta",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestRefreshShouldRespectDeployedChannelByDefault(c *gc.C) {
	s.resolvedChannel = charm.Beta
	_, err := s.runRefresh(c, "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	origin, _ := utils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Beta}, s.testPlatform)
	origin.ID = "testing"
	origin.Revision = (*int)(nil)
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "beta",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestUpgradeFailWithoutCharmHubOriginID(c *gc.C) {
	s.resolvedChannel = charm.Beta
	s.charmAPIClient.charmOrigin.Source = "charm-hub"
	s.charmAPIClient.charmOrigin.ID = ""
	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, gc.ErrorMatches, "\"foo\" deploy incomplete, please try refresh again in a little bit.")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin")
}

func (s *RefreshSuite) TestSwitch(c *gc.C) {
	_, err := s.runRefresh(c, "foo", "--switch=ch:trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)

	s.charmClient.CheckCallNames(c, "CharmInfo", "CharmInfo")
	s.charmClient.CheckCall(c, 0, "CharmInfo", s.resolvedCharmURL.String())
	s.charmAdder.CheckCallNames(c, "CheckCharmPlacement", "AddCharm")
	origin, _ := utils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Stable}, s.testPlatform)

	parsedSwitchUrl, err := charm.ParseURL("ch:trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCall(c, 0, "CheckCharmPlacement", "foo", parsedSwitchUrl)
	origin.Revision = (*int)(nil)
	s.charmAdder.CheckCall(c, 1, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				Source:       "charm-hub",
				Architecture: arch.DefaultArchitecture,
				Risk:         "stable",
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
	var curl *charm.URL
	for _, call := range s.Calls() {
		if call.FuncName == "ResolveCharm" {
			curl = call.Args[0].(*charm.URL)
			break
		}
	}
	c.Assert(curl, gc.NotNil)
	c.Assert(curl.String(), gc.Equals, "ch:trusty/anotherriak")
}

func (s *RefreshSuite) TestSwitchSameURL(c *gc.C) {
	s.charmAPIClient.charmURL = s.resolvedCharmURL
	_, err := s.runRefresh(c, "foo", "--switch="+s.resolvedCharmURL.String())
	// Should not get error since charm already up-to-date
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshSuite) TestSwitchDifferentRevision(c *gc.C) {
	curlCopy := *s.resolvedCharmURL
	s.charmAPIClient.charmURL = &curlCopy
	s.resolvedCharmURL.Revision++
	_, err := s.runRefresh(c, "riak", "--switch="+s.resolvedCharmURL.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshSuite) TestUpgradeWithTermsNotSigned(c *gc.C) {
	termsRequiredError := &common.TermsRequiredError{Terms: []string{"term/1", "term/2"}}
	s.charmAdder.SetErrors(termsRequiredError)
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	_, err := s.runRefresh(c, "terms1")
	c.Assert(err, gc.ErrorMatches, expectedError)
}
func (s *RefreshSuccessStateSuite) TestBlockRefresh(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockRefresh")
	_, err := s.runRefresh(c, s.cmd, "riak", "--path", s.path)
	s.AssertBlocked(c, err, ".*TestBlockRefresh.*")
}

func (s *RefreshSuccessStateSuite) TestRespectsLocalRevisionWhenPossible(c *gc.C) {
	dir, err := charm.ReadCharmDir(s.path)
	c.Assert(err, jc.ErrorIsNil)
	err = dir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)

	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL:      "local:riak",
		Meta:     dir.Meta(),
		Revision: dir.Revision(),
	}
	_, err = s.runRefresh(c, s.cmd, "riak", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 42, false)
	s.AssertCharmUploaded(c, curl)
	s.assertLocalRevision(c, 42, s.path)
}

func (s *RefreshSuccessStateSuite) TestForcedSeriesUpgrade(c *gc.C) {
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
	errs, err := unitassigner.New(s.APIState).AssignUnits(tags)
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
		},
		"\n",
	)
	if _, err := file.WriteString(metadata); err != nil {
		c.Fatal(errors.Annotate(err, "cannot write to metadata.yaml"))
	}

	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL:      ch.URL(),
		Meta:     ch.Meta(),
		Revision: ch.Revision(),
	}
	_, err = s.runRefresh(c, s.cmd, "multi-series", "--path", repoPath, "--force-series")
	c.Assert(err, jc.ErrorIsNil)

	err = app.Refresh()
	c.Assert(err, jc.ErrorIsNil)

	ch, force, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch.Revision(), gc.Equals, 2)
	c.Check(force, gc.Equals, false)
}

func (s *RefreshSuccessStateSuite) TestForcedLXDProfileUpgrade(c *gc.C) {
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
			Base: state.UbuntuBase("22.04"),
			Jobs: []state.MachineJob{state.JobHostUnits},
		},
		state.MachineTemplate{ // parent
			Base: state.UbuntuBase("22.04"),
			Jobs: []state.MachineJob{state.JobHostUnits},
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

	_, err = s.runRefresh(c, s.cmd, "lxd-profile-alt", "--path", repoPath)
	c.Assert(err, gc.ErrorMatches, `invalid lxd-profile.yaml: contains config value "boot.autostart.delay"`)
}

func (s *RefreshSuccessStateSuite) TestInitWithResources(c *gc.C) {
	testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "dummy")
	dir := c.MkDir()

	foopath := path.Join(dir, "foo")
	barpath := path.Join(dir, "bar")
	err := os.WriteFile(foopath, []byte("foo"), 0600)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(barpath, []byte("bar"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	res1 := fmt.Sprintf("foo=%s", foopath)
	res2 := fmt.Sprintf("bar=%s", barpath)

	d := refreshCommand{}
	args := []string{"dummy", "--resource", res1, "--resource", res2}

	err = cmdtesting.InitCommand(modelcmd.Wrap(&d), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, gc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *RefreshSuite) TestUpgradeSameVersionWithResourceUpload(c *gc.C) {
	s.resolvedCharmURL = charm.MustParseURL("ch:quantal/foo-1")
	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL: s.resolvedCharmURL.String(),
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"bar": {
					Name: "bar",
					Type: charmresource.TypeFile,
				},
			},
		},
	}
	s.charmClient.charmResources = []charmresource.Resource{}
	dir := c.MkDir()
	barpath := path.Join(dir, "bar")
	err := os.WriteFile(barpath, []byte("bar"), 0600)
	c.Assert(err, jc.ErrorIsNil)

	res1 := fmt.Sprintf("bar=%s", barpath)

	_, err = s.runRefresh(c, "foo", "--resource="+res1)
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckNoCalls(c)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", model.GenerationMaster, application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL,
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
		ResourceIDs:      map[string]string{"bar": "barId"},
	})
}

type RefreshCharmHubSuite struct {
	BaseRefreshSuite
}

var _ = gc.Suite(&RefreshCharmHubSuite{})

func (s *RefreshCharmHubSuite) SetUpTest(c *gc.C) {
	s.BaseRefreshSuite.SetUpSuite(c)
	s.BaseRefreshSuite.setup(c, charm.MustParseURL("ch:quantal/foo-1"), charm.MustParseURL("ch:quantal/foo-2"))
}

func (s *BaseRefreshSuite) TearDownTest(c *gc.C) {
	//func (s *RefreshCharmHubSuite) TearDownTest(c *gc.C) {
	s.ResetCalls()
}

func (s *RefreshCharmHubSuite) TestUpgradeResourceRevision(c *gc.C) {
	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL: s.resolvedCharmURL.String(),
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"bar": {
					Name: "bar",
					Type: charmresource.TypeFile,
				},
			},
		},
	}
	s.charmClient.charmResources = []charmresource.Resource{
		{
			Meta: charmresource.Meta{
				Name:        "bar",
				Type:        0,
				Path:        "",
				Description: "",
			},
			Origin:   charmresource.OriginStore,
			Revision: 2,
		},
	}

	_, err := s.runRefresh(c, "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmClient.CheckCallNames(c, "CharmInfo", "ListCharmResources", "CharmInfo")
	s.CheckCall(c, 9, "DeployResources", "foo", resources.CharmID{
		URL: s.resolvedCharmURL,
		Origin: commoncharm.Origin{
			ID:           "testing",
			Source:       "charm-hub",
			Risk:         "stable",
			Architecture: arch.DefaultArchitecture,
			Base:         s.testBase}},
		map[string]string(nil),
		map[string]charmresource.Meta{"bar": {Name: "bar", Type: charmresource.TypeFile}},
	)
}

func (s *RefreshCharmHubSuite) TestUpgradeResourceRevisionSupplied(c *gc.C) {
	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL: s.resolvedCharmURL.String(),
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"bar": {
					Name: "bar",
					Type: charmresource.TypeFile,
				},
			},
		},
	}
	s.charmClient.charmResources = []charmresource.Resource{
		{
			Meta: charmresource.Meta{
				Name:        "bar",
				Type:        0,
				Path:        "",
				Description: "",
			},
			Origin:   charmresource.OriginStore,
			Revision: 4,
		},
	}

	_, err := s.runRefresh(c, "foo", "--resource", "bar=3")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmClient.CheckCallNames(c, "CharmInfo", "ListCharmResources", "CharmInfo")
	s.CheckCall(c, 9, "DeployResources", "foo", resources.CharmID{
		URL: s.resolvedCharmURL,
		Origin: commoncharm.Origin{
			ID:           "testing",
			Source:       "charm-hub",
			Risk:         "stable",
			Architecture: arch.DefaultArchitecture,
			Base:         s.testBase}},
		map[string]string{"bar": "3"},
		map[string]charmresource.Meta{"bar": {Name: "bar", Type: charmresource.TypeFile}},
	)
}

func (s *RefreshCharmHubSuite) TestUpgradeResourceNoChange(c *gc.C) {
	s.charmClient.charmInfo = &apicommoncharms.CharmInfo{
		URL: s.resolvedCharmURL.String(),
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"bar": {
					Name: "bar",
					Type: charmresource.TypeFile,
					Path: "/path/to/bar",
				},
			},
		},
	}
	s.charmClient.charmResources = []charmresource.Resource{
		{
			Meta: charmresource.Meta{
				Name: "bar",
				Type: charmresource.TypeFile,
			},
			Origin:   charmresource.OriginStore,
			Revision: 1,
		},
	}

	_, err := s.runRefresh(c, "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmClient.CheckCallNames(c, "CharmInfo", "ListCharmResources", "CharmInfo")
	for _, call := range s.Calls() {
		c.Assert(call.FuncName, gc.Not(gc.Equals), "DeployResources", gc.Commentf("DeployResources should not be called here"))
	}
}

func (s *RefreshSuccessStateSuite) TestForcedUnitsUpgrade(c *gc.C) {
	_, err := s.runRefresh(c, s.cmd, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *RefreshSuccessStateSuite) TestBlockForcedUnitsUpgrade(c *gc.C) {
	// Block operation
	s.BlockAllChanges(c, "TestBlockForcedUpgrade")
	_, err := s.runRefresh(c, s.cmd, "riak", "--force-units", "--path", s.path)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 8, true)
	s.AssertCharmUploaded(c, curl)
	// Local revision is not changed.
	s.assertLocalRevision(c, 7, s.path)
}

func (s *RefreshSuccessStateSuite) TestCharmPath(c *gc.C) {
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")

	// Change the revision to 42 and upgrade to it with explicit revision.
	err := os.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.runRefresh(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 42, false)
	c.Assert(curl, gc.Equals, "local:bionic/riak-42")
	s.assertLocalRevision(c, 42, myriakPath)
}

func (s *RefreshSuccessStateSuite) TestCharmPathNotFound(c *gc.C) {
	myriakPath := filepath.Join(c.MkDir(), "riak")
	_, err := os.Stat(myriakPath)
	c.Assert(err, gc.ErrorMatches, ".*no such file or directory")
	_, err = s.runRefresh(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, ".*file does not exist")
}

func (s *RefreshSuccessStateSuite) TestSwitchToLocal(c *gc.C) {
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")

	// Change the revision to 42 and upgrade to it with explicit revision.
	err := os.WriteFile(path.Join(myriakPath, "revision"), []byte("42"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.runRefresh(c, s.cmd, "riak", "--switch", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	rev := 42
	curl, origin := s.assertUpgraded(c, s.riak, rev, false)
	c.Assert(curl, gc.Equals, "local:bionic/riak-42")
	c.Assert(origin, gc.DeepEquals, &state.CharmOrigin{
		Source:   "local",
		Revision: &rev,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "18.04",
		},
	})
	s.assertLocalRevision(c, rev, myriakPath)
}

func (s *RefreshSuccessStateSuite) TestSwitchToLocalNotFound(c *gc.C) {
	myriakPath := filepath.Join(c.MkDir(), "riak")
	_, err := os.Stat(myriakPath)
	c.Assert(err, gc.ErrorMatches, ".*no such file or directory")
	_, err = s.runRefresh(c, s.cmd, "riak", "--switch", myriakPath)
	c.Assert(err, gc.ErrorMatches, ".*file does not exist")
}

func (s *RefreshSuccessStateSuite) TestCharmPathNoRevUpgrade(c *gc.C) {
	// Revision 7 is running to start with.
	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	s.assertLocalRevision(c, 7, myriakPath)
	_, err := s.runRefresh(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	curl, _ := s.assertUpgraded(c, s.riak, 8, false)
	c.Assert(curl, gc.Equals, "local:bionic/riak-8")
}

func (s *RefreshSuccessStateSuite) TestCharmPathDifferentNameFails(c *gc.C) {
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
	_, err = s.runRefresh(c, s.cmd, "riak", "--path", myriakPath)
	c.Assert(err, gc.ErrorMatches, `cannot refresh "riak" to "myriak"`)
}

type mockAPIConnection struct {
	api.Connection
	serverVersion *version.Number
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
	store.CharmAdder
	testing.Stub
}

func (m *mockCharmAdder) AddCharm(curl *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
	m.MethodCall(m, "AddCharm", curl, origin, force)
	return origin, m.NextErr()
}

func (m *mockCharmAdder) CheckCharmPlacement(appName string, curl *charm.URL) error {
	m.MethodCall(m, "CheckCharmPlacement", appName, curl)
	return m.NextErr()
}

func (m *mockCharmAdder) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	m.MethodCall(m, "AddLocalCharm", curl, ch, force)
	return curl, m.NextErr()
}

type mockCharmClient struct {
	utils.CharmClient
	testing.Stub
	charmInfo      *apicommoncharms.CharmInfo
	charmResources []charmresource.Resource
}

func (m *mockCharmClient) CharmInfo(curl string) (*apicommoncharms.CharmInfo, error) {
	m.MethodCall(m, "CharmInfo", curl)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.charmInfo, nil
}

func (m *mockCharmClient) ListCharmResources(curl string, origin commoncharm.Origin) ([]charmresource.Resource, error) {
	m.MethodCall(m, "ListCharmResources", curl, origin)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.charmResources, nil
}

type mockCharmResolver struct {
	testing.Stub
	resolveFunc func(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error)
}

func (m *mockCharmResolver) ResolveCharm(url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error) {
	return m.resolveFunc(url, preferredOrigin, switchCharm)
}

type mockCharmRefreshClient struct {
	CharmRefreshClient
	testing.Stub
	charmURL    *charm.URL
	charmOrigin commoncharm.Origin

	bindings map[string]string
}

func (m *mockCharmRefreshClient) GetCharmURLOrigin(branchName, appName string) (*charm.URL, commoncharm.Origin, error) {
	m.MethodCall(m, "GetCharmURLOrigin", branchName, appName)
	return m.charmURL, m.charmOrigin, m.NextErr()
}

func (m *mockCharmRefreshClient) SetCharm(branchName string, cfg application.SetCharmConfig) error {
	m.MethodCall(m, "SetCharm", branchName, cfg)
	return m.NextErr()
}

func (m *mockCharmRefreshClient) Get(_, applicationName string) (*params.ApplicationGetResults, error) {
	m.MethodCall(m, "Get", applicationName)
	return &params.ApplicationGetResults{
		EndpointBindings: m.bindings,
	}, m.NextErr()
}

func newMockModelConfigGetter() mockModelConfigGetter {
	return mockModelConfigGetter{cfg: coretesting.FakeConfig()}
}

type mockModelConfigGetter struct {
	deployer.ModelConfigGetter
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

func (m *mockModelConfigGetter) Close() error {
	return nil
}

type mockResourceLister struct {
	utils.ResourceLister
	testing.Stub
}

func (m *mockResourceLister) ListResources([]string) ([]coreresouces.ApplicationResources, error) {
	return []coreresouces.ApplicationResources{{
		Resources: []coreresouces.Resource{{
			Resource: charmresource.Resource{
				Meta: charmresource.Meta{
					Name: "bar",
					Type: charmresource.TypeFile,
					Path: "/path/to/bar",
				},
				Revision: 1,
			},
		}},
	}}, nil
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

type mockDownloadBundleClient struct {
	testing.Stub
	bundle charm.Bundle
}

func (m *mockDownloadBundleClient) DownloadAndReadBundle(_ context.Context, resourceURL *url.URL, archivePath string, _ ...charmhub.DownloadOption) (charm.Bundle, error) {
	m.MethodCall(m, "DownloadAndReadBundle", resourceURL, archivePath)
	return m.bundle, m.NextErr()
}
