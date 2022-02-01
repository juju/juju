// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
)

type infoSuite struct {
	testing.IsolationSuite

	charmHubAPI *mocks.MockCharmHubClient
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
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", "focal", "--format", "json"})
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
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (CharmHubClient, error) {
			return s.charmHubAPI, nil
		},
	}
}

func (s *infoSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmHubAPI = mocks.NewMockCharmHubClient(ctrl)
	return ctrl
}

func (s *infoSuite) expectInfo() {
	s.charmHubAPI.EXPECT().Info(gomock.Any(), "test", gomock.Any()).Return(transport.InfoResponse{
		Name: "wordpress",
		Type: "charm",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		Entity: transport.Entity{
			Description: "This will install and setup WordPress optimized to run in the cloud.",
			Publisher:   map[string]string{"display-name": "Wordress Charmers"},
			Summary:     "WordPress is a full featured web blogging tool, this charm deploys it.",
			StoreURL:    "https://someurl.com/wordpress",
			Categories: []transport.Category{{
				Name: "app",
			}, {
				Name: "seven",
			}},
			UsedBy: []string{
				"wordpress-everlast",
				"wordpress-jorge",
				"wordpress-site",
			},
		},
		DefaultRelease: transport.InfoChannelMap{
			Revision: transport.InfoRevision{
				MetadataYAML: entityMeta,
				ConfigYAML:   entityConfig,
			},
		},
		ChannelMap: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 16,
				Version:  "1.0.3",
				Download: transport.Download{
					Size: 12042240,
				},
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "amd64",
				}, {
					Name:         "ubuntu",
					Channel:      "16.04",
					Architecture: "amd64",
				}},
			},
		}},
	}, nil)
}

var entityMeta = `
name: myname
version: 1.0.3
subordinate: false
summary: A charm or bundle.
description: |
  This will install and setup services optimized to run in the cloud.
  By default it will place Ngnix configured to scale horizontally
  with Nginx's reverse proxy.
series: [bionic, xenial]
provides:
  source:
    interface: dummy-token
requires:
  sink:
    interface: dummy-token
`

var entityConfig = `
options:
  title:
    default: My Title
    description: A descriptive title used for the application.
    type: string
  subtitle:
    default: ""
    description: An optional subtitle used for the application.
  outlook:
    description: No default outlook.
    # type defaults to string in python
  username:
    default: admin001
    description: The name of the initial account (given admin permissions).
    type: string
  skill-level:
    description: A number indicating skill.
    type: int
  agility-ratio:
    description: A number from 0 to 1 indicating agility.
    type: float
  reticulate-splines:
    description: Whether to reticulate splines on launch, or not.
    type: boolean
`
