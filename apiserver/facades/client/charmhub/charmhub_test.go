// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/charmhub/mocks"
)

var _ = gc.Suite(&charmHubAPISuite{})

type charmHubAPISuite struct {
	authorizer *facademocks.MockAuthorizer
	client     *mocks.MockClient
}

func (s *charmHubAPISuite) TestInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo()
	arg := params.Entity{Tag: names.NewApplicationTag("wordpress").String()}
	obtained, err := s.newCharmHubAPIForTest(c).Info(arg)
	c.Assert(err, jc.ErrorIsNil)

	assertInfoResponseSameContents(c, obtained.Result, getParamsInfoResponse())
}

func (s *charmHubAPISuite) newCharmHubAPIForTest(c *gc.C) *CharmHubAPI {
	s.expectAuth()
	api, err := newCharmHubAPI(s.authorizer, s.client)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *charmHubAPISuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.client = mocks.NewMockClient(ctrl)
	return ctrl
}

func (s *charmHubAPISuite) expectAuth() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *charmHubAPISuite) expectInfo() {
	s.client.EXPECT().Info(gomock.Any(), "wordpress").Return(getCharmHubInfoResponse(), nil)
}

func assertInfoResponseSameContents(c *gc.C, obtained, expected params.InfoResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	assertCharmSameContents(c, obtained.Charm, expected.Charm)
	c.Assert(obtained.ChannelMap, gc.DeepEquals, expected.ChannelMap)
	c.Assert(obtained.DefaultRelease, gc.DeepEquals, expected.DefaultRelease)
}

func assertCharmSameContents(c *gc.C, obtained, expected params.CharmHubCharm) {
	c.Assert(obtained.Categories, gc.DeepEquals, expected.Categories)
	c.Assert(obtained.Description, gc.Equals, expected.Description)
	c.Assert(obtained.License, gc.Equals, expected.License)
	c.Assert(obtained.Media, gc.DeepEquals, expected.Media)
	c.Assert(obtained.Publisher, jc.DeepEquals, expected.Publisher)
	c.Assert(obtained.Summary, gc.Equals, expected.Summary)
	c.Assert(obtained.UsedBy, gc.DeepEquals, expected.UsedBy)
}

func getCharmHubInfoResponse() transport.InfoResponse {
	return transport.InfoResponse{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap: []transport.ChannelMap{{
			Channel: transport.Channel{
				Name: "latest/stable",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Risk:       "stable",
				Track:      "latest",
			},
			Revision: transport.Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: transport.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}},
		Charm: transport.Charm{
			Categories: []transport.Category{{
				Featured: true,
				Name:     "blog",
			}},
			Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
			License:     "Apache-2.0",
			Media: []transport.Media{{
				Height: 256,
				Type:   "icon",
				URL:    "https://dashboard.snapcraft.io/site_media/appmedia/2017/04/wpcom.png",
				Width:  256,
			}},
			Publisher: map[string]string{
				"display-name": "Wordress Charmers",
			},
			Summary: "WordPress is a full featured web blogging tool, this charm deploys it.",
			UsedBy: []string{
				"wordpress-everlast",
				"wordpress-jorge",
				"wordpress-site",
			},
		},
		DefaultRelease: transport.ChannelMap{
			Channel: transport.Channel{
				Name: "latest/stable",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Risk:       "stable",
				Track:      "latest",
			},
			Revision: transport.Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: transport.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		},
	}
}

func getParamsInfoResponse() params.InfoResponse {
	return params.InfoResponse{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap: []params.ChannelMap{{
			Channel: params.Channel{
				Name: "latest/stable",
				Platform: params.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: params.Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: params.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []params.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}},
		Charm: params.CharmHubCharm{
			Categories: []params.Category{{
				Featured: true,
				Name:     "blog",
			}},
			Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
			License:     "Apache-2.0",
			Media: []params.Media{{
				Height: 256,
				Type:   "icon",
				URL:    "https://dashboard.snapcraft.io/site_media/appmedia/2017/04/wpcom.png",
				Width:  256,
			}},
			Publisher: map[string]string{
				"display-name": "Wordress Charmers",
			},
			Summary: "WordPress is a full featured web blogging tool, this charm deploys it.",
			UsedBy: []string{
				"wordpress-everlast",
				"wordpress-jorge",
				"wordpress-site",
			},
		},
		DefaultRelease: params.ChannelMap{
			Channel: params.Channel{
				Name: "latest/stable",
				Platform: params.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: params.Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: params.Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []params.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		},
	}
}
