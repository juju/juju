// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"

	"github.com/juju/juju/api/charmhub"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type printInfoSuite struct{}

var _ = gc.Suite(&printInfoSuite{})

func (s *printInfoSuite) TestCharmPrintInfo(c *gc.C) {
	ir := getCharmInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx, &ir)
	err := iw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: wordpress
charm-id: charmCHARMcharmCHARMcharmCHARM01
summary: WordPress is a full featured web blogging tool, this charm deploys it.
publisher: Wordress Charmers
supports: bionic, xenial
tags: app, seven
subordinate: false
description: This will install and setup WordPress optimized to run in the cloud.
  By default it will place Ngnix and php-fpm configured to scale horizontally with
  Nginx's reverse proxy.
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
`
	c.Assert(obtained, gc.Equals, expected)
}

func (s *printInfoSuite) TestBundlePrintInfo(c *gc.C) {
	ir := getBundleInfoResponse()
	ctx := commandContextForTest(c)
	iw := makeInfoWriter(ctx, &ir)
	err := iw.Print()
	c.Assert(err, jc.ErrorIsNil)

	obtained := ctx.Stdout.(*bytes.Buffer).String()
	expected := `name: osm
bundle-id: bundleBUNDLEbundleBUNDLEbundle01
summary: A bundle by charmed-osm.
publisher: charmed-osm
description: Single instance OSM bundle.
channels: |
  latest/stable:     1.0.3  2019-12-16  (16)  12MB
  latest/candidate:  1.0.3  2019-12-16  (17)  12MB
  latest/beta:       1.0.3  2019-12-16  (17)  12MB
  latest/edge:       1.0.3  2019-12-16  (18)  12MB
`
	c.Assert(obtained, gc.Equals, expected)
}

func getBundleInfoResponse() charmhub.InfoResponse {
	return charmhub.InfoResponse{
		Name: "osm",
		Type: "bundle",
		ID:   "bundleBUNDLEbundleBUNDLEbundle01",
		ChannelMap: []charmhub.ChannelMap{
			{
				Channel: charmhub.Channel{
					Name:       "latest/stable",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					CreatedAt: "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					Revision: 16,
					Version:  "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/candidate",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					ConfigYAML: config,
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Revision:     17,
					Version:      "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/beta",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					CreatedAt: "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					Revision: 17,
					Version:  "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/edge",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					CreatedAt: "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					Revision: 18,
					Version:  "1.0.3",
				},
			}},
		Charm: charmhub.Charm{
			Categories: []charmhub.Category{{
				Featured: true,
				Name:     "blog",
			}},
			Summary: "A bundle by charmed-osm.",
			Publisher: map[string]string{
				"display-name": "charmed-osm",
			},
			Description: "Single instance OSM bundle.",
		},
		DefaultRelease: charmhub.ChannelMap{
			Channel: charmhub.Channel{
				Name: "latest/stable",
				Platform: charmhub.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: charmhub.Revision{
				CreatedAt: "2019-12-16T19:20:26.673192+00:00",
				Download: charmhub.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				Revision: 16,
				Version:  "1.0.3",
			},
		},
	}
}

func getCharmInfoResponse() charmhub.InfoResponse {
	return charmhub.InfoResponse{
		Name: "wordpress",
		Type: "charm",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap: []charmhub.ChannelMap{
			{
				Channel: charmhub.Channel{
					Name:       "latest/stable",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					ConfigYAML: config,
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Revision:     16,
					Version:      "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/candidate",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					ConfigYAML: config,
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Revision:     17,
					Version:      "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/beta",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					ConfigYAML: config,
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Revision:     17,
					Version:      "1.0.3",
				},
			}, {
				Channel: charmhub.Channel{
					Name:       "latest/edge",
					ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				},
				Revision: charmhub.Revision{
					ConfigYAML: config,
					CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
					Download: charmhub.Download{
						HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
						Size:       12042240,
						URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
					},
					MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
					Revision:     18,
					Version:      "1.0.3",
				},
			}},
		Charm: charmhub.Charm{
			Categories: []charmhub.Category{{
				Featured: true,
				Name:     "blog",
			}},
			Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
			Publisher: map[string]string{
				"display-name": "Wordress Charmers",
			},
			Summary: "WordPress is a full featured web blogging tool, this charm deploys it.",
		},
		DefaultRelease: charmhub.ChannelMap{
			Channel: charmhub.Channel{
				Name: "latest/stable",
				Platform: charmhub.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: charmhub.Revision{
				ConfigYAML: config,
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: charmhub.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsubordinate: false\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\ntags: [app, seven]\nseries: [bionic, xenial]\n",
				Revision:     16,
				Version:      "1.0.3",
			},
		},
	}
}

var config = `
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
