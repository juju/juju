// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/client/charms/mocks"
	"github.com/juju/juju/charmhub/transport"
)

type charmHubRepositoriesSuite struct {
	client *mocks.MockCharmHubClient
}

var _ = gc.Suite(&charmHubRepositoriesSuite{})

func (s *charmHubRepositoriesSuite) TestResolveDefaultChannelMap(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub"}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 16
	origin.Revision = &curl.Revision
	origin.Risk = "stable"
	track := "latest"
	origin.Track = &track
	c.Assert(obtainedCurl, jc.DeepEquals, curl)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "xenial"})
}

func (s *charmHubRepositoriesSuite) TestResolveWithRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress-13")
	origin := params.CharmOrigin{Source: "charm-hub"}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 13
	origin.Revision = &curl.Revision
	origin.Risk = "stable"
	track := "second"
	origin.Track = &track
	c.Assert(obtainedCurl, jc.DeepEquals, curl)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "xenial"})
}

func (s *charmHubRepositoriesSuite) TestResolveWithRevisionNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress-42")
	origin := params.CharmOrigin{Source: "charm-hub"}

	resolver := &chRepo{client: s.client}
	_, _, _, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *charmHubRepositoriesSuite) TestResolveWithChannel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress")
	track := "latest"
	origin := params.CharmOrigin{Source: "charm-hub", Risk: "edge", Track: &track}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 19
	origin.Revision = &curl.Revision
	c.Assert(obtainedCurl, jc.DeepEquals, curl)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "xenial"})
}

func (s *charmHubRepositoriesSuite) TestResolveWithChannelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress")
	track := "testme"
	origin := params.CharmOrigin{Source: "charm-hub", Risk: "edge", Track: &track}

	resolver := &chRepo{client: s.client}
	_, _, _, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *charmHubRepositoriesSuite) TestResolveWithChannelRiskOnly(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(nil)

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub", Risk: "candidate"}

	resolver := &chRepo{client: s.client}
	obtainedCurl, obtainedOrigin, obtainedSeries, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.ErrorIsNil)

	curl.Revision = 19
	origin.Revision = &curl.Revision
	track := "latest"
	origin.Track = &track
	c.Assert(obtainedCurl, jc.DeepEquals, curl)
	c.Assert(obtainedOrigin, jc.DeepEquals, origin)
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "xenial"})
}

func (s *charmHubRepositoriesSuite) TestResolveInfoError(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectInfo(errors.NotSupportedf("error for test"))

	curl := charm.MustParseURL("ch:wordpress")
	origin := params.CharmOrigin{Source: "charm-hub"}

	resolver := &chRepo{client: s.client}
	_, _, _, err := resolver.ResolveWithPreferredChannel(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *charmHubRepositoriesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockCharmHubClient(ctrl)
	return ctrl
}

func (s *charmHubRepositoriesSuite) expectInfo(err error) {
	s.client.EXPECT().Info(gomock.Any(), "wordpress").Return(getCharmHubInfoResponse(), err)
}

type charmStoreResolversSuite struct {
	repo CSRepository
}

var _ = gc.Suite(&charmStoreResolversSuite{})

func (s *charmStoreResolversSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.repo = mocks.NewMockCSRepository(ctrl)
	return ctrl
}

func (s *charmStoreResolversSuite) expectResolveWithPreferredChannel(c *gc.C) {

}

func getCharmHubInfoResponse() transport.InfoResponse {
	channelMap, defaultRelease := getCharmHubResponse()
	return transport.InfoResponse{
		Name:           "wordpress",
		Type:           "charm",
		ID:             "charmCHARMcharmCHARMcharmCHARM01",
		ChannelMap:     channelMap,
		DefaultRelease: defaultRelease,
	}
}

func getCharmHubResponse() ([]transport.ChannelMap, transport.ChannelMap) {
	return []transport.ChannelMap{{
			Channel: transport.Channel{
				Name: "latest/stable",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.Revision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 18,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "latest/candidate",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "candidate",
				Track: "latest",
			},
			Revision: transport.Revision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "latest/edge",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "edge",
				Track: "latest",
			},
			Revision: transport.Revision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 19,
				Version:  "1.0.3",
			},
		}, {
			Channel: transport.Channel{
				Name: "second/stable",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "second",
			},
			Revision: transport.Revision{
				MetadataYAML: entityMeta,
				Platforms: []transport.Platform{{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				}},
				Revision: 13,
				Version:  "1.0.3",
			},
		}}, transport.ChannelMap{
			Channel: transport.Channel{
				Name: "latest/stable",
				Platform: transport.Platform{
					Architecture: "all",
					OS:           "ubuntu",
					Series:       "bionic",
				},
				Risk:  "stable",
				Track: "latest",
			},
			Revision: transport.Revision{
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
