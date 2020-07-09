// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
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
	resultSource := params.CharmHubEntityInfoResult{
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
	resultSource := params.CharmHubEntityFindResult{
		Results: getParamsFindResponses(),
	}
	s.facade.EXPECT().FacadeCall("Find", arg, gomock.Any()).SetArg(2, resultSource)

	obtained, err := s.newClientFromFacadeForTest().Find("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	assertFindResponsesSameContents(c, obtained, getFindResponses())
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
	return InfoResponse{
		Name:        "wordpress",
		Type:        "charm",
		ID:          "charmCHARMcharmCHARMcharmCHARM01",
		Description: "This will install and setup WordPress optimized to run in the cloud.\nBy default it will place Ngnix configured to scale horizontally\nwith Nginx's reverse proxy.",
		Publisher:   "Wordress Charmers",
		Summary:     "WordPress is a full featured web blogging tool, this charm deploys it.",
		Tracks:      []string{"latest"},
		Series:      []string{"bionic", "xenial"},
		StoreURL:    "https://someurl.com/wordpress",
		Tags:        []string{"app", "seven"},
		Channels: map[string]Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Size:       12042240,
				Revision:   16,
				Version:    "1.0.3",
			}},
		Charm: &Charm{
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
	}
}

func getFindResponses() []FindResponse {
	//_, entity, defaultChannelMap := getChannelMapResponse()
	return []FindResponse{{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		//Entity:         entity,
		//DefaultRelease: defaultChannelMap,
	}}
}

func assertInfoResponseSameContents(c *gc.C, obtained, expected InfoResponse) {
	c.Assert(obtained.Type, gc.Equals, expected.Type)
	c.Assert(obtained.ID, gc.Equals, expected.ID)
	c.Assert(obtained.Name, gc.Equals, expected.Name)
	c.Assert(obtained.Description, gc.Equals, expected.Description)
	c.Assert(obtained.Publisher, jc.DeepEquals, expected.Publisher)
	c.Assert(obtained.Summary, gc.Equals, expected.Summary)
	c.Assert(obtained.Tags, gc.DeepEquals, expected.Tags)
	c.Assert(obtained.Channels, jc.DeepEquals, expected.Channels)
	c.Assert(obtained.Tracks, jc.SameContents, expected.Tracks)
	assertCharmSameContents(c, obtained.Charm, expected.Charm)
}

func assertCharmSameContents(c *gc.C, obtained, expected *Charm) {
	c.Assert(obtained.Config, gc.DeepEquals, expected.Config)
	c.Assert(obtained.Relations, jc.DeepEquals, expected.Relations)
	c.Assert(obtained.Subordinate, gc.Equals, expected.Subordinate)
	c.Assert(obtained.UsedBy, gc.DeepEquals, expected.UsedBy)
}

func assertFindResponsesSameContents(c *gc.C, obtained, expected []FindResponse) {
	c.Assert(obtained, gc.HasLen, 1)
	c.Assert(expected, gc.HasLen, 1)
	want := obtained[0]
	got := expected[0]
	c.Assert(want.Type, gc.Equals, got.Type)
	c.Assert(want.ID, gc.Equals, got.ID)
	c.Assert(want.Name, gc.Equals, got.Name)
	//assertEntitySameContents(c, want.Entity, got.Entity)
	c.Assert(want.DefaultRelease, gc.DeepEquals, got.DefaultRelease)
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
		Tags:        []string{"app", "seven"},
		Channels: map[string]params.Channel{
			"latest/stable": {
				ReleasedAt: "2019-12-16T19:44:44.076943+00:00",
				Track:      "latest",
				Risk:       "stable",
				Size:       12042240,
				Revision:   16,
				Version:    "1.0.3",
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

func getParamsFindResponses() []params.FindResponse {
	//_, entity, defaultChannelMap := getParamsChannelMapResponse()
	return []params.FindResponse{{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		//Entity:         entity,
		//DefaultRelease: defaultChannelMap,
	}}
}
