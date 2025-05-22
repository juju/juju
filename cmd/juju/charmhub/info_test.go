// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testhelpers"
)

type infoSuite struct {
	testhelpers.IsolationSuite

	charmHubAPI *mocks.MockCharmHubClient
}

func TestInfoSuite(t *testing.T) {
	tc.Run(t, &infoSuite{})
}

func (s *infoSuite) TestInitNoArgs(c *tc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
	}
	err := command.Init([]string{})
	c.Assert(err, tc.NotNil)
}

func (s *infoSuite) TestInitSuccess(c *tc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
	}
	err := command.Init([]string{"test"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *infoSuite) TestInitFailCS(c *tc.C) {
	command := &infoCommand{
		charmHubCommand: &charmHubCommand{},
	}
	err := command.Init([]string{"cs:test"})
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *infoSuite) TestRun(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *infoSuite) TestRunJSON(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indentJSON(c, cmdtesting.Stdout(ctx)), tc.Equals, `
{
  "type": "charm",
  "id": "charmCHARMcharmCHARMcharmCHARM01",
  "name": "wordpress",
  "description": "This will install and setup WordPress optimized to run in the cloud.",
  "publisher": "WordPress Charmers",
  "summary": "WordPress is a full featured web blogging tool, this charm deploys it.",
  "supports": [
    {
      "name": "ubuntu",
      "channel": "18.04"
    },
    {
      "name": "ubuntu",
      "channel": "16.04"
    }
  ],
  "store-url": "https://someurl.com/wordpress",
  "tags": [
    "app",
    "seven"
  ],
  "charm": {
    "config": {
      "Options": {
        "agility-ratio": {
          "Type": "float",
          "Description": "A number from 0 to 1 indicating agility.",
          "Default": null
        },
        "outlook": {
          "Type": "string",
          "Description": "No default outlook.",
          "Default": null
        },
        "reticulate-splines": {
          "Type": "boolean",
          "Description": "Whether to reticulate splines on launch, or not.",
          "Default": null
        },
        "skill-level": {
          "Type": "int",
          "Description": "A number indicating skill.",
          "Default": null
        },
        "subtitle": {
          "Type": "string",
          "Description": "An optional subtitle used for the application.",
          "Default": ""
        },
        "title": {
          "Type": "string",
          "Description": "A descriptive title used for the application.",
          "Default": "My Title"
        },
        "username": {
          "Type": "string",
          "Description": "The name of the initial account (given admin permissions).",
          "Default": "admin001"
        }
      }
    },
    "relations": {
      "provides": {
        "source": "dummy-token"
      },
      "requires": {
        "sink": "dummy-token"
      }
    },
    "used-by": [
      "wordpress-everlast",
      "wordpress-jorge",
      "wordpress-site"
    ]
  },
  "channels": {
    "latest": {
      "stable": [
        {
          "track": "latest",
          "risk": "stable",
          "version": "1.0.3",
          "revision": 16,
          "released-at": "2019-12-16T19:44:44.076943+00:00",
          "size": 12042240,
          "architectures": [
            "amd64"
          ],
          "bases": [
            {
              "name": "ubuntu",
              "channel": "18.04"
            },
            {
              "name": "ubuntu",
              "channel": "16.04"
            }
          ]
        }
      ]
    }
  },
  "tracks": [
    "latest"
  ]
}
`[1:])
}

func indentJSON(c *tc.C, input string) string {
	var buf bytes.Buffer
	err := json.Indent(&buf, []byte(input), "", "  ")
	c.Assert(err, tc.IsNil)
	return buf.String()
}

func (s *infoSuite) TestRunJSONSpecifySeriesNotDefault(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json", "--base", "ubuntu@16.04"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indentJSON(c, cmdtesting.Stdout(ctx)), tc.Equals, `
{
  "type": "charm",
  "id": "charmCHARMcharmCHARMcharmCHARM01",
  "name": "wordpress",
  "description": "This will install and setup WordPress optimized to run in the cloud.",
  "publisher": "WordPress Charmers",
  "summary": "WordPress is a full featured web blogging tool, this charm deploys it.",
  "supports": [
    {
      "name": "ubuntu",
      "channel": "18.04"
    },
    {
      "name": "ubuntu",
      "channel": "16.04"
    }
  ],
  "store-url": "https://someurl.com/wordpress",
  "tags": [
    "app",
    "seven"
  ],
  "charm": {
    "config": {
      "Options": {
        "agility-ratio": {
          "Type": "float",
          "Description": "A number from 0 to 1 indicating agility.",
          "Default": null
        },
        "outlook": {
          "Type": "string",
          "Description": "No default outlook.",
          "Default": null
        },
        "reticulate-splines": {
          "Type": "boolean",
          "Description": "Whether to reticulate splines on launch, or not.",
          "Default": null
        },
        "skill-level": {
          "Type": "int",
          "Description": "A number indicating skill.",
          "Default": null
        },
        "subtitle": {
          "Type": "string",
          "Description": "An optional subtitle used for the application.",
          "Default": ""
        },
        "title": {
          "Type": "string",
          "Description": "A descriptive title used for the application.",
          "Default": "My Title"
        },
        "username": {
          "Type": "string",
          "Description": "The name of the initial account (given admin permissions).",
          "Default": "admin001"
        }
      }
    },
    "relations": {
      "provides": {
        "source": "dummy-token"
      },
      "requires": {
        "sink": "dummy-token"
      }
    },
    "used-by": [
      "wordpress-everlast",
      "wordpress-jorge",
      "wordpress-site"
    ]
  },
  "channels": {
    "latest": {
      "stable": [
        {
          "track": "latest",
          "risk": "stable",
          "version": "1.0.3",
          "revision": 16,
          "released-at": "2019-12-16T19:44:44.076943+00:00",
          "size": 12042240,
          "architectures": [
            "amd64"
          ],
          "bases": [
            {
              "name": "ubuntu",
              "channel": "18.04"
            },
            {
              "name": "ubuntu",
              "channel": "16.04"
            }
          ]
        }
      ]
    }
  },
  "tracks": [
    "latest"
  ]
}
`[1:])
}

func (s *infoSuite) TestRunJSONSpecifyArch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json", "--arch", "amd64"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indentJSON(c, cmdtesting.Stdout(ctx)), tc.Equals, `
{
  "type": "charm",
  "id": "charmCHARMcharmCHARMcharmCHARM01",
  "name": "wordpress",
  "description": "This will install and setup WordPress optimized to run in the cloud.",
  "publisher": "WordPress Charmers",
  "summary": "WordPress is a full featured web blogging tool, this charm deploys it.",
  "supports": [
    {
      "name": "ubuntu",
      "channel": "18.04"
    },
    {
      "name": "ubuntu",
      "channel": "16.04"
    }
  ],
  "store-url": "https://someurl.com/wordpress",
  "tags": [
    "app",
    "seven"
  ],
  "charm": {
    "config": {
      "Options": {
        "agility-ratio": {
          "Type": "float",
          "Description": "A number from 0 to 1 indicating agility.",
          "Default": null
        },
        "outlook": {
          "Type": "string",
          "Description": "No default outlook.",
          "Default": null
        },
        "reticulate-splines": {
          "Type": "boolean",
          "Description": "Whether to reticulate splines on launch, or not.",
          "Default": null
        },
        "skill-level": {
          "Type": "int",
          "Description": "A number indicating skill.",
          "Default": null
        },
        "subtitle": {
          "Type": "string",
          "Description": "An optional subtitle used for the application.",
          "Default": ""
        },
        "title": {
          "Type": "string",
          "Description": "A descriptive title used for the application.",
          "Default": "My Title"
        },
        "username": {
          "Type": "string",
          "Description": "The name of the initial account (given admin permissions).",
          "Default": "admin001"
        }
      }
    },
    "relations": {
      "provides": {
        "source": "dummy-token"
      },
      "requires": {
        "sink": "dummy-token"
      }
    },
    "used-by": [
      "wordpress-everlast",
      "wordpress-jorge",
      "wordpress-site"
    ]
  },
  "channels": {
    "latest": {
      "stable": [
        {
          "track": "latest",
          "risk": "stable",
          "version": "1.0.3",
          "revision": 16,
          "released-at": "2019-12-16T19:44:44.076943+00:00",
          "size": 12042240,
          "architectures": [
            "amd64"
          ],
          "bases": [
            {
              "name": "ubuntu",
              "channel": "18.04"
            },
            {
              "name": "ubuntu",
              "channel": "16.04"
            }
          ]
        }
      ]
    }
  },
  "tracks": [
    "latest"
  ]
}
`[1:])
}

func (s *infoSuite) TestRunJSONWithSeriesFoundChannel(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--base", "ubuntu@20.04", "--format", "json"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indentJSON(c, cmdtesting.Stdout(ctx)), tc.Equals, `
{
  "type": "charm",
  "id": "charmCHARMcharmCHARMcharmCHARM01",
  "name": "wordpress",
  "description": "This will install and setup WordPress optimized to run in the cloud.",
  "publisher": "WordPress Charmers",
  "summary": "WordPress is a full featured web blogging tool, this charm deploys it.",
  "supports": [
    {
      "name": "ubuntu",
      "channel": "18.04"
    },
    {
      "name": "ubuntu",
      "channel": "16.04"
    }
  ],
  "store-url": "https://someurl.com/wordpress",
  "tags": [
    "app",
    "seven"
  ],
  "charm": {
    "config": {
      "Options": {
        "agility-ratio": {
          "Type": "float",
          "Description": "A number from 0 to 1 indicating agility.",
          "Default": null
        },
        "outlook": {
          "Type": "string",
          "Description": "No default outlook.",
          "Default": null
        },
        "reticulate-splines": {
          "Type": "boolean",
          "Description": "Whether to reticulate splines on launch, or not.",
          "Default": null
        },
        "skill-level": {
          "Type": "int",
          "Description": "A number indicating skill.",
          "Default": null
        },
        "subtitle": {
          "Type": "string",
          "Description": "An optional subtitle used for the application.",
          "Default": ""
        },
        "title": {
          "Type": "string",
          "Description": "A descriptive title used for the application.",
          "Default": "My Title"
        },
        "username": {
          "Type": "string",
          "Description": "The name of the initial account (given admin permissions).",
          "Default": "admin001"
        }
      }
    },
    "relations": {
      "provides": {
        "source": "dummy-token"
      },
      "requires": {
        "sink": "dummy-token"
      }
    },
    "used-by": [
      "wordpress-everlast",
      "wordpress-jorge",
      "wordpress-site"
    ]
  },
  "channels": {},
  "tracks": [
    "latest"
  ]
}
`[1:])
}

func (s *infoSuite) TestRunYAML(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectInfo()

	command := &infoCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "yaml"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
type: charm
id: charmCHARMcharmCHARMcharmCHARM01
name: wordpress
description: This will install and setup WordPress optimized to run in the cloud.
publisher: WordPress Charmers
summary: WordPress is a full featured web blogging tool, this charm deploys it.
supports:
- name: ubuntu
  channel: "18.04"
- name: ubuntu
  channel: "16.04"
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
channels:
  latest:
    stable:
    - track: latest
      risk: stable
      version: 1.0.3
      revision: 16
      released-at: "2019-12-16T19:44:44.076943+00:00"
      size: 12042240
      architectures:
      - amd64
      bases:
      - name: ubuntu
        channel: "18.04"
      - name: ubuntu
        channel: "16.04"
tracks:
- latest
`[1:])
}

func (s *infoSuite) newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		CharmHubClientFunc: func(charmhub.Config) (CharmHubClient, error) {
			return s.charmHubAPI, nil
		},
	}
}

func (s *infoSuite) setUpMocks(c *tc.C) *gomock.Controller {
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
			Publisher:   map[string]string{"display-name": "WordPress Charmers"},
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
				ConfigYAML:   entityConfig,
				MetadataYAML: entityMeta,
				Bases: []transport.Base{
					{Name: "ubuntu", Channel: "18.04"},
					{Name: "ubuntu", Channel: "16.04"},
				},
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
