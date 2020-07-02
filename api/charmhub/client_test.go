// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/apiserver/params"
)

type charmHubSuite struct {
	client *mocks.MockClientFacade
	facade *mocks.MockFacadeCaller
}

var _ = gc.Suite(&charmHubSuite{})

func (s charmHubSuite) TestInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.Entity{Tag: names.NewApplicationTag("wordpress").String()}
	resultSource := params.CharmHubCharmInfoResult{
		Result: getParamsInfoResponse(),
	}
	s.facade.EXPECT().FacadeCall("Info", arg, gomock.Any()).SetArg(2, resultSource)

	obtained, err := s.newClientFromFacadeForTest().Info("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	assertInfoResponseSameContents(c, obtained, getInfoResponse())
}

func (s charmHubSuite) TestFind(c *gc.C) {
	defer s.setupMocks(c).Finish()

	arg := params.Query{Query: "wordpress"}
	resultSource := params.CharmHubCharmFindResult{
		Result: getParamsFindResponse(),
	}
	s.facade.EXPECT().FacadeCall("Find", arg, gomock.Any()).SetArg(2, resultSource)

	obtained, err := s.newClientFromFacadeForTest().Find("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	assertFindResponseSameContents(c, obtained, getFindResponse())
}

func (s *charmHubSuite) newClientFromFacadeForTest() *Client {
	return newClientFromFacade(s.client, s.facade)
}

func (s *charmHubSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.client = mocks.NewMockClientFacade(ctrl)
	s.facade = mocks.NewMockFacadeCaller(ctrl)

	return ctrl
}

func getInfoResponse() InfoResponse {
	channelMaps, charm, defaultChannelMap := getChannelMapResponse()
	return InfoResponse{
		Name:           "wordpress",
		Type:           "object",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMaps,
		Charm:          charm,
		DefaultRelease: defaultChannelMap,
	}
}

func getFindResponse() FindResponse {
	channelMaps, charm, defaultChannelMap := getChannelMapResponse()
	return FindResponse{
		Name:           "wordpress",
		Type:           "object",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMaps,
		Charm:          charm,
		DefaultRelease: defaultChannelMap,
	}
}

func getChannelMapResponse() ([]ChannelMap, Charm, ChannelMap) {
	return []ChannelMap{{
			Channel: Channel{
				Name: "latest/stable",
				Platform: Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}}, Charm{
			Categories: []Category{{
				Featured: true,
				Name:     "blog",
			}},
			Description: "This will install and setup WordPress optimized to run in the cloud. By default it will place Ngnix and php-fpm configured to scale horizontally with Nginx's reverse proxy.",
			License:     "Apache-2.0",
			Media: []Media{{
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
		}, ChannelMap{
			Channel: Channel{
				Name: "latest/stable",
				Platform: Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
			},
			Revision: Revision{
				ConfigYAML: "one: 1\ntwo: 2\nitems: [1,2,3,4]\n\"",
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: Download{
					HashSHA265: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: "name: myname\nversion: 1.0.3\nsummary: A charm or bundle.\ndescription: |\n  This will install and setup services optimized to run in the cloud.\n  By default it will place Ngnix configured to scale horizontally\n  with Nginx's reverse proxy.\n",
				Platforms: []Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}
}

func assertInfoResponseSameContents(c *gc.C, obtained, expected InfoResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	assertCharmSameContents(c, obtained.Charm, expected.Charm)
	c.Assert(obtained.ChannelMap, gc.DeepEquals, expected.ChannelMap)
	c.Assert(obtained.DefaultRelease, gc.DeepEquals, expected.DefaultRelease)
}

func assertFindResponseSameContents(c *gc.C, obtained, expected FindResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	assertCharmSameContents(c, obtained.Charm, expected.Charm)
	c.Assert(obtained.ChannelMap, gc.DeepEquals, expected.ChannelMap)
	c.Assert(obtained.DefaultRelease, gc.DeepEquals, expected.DefaultRelease)
}

func assertCharmSameContents(c *gc.C, obtained, expected Charm) {
	c.Assert(obtained.Categories, gc.DeepEquals, expected.Categories)
	c.Assert(obtained.Description, gc.Equals, expected.Description)
	c.Assert(obtained.License, gc.Equals, expected.License)
	c.Assert(obtained.Media, gc.DeepEquals, expected.Media)
	c.Assert(obtained.Publisher, jc.DeepEquals, expected.Publisher)
	c.Assert(obtained.Summary, gc.Equals, expected.Summary)
	c.Assert(obtained.UsedBy, gc.DeepEquals, expected.UsedBy)
}

func getParamsInfoResponse() params.InfoResponse {
	channelMaps, charm, defaultChannelMap := getParamsChannelMapResponse()
	return params.InfoResponse{
		Name:           "wordpress",
		Type:           "object",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMaps,
		Charm:          charm,
		DefaultRelease: defaultChannelMap,
	}
}

func getParamsFindResponse() params.FindResponse {
	channelMaps, charm, defaultChannelMap := getParamsChannelMapResponse()
	return params.FindResponse{
		Name:           "wordpress",
		Type:           "object",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMaps,
		Charm:          charm,
		DefaultRelease: defaultChannelMap,
	}
}

func getParamsChannelMapResponse() ([]params.ChannelMap, params.CharmHubCharm, params.ChannelMap) {
	return []params.ChannelMap{{
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
		}}, params.CharmHubCharm{
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
		}, params.ChannelMap{
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
		}
}
