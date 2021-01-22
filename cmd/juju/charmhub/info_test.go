// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/environs/config"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
)

type infoSuite struct {
	testing.IsolationSuite

	infoCommandAPI *mocks.MockInfoCommandAPI
	modelConfigAPI *mocks.MockModelConfigClient
	apiRoot        *basemocks.MockAPICallCloser
}

var _ = gc.Suite(&infoSuite{})

func (s *infoSuite) TestInitNoArgs(c *gc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
	}
	err := command.Init([]string{})
	c.Assert(err, gc.NotNil)
}

func (s *infoSuite) TestInitSuccess(c *gc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
	}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestInitFailCS(c *gc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{},
	}
	err := command.Init([]string{"cs:test"})
	c.Assert(err, gc.ErrorMatches, "\"cs:test\" is not a Charm Hub charm")
}

func (s *infoSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *infoSuite) TestRunJSON(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{"latest/stable":{"released-at":"2019-12-16T19:44:44.076943+00:00","track":"latest","risk":"stable","revision":16,"size":12042240,"version":"1.0.3","architectures":["amd64"],"series":["bionic","xenial"]}},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunJSONSpecifySeriesNotDefault(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json", "--series", "xenial"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{"latest/stable":{"released-at":"2019-12-16T19:44:44.076943+00:00","track":"latest","risk":"stable","revision":16,"size":12042240,"version":"1.0.3","architectures":["amd64"],"series":["bionic","xenial"]}},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunJSONSpecifyArch(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json", "--arch", "amd64"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{"latest/stable":{"released-at":"2019-12-16T19:44:44.076943+00:00","track":"latest","risk":"stable","revision":16,"size":12042240,"version":"1.0.3","architectures":["amd64"],"series":["bionic","xenial"]}},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunJSONWithSeriesFoundChannel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", "focal", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunJSONDefaultSeriesFoundChannel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()
	s.expectModelConfig(c, "bionic")

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", "", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{"latest/stable":{"released-at":"2019-12-16T19:44:44.076943+00:00","track":"latest","risk":"stable","revision":16,"size":12042240,"version":"1.0.3","architectures":["amd64"],"series":["bionic","xenial"]}},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunJSONDefaultSeriesNotFoundNoChannel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()
	s.expectModelConfig(c, "focal")

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", "", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `{"type":"charm","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","description":"This will install and setup WordPress optimized to run in the cloud.","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","series":["bionic","xenial"],"store-url":"https://someurl.com/wordpress","tags":["app","seven"],"charm":{"config":{"Options":{"agility-ratio":{"Type":"float","Description":"A number from 0 to 1 indicating agility.","Default":null},"outlook":{"Type":"string","Description":"No default outlook.","Default":null},"reticulate-splines":{"Type":"boolean","Description":"Whether to reticulate splines on launch, or not.","Default":null},"skill-level":{"Type":"int","Description":"A number indicating skill.","Default":null},"subtitle":{"Type":"string","Description":"An optional subtitle used for the application.","Default":""},"title":{"Type":"string","Description":"A descriptive title used for the application.","Default":"My Title"},"username":{"Type":"string","Description":"The name of the initial account (given admin permissions).","Default":"admin001"}}},"relations":{"provides":{"source":"dummy-token"},"requires":{"sink":"dummy-token"}},"used-by":["wordpress-everlast","wordpress-jorge","wordpress-site"]},"channel-map":{},"tracks":["latest"]}
`)
}

func (s *infoSuite) TestRunYAML(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) InfoCommandAPI {
			return s.infoCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "yaml"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
type: charm
id: charmCHARMcharmCHARMcharmCHARM01
name: wordpress
description: This will install and setup WordPress optimized to run in the cloud.
publisher: Wordress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
series:
- bionic
- xenial
store-url: https://someurl.com/wordpress
tags:
- app
- seven
charm:
  config:
    options:
      agility-ratio:
        type: float
        description: A number from 0 to 1 indicating agility.
      outlook:
        type: string
        description: No default outlook.
      reticulate-splines:
        type: boolean
        description: Whether to reticulate splines on launch, or not.
      skill-level:
        type: int
        description: A number indicating skill.
      subtitle:
        type: string
        description: An optional subtitle used for the application.
        default: ""
      title:
        type: string
        description: A descriptive title used for the application.
        default: My Title
      username:
        type: string
        description: The name of the initial account (given admin permissions).
        default: admin001
  relations:
    provides:
      source: dummy-token
    requires:
      sink: dummy-token
  used-by:
  - wordpress-everlast
  - wordpress-jorge
  - wordpress-site
channel-map:
  latest/stable:
    released-at: "2019-12-16T19:44:44.076943+00:00"
    track: latest
    risk: stable
    revision: 16
    size: 12042240
    version: 1.0.3
    architectures:
    - amd64
    series:
    - bionic
    - xenial
tracks:
- latest
`[1:])
}

func (s *infoSuite) newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
		ModelConfigClientFunc: func(api base.APICallCloser) ModelConfigClient {
			return s.modelConfigAPI
		},
	}
}

func (s *infoSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.infoCommandAPI = mocks.NewMockInfoCommandAPI(ctrl)
	s.infoCommandAPI.EXPECT().Close().AnyTimes()

	s.modelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	s.modelConfigAPI.EXPECT().Close().AnyTimes()

	s.apiRoot = basemocks.NewMockAPICallCloser(ctrl)
	s.apiRoot.EXPECT().Close().AnyTimes()
	s.apiRoot.EXPECT().BestFacadeVersion("CharmHub").Return(1)

	return ctrl
}

func (s *infoSuite) expectInfo() {
	s.infoCommandAPI.EXPECT().Info("test", gomock.Any()).Return(charmhub.InfoResponse{
		Name:        "wordpress",
		Type:        "charm",
		ID:          "charmCHARMcharmCHARMcharmCHARM01",
		Description: "This will install and setup WordPress optimized to run in the cloud.",
		Publisher:   "Wordress Charmers",
		Summary:     "WordPress is a full featured web blogging tool, this charm deploys it.",
		Tracks:      []string{"latest"},
		Series:      []string{"bionic", "xenial"},
		StoreURL:    "https://someurl.com/wordpress",
		Tags:        []string{"app", "seven"},
		Channels: map[string]charmhub.Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Size:       12042240,
				Revision:   16,
				Version:    "1.0.3",
				Platforms: []charmhub.Platform{
					{Architecture: "amd64", Series: "bionic"},
					{Architecture: "amd64", Series: "xenial"},
				},
			}},
		Charm: &charmhub.Charm{
			Subordinate: false,
			Config: &charm.Config{
				Options: map[string]charm.Option{
					"reticulate-splines": {Type: "boolean", Description: "Whether to reticulate splines on launch, or not."},
					"title":              {Type: "string", Description: "A descriptive title used for the application.", Default: "My Title"},
					"subtitle":           {Type: "string", Description: "An optional subtitle used for the application.", Default: ""},
					"outlook":            {Type: "string", Description: "No default outlook."},
					"username":           {Type: "string", Description: "The name of the initial account (given admin permissions).", Default: "admin001"},
					"skill-level":        {Type: "int", Description: "A number indicating skill."},
					"agility-ratio":      {Type: "float", Description: "A number from 0 to 1 indicating agility."},
				},
			},
			Relations: map[string]map[string]string{
				"provides": {"source": "dummy-token"},
				"requires": {"sink": "dummy-token"}},
			UsedBy: []string{
				"wordpress-everlast",
				"wordpress-jorge",
				"wordpress-site",
			},
		},
	}, nil)
}

func (s *infoSuite) expectModelConfig(c *gc.C, series string) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"default-series": series,
		"type":           "my-type",
		"name":           "my-name",
		"uuid":           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil)
}
