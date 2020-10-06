// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&charmHubAPISuite{})

type charmHubAPISuite struct {
	backend       *MockBackend
	authorizer    *facademocks.MockAuthorizer
	clientFactory *MockClientFactory
	client        *MockClient
}

func (s *charmHubAPISuite) TestInfo(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo()
	arg := params.Entity{Tag: names.NewApplicationTag("wordpress").String()}
	obtained, err := s.newCharmHubAPIForTest(c).Info(arg)
	c.Assert(err, jc.ErrorIsNil)

	assertInfoResponseSameContents(c, obtained.Result, getParamsInfoResponse())
}

func (s *charmHubAPISuite) TestFind(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectFind()
	arg := params.Query{Query: "wordpress"}
	obtained, err := s.newCharmHubAPIForTest(c).Find(arg)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(obtained.Results, gc.HasLen, 1)
	assertFindResponseSameContents(c, obtained.Results[0], getParamsFindResponse())
}

func (s *charmHubAPISuite) newCharmHubAPIForTest(c *gc.C) *CharmHubAPI {
	s.expectModelConfig(c)
	s.expectAuth()
	s.expectClient()
	api, err := newCharmHubAPI(s.backend, s.authorizer, s.clientFactory)
	c.Assert(err, jc.ErrorIsNil)
	return api
}

func (s *charmHubAPISuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.backend = NewMockBackend(ctrl)
	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.clientFactory = NewMockClientFactory(ctrl)
	s.client = NewMockClient(ctrl)
	return ctrl
}

func (s *charmHubAPISuite) expectModelConfig(c *gc.C) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"charm-hub-url": "https://someurl.com",
		"type":          "my-type",
		"name":          "my-name",
		"uuid":          testing.ModelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.EXPECT().ModelConfig().Return(cfg, nil)
}

func (s *charmHubAPISuite) expectAuth() {
	s.authorizer.EXPECT().AuthClient().Return(true)
}

func (s *charmHubAPISuite) expectClient() {
	s.clientFactory.EXPECT().Client("https://someurl.com").Return(s.client, nil)
}

func (s *charmHubAPISuite) expectInfo() {
	s.client.EXPECT().Info(gomock.Any(), "wordpress").Return(getCharmHubInfoResponse(), nil)
}

func (s *charmHubAPISuite) expectFind() {
	s.client.EXPECT().Find(gomock.Any(), "wordpress").Return(getCharmHubFindResponses(), nil)
}

func assertInfoResponseSameContents(c *gc.C, obtained, expected params.InfoResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	c.Assert(obtained.Publisher, jc.DeepEquals, expected.Publisher)
	c.Assert(obtained.Summary, gc.Equals, expected.Summary)
	c.Assert(obtained.Series, gc.DeepEquals, expected.Series)
	c.Assert(obtained.Channels, jc.DeepEquals, expected.Channels)
	c.Assert(obtained.Tracks, jc.SameContents, expected.Tracks)
	c.Assert(obtained.Tags, gc.DeepEquals, expected.Tags)
	c.Assert(obtained.StoreURL, gc.Equals, expected.StoreURL)
	assertCharmSameContents(c, obtained.Charm, expected.Charm)
}

func assertFindResponseSameContents(c *gc.C, obtained, expected params.FindResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	c.Assert(obtained.Publisher, gc.Equals, expected.Publisher)
	c.Assert(obtained.Summary, gc.Equals, expected.Summary)
	c.Assert(obtained.Version, gc.Equals, expected.Version)
	c.Assert(obtained.StoreURL, gc.Equals, expected.StoreURL)
	c.Assert(obtained.Series, gc.DeepEquals, expected.Series)
}

func assertCharmSameContents(c *gc.C, obtained, expected *params.CharmHubCharm) {
	c.Assert(obtained.Config, gc.DeepEquals, expected.Config)
	c.Assert(obtained.Relations, jc.DeepEquals, expected.Relations)
	c.Assert(obtained.Subordinate, gc.Equals, expected.Subordinate)
	c.Assert(obtained.UsedBy, gc.DeepEquals, expected.UsedBy)
}

func getCharmHubFindResponses() []transport.FindResponse {
	_, entity, defaultRelease := getCharmHubResponse()
	return []transport.FindResponse{{
		Name:           "wordpress",
		Type:           "object",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		Entity:         entity,
		DefaultRelease: defaultRelease,
	}}
}

func getCharmHubInfoResponse() transport.InfoResponse {
	channelMap, entity, defaultRelease := getCharmHubResponse()
	return transport.InfoResponse{
		Name:           "wordpress",
		Type:           "charm",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMap,
		Entity:         entity,
		DefaultRelease: defaultRelease,
	}
}

func getCharmHubResponse() ([]transport.ChannelMap, transport.Entity, transport.ChannelMap) {
	return []transport.ChannelMap{{
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
					HashSHA256: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
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
		}}, transport.Entity{
			Categories: []transport.Category{{
				Featured: true,
				Name:     "blog",
			}},
			License:     "Apache-2.0",
			Description: "This will install and setup Wordpress optimized to run in the cloud.\nBy default it will place Ngnix configured to scale horizontally\nwith Nginx's reverse proxy.",

			Media: []transport.Media{{
				Height: 256,
				Type:   "icon",
				URL:    "https://dashboard.snapcraft.io/site_media/appmedia/2017/04/wpcom.png",
				Width:  256,
			}},
			Publisher: map[string]string{
				"display-name": "Wordress Charmers",
			},
			Summary:  "WordPress is a full featured web blogging tool, this charm deploys it.",
			StoreURL: "https://someurl.com/wordpress",
			UsedBy: []string{
				"wordpress-everlast",
				"wordpress-jorge",
				"wordpress-site",
			},
		}, transport.ChannelMap{
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
				ConfigYAML: entityConfig,
				CreatedAt:  "2019-12-16T19:20:26.673192+00:00",
				Download: transport.Download{
					HashSHA256: "92a8b825ed1108ab64864a7df05eb84ed3925a8d5e4741169185f77cef9b52517ad4b79396bab43b19e544a908ec83c4",
					Size:       12042240,
					URL:        "https://api.snapcraft.io/api/v1/snaps/download/QLLfVfIKfcnTZiPFnmGcigB2vB605ZY7_16.snap",
				},
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 16,
				Version:  "1.0.3",
			},
		}
}

func getParamsInfoResponse() params.InfoResponse {
	return params.InfoResponse{
		Name:        "wordpress",
		Type:        "charm",
		ID:          "charmCHARMcharmCHARMcharmCHARM01",
		Description: "This will install and setup WordPress optimized to run in the cloud.\nBy default it will place Ngnix configured to scale horizontally\nwith Nginx's reverse proxy.",
		Publisher:   "Wordress Charmers",
		Summary:     "WordPress is a full featured web blogging tool, this charm deploys it.",
		Tracks:      []string{"latest"},
		Series:      []string{"bionic", "xenial"},
		StoreURL:    "https://someurl.com/wordpress",
		Tags:        []string{"blog"},
		Channels: map[string]params.Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Size:       12042240,
				Revision:   16,
				Version:    "1.0.3",
				Platforms:  []params.Platform{{Architecture: "all", OS: "ubuntu", Series: "bionic"}},
			}},
		Charm: &params.CharmHubCharm{
			Subordinate: false,
			Config: map[string]params.CharmOption{
				"reticulate-splines": {Type: "boolean", Description: "Whether to reticulate splines on launch, or not."},
				"title":              {Type: "string", Description: "A descriptive title used for the application.", Default: "My Title"},
				"subtitle":           {Type: "string", Description: "An optional subtitle used for the application.", Default: ""},
				"outlook":            {Type: "string", Description: "No default outlook."},
				"username":           {Type: "string", Description: "The name of the initial account (given admin permissions).", Default: "admin001"},
				"skill-level":        {Type: "int", Description: "A number indicating skill."},
				"agility-ratio":      {Type: "float", Description: "A number from 0 to 1 indicating agility."},
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
	}
}

func getParamsFindResponse() params.FindResponse {
	return params.FindResponse{
		Type:      "object",
		ID:        "charmCHARMcharmCHARMcharmCHARM01",
		Name:      "wordpress",
		Publisher: "Wordress Charmers",
		Summary:   "WordPress is a full featured web blogging tool, this charm deploys it.",
		Version:   "1.0.3",
		Series:    []string{"bionic", "xenial"},
		StoreURL:  "https://someurl.com/wordpress",
	}
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
