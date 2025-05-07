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
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/resources"
	commoncharm "github.com/juju/juju/api/common/charm"
	apicommoncharms "github.com/juju/juju/api/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/application/deployer"
	"github.com/juju/juju/cmd/juju/application/store"
	apputils "github.com/juju/juju/cmd/juju/application/utils"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/network"
	coreresouces "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/storage"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
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

func (s *BaseRefreshSuite) runRefresh(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.refreshCommand(), args...)
}

type RefreshSuite struct {
	BaseRefreshSuite
}

var _ = tc.Suite(&RefreshSuite{})

func (s *RefreshSuite) SetUpTest(c *tc.C) {
	s.BaseRefreshSuite.SetUpSuite(c)
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@12.10"), charm.MustParseURL("ch:foo-1"), charm.MustParseURL("ch:foo-2"))
}

func (s *BaseRefreshSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub.ResetCalls()
}

func (s *BaseRefreshSuite) setup(c *tc.C, b corebase.Base, currentCharmURL, latestCharmURL *charm.URL) {
	// Create persistent cookies in a temporary location.
	cookieFile := filepath.Join(c.MkDir(), "cookies")
	s.PatchEnvironment("JUJU_COOKIEFILE", cookieFile)

	s.testBase = b
	s.testPlatform = corecharm.MustParsePlatform(fmt.Sprintf("%s/%s/%s", arch.DefaultArchitecture, s.testBase.OS, s.testBase.Channel))

	s.deployResources = func(
		_ context.Context,
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
		serverVersion: &semversion.Number{
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
			Source:       schemaToOriginSource(currentCharmURL.Schema),
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
	s.downloadBundleClient = mockDownloadBundleClient{}
}

func schemaToOriginSource(schema string) commoncharm.OriginSource {
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
			"admin/bar": {},
		},
	}
	memStore.Accounts["foo"] = jujuclient.AccountDetails{
		User: "admin", Password: "hunter2",
	}
	apiOpen := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
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
		func(conn base.APICallCloser) apputils.CharmClient {
			s.AddCall("NewCharmClient", conn)
			s.PopNoErr()
			return &s.charmClient
		},
		func(conn base.APICallCloser) CharmRefreshClient {
			s.AddCall("NewCharmAPIClient", conn)
			s.PopNoErr()
			return &s.charmAPIClient
		},
		func(conn base.APICallCloser) (apputils.ResourceLister, error) {
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

func (s *RefreshSuite) TestStorageDirectives(c *tc.C) {
	_, err := s.runRefresh(c, "foo", "--storage", "bar=baz")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
			Origin: commoncharm.Origin{
				ID:           "testing",
				Source:       "charm-hub",
				Risk:         "stable",
				Architecture: arch.DefaultArchitecture,
				Base:         s.testBase,
			},
		},
		StorageDirectives: map[string]storage.Directive{
			"bar": {Pool: "baz", Count: 1},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestConfigSettings(c *tc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runRefresh(c, "foo", "--config", configFile)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestConfigSettingsWithTrust(c *tc.C) {
	_, err := s.runRefresh(c, "foo", "--trust", "--config", "foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestConfigSettingsWithTrustFalse(c *tc.C) {
	_, err := s.runRefresh(c, "foo", "--trust=false", "--config", "foo=bar")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestConfigSettingsWithKeyValuesAndFile(c *tc.C) {
	tempdir := c.MkDir()
	configFile := filepath.Join(tempdir, "config.yaml")
	err := os.WriteFile(configFile, []byte("foo:{}"), 0644)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runRefresh(c, "foo", "--trust", "--config", "foo=bar", "--config", configFile)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestUpgradeWithBindDefaults(c *tc.C) {
	s.charmAPIClient.bindings = map[string]string{
		"": "testing",
	}

	s.testUpgradeWithBind(c, map[string]string{
		"ep1": "sp1",
		"ep2": "testing",
	})
}

func (s *RefreshSuite) testUpgradeWithBind(c *tc.C, expectedBindings map[string]string) {
	s.apiConnection = mockAPIConnection{
		serverVersion: &semversion.Number{
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

	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestUpgradeWithBindAndUnknownEndpoint(c *tc.C) {
	s.apiConnection = mockAPIConnection{
		serverVersion: &semversion.Number{
			Major: 1,
			Minor: 2,
			Patch: 3,
		},
	}

	s.charmClient.charmInfo.Meta.ExtraBindings = map[string]charm.ExtraBinding{
		"ep1": {Name: "ep1"},
	}

	_, err := s.runRefresh(c, "foo", "--bind", "unknown=sp1")
	c.Assert(err, tc.ErrorMatches, `endpoint "unknown" not found`)
}

func (s *RefreshSuite) TestInvalidArgs(c *tc.C) {
	_, err := s.runRefresh(c)
	c.Assert(err, tc.ErrorMatches, "no application specified")
	_, err = s.runRefresh(c, "invalid:name")
	c.Assert(err, tc.ErrorMatches, `invalid application name "invalid:name"`)
	_, err = s.runRefresh(c, "foo", "bar")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["bar"\]`)
}

func (s *RefreshSuite) TestNoPathFails(c *tc.C) {
	s.charmAPIClient.charmURL = charm.MustParseURL("local:riak")
	_, err := s.runRefresh(c, "riak")
	c.Assert(err, tc.ErrorMatches, "refreshing a local charm requires either --path or --switch")
}

func (s *RefreshSuite) TestSwitchAndRevisionFails(c *tc.C) {
	_, err := s.runRefresh(c, "riak", "--switch=riak", "--revision=2")
	c.Assert(err, tc.ErrorMatches, "--switch and --revision are mutually exclusive")
}

func (s *RefreshSuite) TestPathAndRevisionFails(c *tc.C) {
	_, err := s.runRefresh(c, "riak", "--path=foo", "--revision=2")
	c.Assert(err, tc.ErrorMatches, "--path and --revision are mutually exclusive")
}

func (s *RefreshSuite) TestSwitchAndPathFails(c *tc.C) {
	_, err := s.runRefresh(c, "riak", "--switch=riak", "--path=foo")
	c.Assert(err, tc.ErrorMatches, "--switch and --path are mutually exclusive")
}

func (s *RefreshSuite) TestInvalidRevision(c *tc.C) {
	_, err := s.runRefresh(c, "riak", "--revision=blah")
	c.Assert(err, tc.ErrorMatches, `invalid value "blah" for option --revision: strconv.(ParseInt|Atoi): parsing "blah": invalid syntax`)
}

func (s *RefreshSuite) TestLocalRevisionUnchanged(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:riak"), charm.MustParseURL("ch:riak"))
	s.charmAPIClient.charmOrigin = commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@18.04")}

	path := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	_, err := s.runRefresh(c, "riak", "--path", s.archivePath(c, path))
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCallNames(c, "AddLocalCharm")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 7
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "riak",
		CharmID: application.CharmID{
			URL: "local:riak-7",
			Origin: commoncharm.Origin{
				Base:     s.charmAPIClient.charmOrigin.Base,
				Source:   "local",
				Revision: &rev,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestUpgradeWithChannel(c *tc.C) {
	s.resolvedChannel = charm.Beta
	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	origin, _ := apputils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Beta}, corecharm.Platform{
		Architecture: arch.DefaultArchitecture,
	})
	origin.ID = "testing"
	origin.Revision = (*int)(nil)
	origin.Architecture = arch.DefaultArchitecture
	origin.Base = s.testBase
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestUpgradeWithChannelNoNewCharmURL(c *tc.C) {
	// Test setting a new charm channel, without an actual
	// charm upgrade needed.
	s.resolvedChannel = charm.Beta
	s.resolvedCharmURL = charm.MustParseURL("ch:foo-1")

	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestRefreshShouldRespectDeployedChannelByDefault(c *tc.C) {
	s.resolvedChannel = charm.Beta
	_, err := s.runRefresh(c, "foo")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAdder.CheckCallNames(c, "AddCharm")
	origin, _ := apputils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Beta}, s.testPlatform)
	origin.ID = "testing"
	origin.Revision = (*int)(nil)
	s.charmAdder.CheckCall(c, 0, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) TestUpgradeFailWithoutCharmHubOriginID(c *tc.C) {
	s.resolvedChannel = charm.Beta
	s.charmAPIClient.charmOrigin.Source = "charm-hub"
	s.charmAPIClient.charmOrigin.ID = ""
	_, err := s.runRefresh(c, "foo", "--channel=beta")
	c.Assert(err, tc.ErrorMatches, "\"foo\" deploy incomplete, please try refresh again in a little bit.")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin")
}

func (s *RefreshSuite) TestSwitch(c *tc.C) {
	_, err := s.runRefresh(c, "foo", "--switch=ch:trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)

	s.charmClient.CheckCallNames(c, "CharmInfo", "CharmInfo")
	s.charmClient.CheckCall(c, 0, "CharmInfo", s.resolvedCharmURL.String())
	s.charmAdder.CheckCallNames(c, "CheckCharmPlacement", "AddCharm")
	origin, _ := apputils.MakeOrigin(charm.Schema(s.resolvedCharmURL.Schema), s.resolvedCharmURL.Revision, charm.Channel{Risk: charm.Stable}, s.testPlatform)

	parsedSwitchUrl, err := charm.ParseURL("ch:trusty/anotherriak")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCall(c, 0, "CheckCharmPlacement", "foo", parsedSwitchUrl)
	origin.Revision = (*int)(nil)
	s.charmAdder.CheckCall(c, 1, "AddCharm", s.resolvedCharmURL, origin, false)
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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
	c.Assert(curl, tc.NotNil)
	c.Assert(curl.String(), tc.Equals, "ch:trusty/anotherriak")
}

func (s *RefreshSuite) TestSwitchSameURL(c *tc.C) {
	s.charmAPIClient.charmURL = s.resolvedCharmURL
	_, err := s.runRefresh(c, "foo", "--switch="+s.resolvedCharmURL.String())
	// Should not get error since charm already up-to-date
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshSuite) TestSwitchDifferentRevision(c *tc.C) {
	curlCopy := *s.resolvedCharmURL
	s.charmAPIClient.charmURL = &curlCopy
	s.resolvedCharmURL.Revision++
	_, err := s.runRefresh(c, "riak", "--switch="+s.resolvedCharmURL.String())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *RefreshSuite) TestSwitchToLocalNotFound(c *tc.C) {
	myriakPath := filepath.Join(c.MkDir(), "riak")
	_, err := os.Stat(myriakPath)
	c.Assert(err, tc.ErrorMatches, ".*no such file or directory")
	_, err = s.runRefresh(c, "riak", "--switch", myriakPath)
	c.Assert(err, tc.ErrorMatches, ".*file does not exist")
}

func (s *RefreshSuite) TestUpgradeWithTermsNotSigned(c *tc.C) {
	termsRequiredError := &common.TermsRequiredError{Terms: []string{"term/1", "term/2"}}
	s.charmAdder.SetErrors(termsRequiredError)
	expectedError := `Declined: some terms require agreement. Try: "juju agree term/1 term/2"`
	_, err := s.runRefresh(c, "terms1")
	c.Assert(err, tc.ErrorMatches, expectedError)
}

func (s *RefreshSuite) TestRespectsLocalRevisionWhenPossible(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:riak"), charm.MustParseURL("ch:riak"))
	s.charmAPIClient.charmOrigin = commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@18.04")}

	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")

	// Set the disk revision to 42.
	revFile, err := os.OpenFile(filepath.Join(myriakPath, "revision"), os.O_CREATE|os.O_WRONLY, 0666)
	c.Assert(err, jc.ErrorIsNil)
	_, err = revFile.WriteString("42")
	c.Check(err, jc.ErrorIsNil)
	err = revFile.Close()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.runRefresh(c, "riak", "--path", s.archivePath(c, myriakPath))
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCallNames(c, "AddLocalCharm")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "riak",
		CharmID: application.CharmID{
			URL: "local:riak-42",
			Origin: commoncharm.Origin{
				Base:     s.charmAPIClient.charmOrigin.Base,
				Source:   "local",
				Revision: ptr(42),
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestForcedSeriesUpgrade(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:multi-series"), charm.MustParseURL("ch:multi-series"))
	repoPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "multi-series")
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

	// TODO (jam) 2024-11-15: this test is kept for backward compatibility,
	//  in 3.x the --force-series argument exists, though it is being
	//  replaced by --force-base
	_, err = s.runRefresh(c, "multi-series", "--path", s.archivePath(c, repoPath), "--force-series")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 1
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "multi-series",
		CharmID: application.CharmID{
			URL: "local:multi-series-1",
			Origin: commoncharm.Origin{
				Base:         s.charmAPIClient.charmOrigin.Base,
				Source:       "local",
				Architecture: arch.DefaultArchitecture,
				Revision:     &rev,
			},
		},
		ForceBase:        true,
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestForcedBaseUpgrade(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:multi-base"), charm.MustParseURL("ch:multi-base"))

	// Overwrite the manifest.yaml to change the supported series.
	repoPath := testcharms.RepoWithSeries("jammy").ClonedDirPath(c.MkDir(), "multi-base")
	manifestPath := filepath.Join(repoPath, "manifest.yaml")
	file, err := os.OpenFile(manifestPath, os.O_TRUNC|os.O_RDWR, 0666)
	if err != nil {
		c.Fatal(errors.Annotate(err, "cannot open manifest.yaml for overwriting"))
	}
	defer func() { _ = file.Close() }()

	// We deployed a version of the charm that supported jammy (22.04), but
	// now we declare that this charm only supports focal, but with a
	// --force-base we are allowed to target it anyway.
	manifest := strings.Join(
		[]string{
			`bases:`,
			`- architectures:`,
			`  - amd64`,
			// Now only supports focal
			`  channel: '20.04'`,
			`  name: ubuntu`,
		},
		"\n",
	)
	if _, err := file.WriteString(manifest); err != nil {
		c.Fatal(errors.Annotate(err, "cannot write to manifest.yaml"))
	}

	_, err = s.runRefresh(c, "multi-base", "--base", "ubuntu@20.04",
		"--path", s.archivePath(c, repoPath), "--force-base")
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 1
	origin := commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@20.04")}
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "multi-base",
		CharmID: application.CharmID{
			URL: "local:multi-base-1",
			Origin: commoncharm.Origin{
				Base:         origin.Base,
				Source:       "local",
				Architecture: arch.DefaultArchitecture,
				Revision:     &rev,
			},
		},
		ForceBase:        true,
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestForcedUnitsUpgrade(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:riak"), charm.MustParseURL("ch:riak"))
	s.charmAPIClient.charmOrigin = commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@18.04")}

	myriakPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "riak")
	_, err := s.runRefresh(c, "riak", "--path", s.archivePath(c, myriakPath), "--force-units")
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCallNames(c, "AddLocalCharm")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 7
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "riak",
		CharmID: application.CharmID{
			URL: "local:riak-7",
			Origin: commoncharm.Origin{
				Base:     s.charmAPIClient.charmOrigin.Base,
				Source:   "local",
				Revision: &rev,
			},
		},
		ForceUnits:       true,
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestBlockRefresh(c *tc.C) {
	s.charmAPIClient.SetErrors(nil, nil, apiservererrors.OperationBlockedError("refresh"))

	_, err := s.runRefresh(c, "riak")
	c.Assert(err, tc.NotNil)
	c.Assert(strings.Contains(err.Error(), "All operations that change model have been disabled for the current model"), jc.IsTrue)
}

func (s *RefreshSuite) TestCharmPathNotFound(c *tc.C) {
	myriakPath := filepath.Join(c.MkDir(), "riak")
	_, err := os.Stat(myriakPath)
	c.Assert(err, tc.ErrorMatches, ".*no such file or directory")
	_, err = s.runRefresh(c, "riak", "--path", myriakPath)
	c.Assert(err, tc.ErrorMatches, ".*file does not exist")
}

func (s *RefreshSuite) TestCharmPathNoRevUpgrade(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("local:riak"), charm.MustParseURL("local:riak"))
	s.charmAPIClient.charmOrigin = commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@18.04")}
	// Revision 7 is running to start with.
	myriakPath := testcharms.RepoWithSeries("bionic").CharmArchivePath(c.MkDir(), "riak")

	_, err := s.runRefresh(c, "riak", "--path", myriakPath)
	c.Assert(err, jc.ErrorIsNil)
	s.charmAdder.CheckCallNames(c, "AddLocalCharm")
	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 7
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "riak",
		CharmID: application.CharmID{
			URL: "local:riak-7",
			Origin: commoncharm.Origin{
				Base:     s.charmAPIClient.charmOrigin.Base,
				Source:   "local",
				Revision: &rev,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestCharmPathDifferentNameFails(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("local:riak"), charm.MustParseURL("local:riak"))
	s.charmAPIClient.charmOrigin = commoncharm.Origin{Base: corebase.MustParseBaseFromString("ubuntu@18.04")}
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

	_, err = s.runRefresh(c, "riak", "--path", s.archivePath(c, myriakPath))
	c.Assert(err, tc.ErrorMatches, `cannot refresh "riak" to "myriak"`)
}

func (s *RefreshSuite) TestForcedLXDProfileUpgrade(c *tc.C) {
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@18.04"), charm.MustParseURL("ch:lxd-profile-alt"), charm.MustParseURL("ch:lxd-profile-alt"))
	repoPath := testcharms.RepoWithSeries("bionic").ClonedDirPath(c.MkDir(), "lxd-profile-alt")
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

	_, err = s.runRefresh(c, "lxd-profile-alt", "--path", s.archivePath(c, repoPath))
	c.Assert(err, jc.ErrorIsNil)

	s.charmAPIClient.CheckCallNames(c, "GetCharmURLOrigin", "Get", "SetCharm")
	rev := 0
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "lxd-profile-alt",
		CharmID: application.CharmID{
			URL: "local:lxd-profile-alt-0",
			Origin: commoncharm.Origin{
				Base:         s.charmAPIClient.charmOrigin.Base,
				Source:       "local",
				Architecture: arch.DefaultArchitecture,
				Revision:     &rev,
			},
		},
		ConfigSettings:   map[string]string{},
		EndpointBindings: map[string]string{},
	})
}

func (s *RefreshSuite) TestInitWithResources(c *tc.C) {
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

	cmd := modelcmd.Wrap(&d)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	err = cmdtesting.InitCommand(cmd, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(d.Resources, tc.DeepEquals, map[string]string{
		"foo": foopath,
		"bar": barpath,
	})
}

func (s *RefreshSuite) TestUpgradeSameVersionWithResourceUpload(c *tc.C) {
	s.resolvedCharmURL = charm.MustParseURL("ch:foo-1")
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
	s.charmAPIClient.CheckCall(c, 2, "SetCharm", application.SetCharmConfig{
		ApplicationName: "foo",
		CharmID: application.CharmID{
			URL: s.resolvedCharmURL.String(),
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

func (s *RefreshSuite) archivePath(c *tc.C, path string) string {
	charm, err := charmtesting.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)

	archivePath := filepath.Join(c.MkDir(), "myriak.charm")
	err = charm.ArchiveToPath(archivePath)
	c.Assert(err, jc.ErrorIsNil)

	return archivePath
}

type RefreshCharmHubSuite struct {
	BaseRefreshSuite
}

var _ = tc.Suite(&RefreshCharmHubSuite{})

func (s *RefreshCharmHubSuite) SetUpTest(c *tc.C) {
	s.BaseRefreshSuite.SetUpSuite(c)
	s.BaseRefreshSuite.setup(c, corebase.MustParseBaseFromString("ubuntu@12.10"), charm.MustParseURL("ch:foo-1"), charm.MustParseURL("ch:foo-2"))
}

func (s *BaseRefreshSuite) TearDownTest(c *tc.C) {
	//func (s *RefreshCharmHubSuite) TearDownTest(c *gc.C) {
	s.ResetCalls()
}

func (s *RefreshCharmHubSuite) TestUpgradeResourceRevision(c *tc.C) {
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
		URL: s.resolvedCharmURL.String(),
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

func (s *RefreshCharmHubSuite) TestUpgradeResourceRevisionSupplied(c *tc.C) {
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
		URL: s.resolvedCharmURL.String(),
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

func (s *RefreshCharmHubSuite) TestUpgradeResourceNoChange(c *tc.C) {
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
		c.Assert(call.FuncName, tc.Not(tc.Equals), "DeployResources", tc.Commentf("DeployResources should not be called here"))
	}
}

type mockAPIConnection struct {
	api.Connection
	serverVersion *semversion.Number
	authTag       names.Tag
}

func (m *mockAPIConnection) Addr() *url.URL {
	return &url.URL{
		Scheme: "wss",
		Host:   "0.1.2.3:1234",
	}
}

func (m *mockAPIConnection) IsProxied() bool {
	return false
}

func (m *mockAPIConnection) IPAddr() string {
	return "0.1.2.3:1234"
}

func (m *mockAPIConnection) AuthTag() names.Tag {
	return m.authTag
}

func (m *mockAPIConnection) PublicDNSName() string {
	return ""
}

func (m *mockAPIConnection) APIHostPorts() []network.MachineHostPorts {
	url := m.Addr()
	hp, _ := network.ParseMachineHostPort(url.Host)
	return []network.MachineHostPorts{{*hp}}
}

func (m *mockAPIConnection) ServerVersion() (semversion.Number, bool) {
	if m.serverVersion != nil {
		return *m.serverVersion, true
	}
	return semversion.Number{}, false
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

func (m *mockCharmAdder) AddCharm(ctx context.Context, curl *charm.URL, origin commoncharm.Origin, force bool) (commoncharm.Origin, error) {
	m.MethodCall(m, "AddCharm", curl, origin, force)
	return origin, m.NextErr()
}

func (m *mockCharmAdder) CheckCharmPlacement(ctx context.Context, appName string, curl *charm.URL) error {
	m.MethodCall(m, "CheckCharmPlacement", appName, curl)
	return m.NextErr()
}

func (m *mockCharmAdder) AddLocalCharm(ctx context.Context, curl *charm.URL, ch charm.Charm, force bool) (*charm.URL, error) {
	m.MethodCall(m, "AddLocalCharm", curl, ch, force)
	return curl, m.NextErr()
}

type mockCharmClient struct {
	apputils.CharmClient
	testing.Stub
	charmInfo      *apicommoncharms.CharmInfo
	charmResources []charmresource.Resource
}

func (m *mockCharmClient) CharmInfo(ctx context.Context, curl string) (*apicommoncharms.CharmInfo, error) {
	m.MethodCall(m, "CharmInfo", curl)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.charmInfo, nil
}

func (m *mockCharmClient) ListCharmResources(ctx context.Context, curl string, origin commoncharm.Origin) ([]charmresource.Resource, error) {
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

func (m *mockCharmResolver) ResolveCharm(ctx context.Context, url *charm.URL, preferredOrigin commoncharm.Origin, switchCharm bool) (*charm.URL, commoncharm.Origin, []corebase.Base, error) {
	return m.resolveFunc(url, preferredOrigin, switchCharm)
}

type mockCharmRefreshClient struct {
	CharmRefreshClient
	testing.Stub
	charmURL    *charm.URL
	charmOrigin commoncharm.Origin

	bindings map[string]string
}

func (m *mockCharmRefreshClient) GetCharmURLOrigin(ctx context.Context, appName string) (*charm.URL, commoncharm.Origin, error) {
	m.MethodCall(m, "GetCharmURLOrigin", appName)
	return m.charmURL, m.charmOrigin, m.NextErr()
}

func (m *mockCharmRefreshClient) SetCharm(ctx context.Context, cfg application.SetCharmConfig) error {
	m.MethodCall(m, "SetCharm", cfg)
	return m.NextErr()
}

func (m *mockCharmRefreshClient) Get(ctx context.Context, applicationName string) (*params.ApplicationGetResults, error) {
	m.MethodCall(m, "Get", applicationName)
	return &params.ApplicationGetResults{
		EndpointBindings: m.bindings,
		Base: params.Base{
			Name:    m.charmOrigin.Base.OS,
			Channel: m.charmOrigin.Base.Channel.String(),
		},
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

func (m *mockModelConfigGetter) ModelGet(ctx context.Context) (map[string]interface{}, error) {
	m.MethodCall(m, "ModelGet")
	return m.cfg, m.NextErr()
}

func (m *mockModelConfigGetter) SetDefaultSpace(ctx context.Context, name string) {
	m.cfg["default-space"] = name
}

func (m *mockModelConfigGetter) Close() error {
	return nil
}

type mockResourceLister struct {
	apputils.ResourceLister
	testing.Stub
}

func (m *mockResourceLister) ListResources(context.Context, []string) ([]coreresouces.ApplicationResources, error) {
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

func (m *mockSpacesClient) ListSpaces(ctx context.Context) ([]params.Space, error) {
	m.MethodCall(m, "ListSpaces")
	return m.spaceList, m.NextErr()
}

type mockDownloadBundleClient struct {
	testing.Stub
}

func (m *mockDownloadBundleClient) Download(_ context.Context, resourceURL *url.URL, archivePath string, _ ...charmhub.DownloadOption) (*charmhub.Digest, error) {
	m.MethodCall(m, "DownloadAndReadBundle", resourceURL, archivePath)
	return &charmhub.Digest{}, m.NextErr()
}
