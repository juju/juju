// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package repository

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/charm/repository/mocks"
)

type charmStoreRepositorySuite struct {
	testing.IsolationSuite
	client *mocks.MockCharmStoreClient
	logger *mocks.MockLogger
}

var _ = gc.Suite(&charmStoreRepositorySuite{})

func (s *charmStoreRepositorySuite) TestResolveCharmWithChannelAndMacaroons(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:wordpress")
	resolvedCurl := charm.MustParseURL("cs:wordpress")
	resolvedCurl.Series = "charm"
	requestedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "edge"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Type:    "charm",
		Channel: &charm.Channel{Risk: "stable"},
	}
	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

	repo := NewCharmStoreRepository(s.logger, "store-api-endpoint")
	repo.clientFactory = func(gotStoreURL string, gotChannel csparams.Channel, gotMacaroons macaroon.Slice) (CharmStoreClient, error) {
		c.Assert(gotStoreURL, gc.Equals, "store-api-endpoint", gc.Commentf("the provided store API endpoint was not passed to the client factory"))
		c.Assert(gotChannel, gc.Equals, csparams.Channel("edge"), gc.Commentf("the channel from the provided origin was not passed to the client factory"))
		c.Assert(gotMacaroons, gc.DeepEquals, macaroons, gc.Commentf("the provided macaroons were not passed to the client factory"))
		return s.client, nil
	}
	s.client.EXPECT().ResolveWithPreferredChannel(curl, csparams.Channel("edge")).Return(curl, csparams.Channel("stable"), []string{"focal"}, nil)

	gotCurl, gotOrigin, gotSeries, err := repo.ResolveWithPreferredChannel(curl, requestedOrigin, macaroons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotCurl, jc.DeepEquals, curl)
	c.Assert(gotOrigin, jc.DeepEquals, resolvedOrigin)
	c.Assert(gotSeries, jc.SameContents, []string{"focal"})
}

func (s *charmStoreRepositorySuite) TestResolveBundleWithChannelAndMacaroons(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:core-kubernetes")
	resolvedCurl := charm.MustParseURL("cs:core-kubernetes")
	resolvedCurl.Series = "bundle"
	requestedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "edge"},
	}
	resolvedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Type:    "charm",
		Channel: &charm.Channel{Risk: "stable"},
	}
	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

	repo := NewCharmStoreRepository(s.logger, "store-api-endpoint")
	repo.clientFactory = func(gotStoreURL string, gotChannel csparams.Channel, gotMacaroons macaroon.Slice) (CharmStoreClient, error) {
		c.Assert(gotStoreURL, gc.Equals, "store-api-endpoint", gc.Commentf("the provided store API endpoint was not passed to the client factory"))
		c.Assert(gotChannel, gc.Equals, csparams.Channel("edge"), gc.Commentf("the channel from the provided origin was not passed to the client factory"))
		c.Assert(gotMacaroons, gc.DeepEquals, macaroons, gc.Commentf("the provided macaroons were not passed to the client factory"))
		return s.client, nil
	}
	s.client.EXPECT().ResolveWithPreferredChannel(curl, csparams.Channel("edge")).Return(curl, csparams.Channel("stable"), []string{"focal"}, nil)

	gotCurl, gotOrigin, gotSeries, err := repo.ResolveWithPreferredChannel(curl, requestedOrigin, macaroons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotCurl, jc.DeepEquals, curl)
	c.Assert(gotOrigin, jc.DeepEquals, resolvedOrigin)
	c.Assert(gotSeries, jc.SameContents, []string{"focal"})
}

func (s *charmStoreRepositorySuite) TestDownloadCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:ubuntu-lite")
	resolvedCurl := charm.MustParseURL("cs:ubuntu-lite")
	resolvedCurl.Series = "charm"
	requestedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "edge"},
	}
	resolvedOrigin := corecharm.Origin{
		Type:    "charm",
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "stable"},
	}
	resolvedArchive := new(charm.CharmArchive)
	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

	repo := NewCharmStoreRepository(s.logger, "store-api-endpoint")
	repo.clientFactory = func(gotStoreURL string, gotChannel csparams.Channel, gotMacaroons macaroon.Slice) (CharmStoreClient, error) {
		c.Assert(gotStoreURL, gc.Equals, "store-api-endpoint", gc.Commentf("the provided store API endpoint was not passed to the client factory"))
		c.Assert(gotChannel, gc.Equals, csparams.Channel("edge"), gc.Commentf("the channel from the provided origin was not passed to the client factory"))
		c.Assert(gotMacaroons, gc.DeepEquals, macaroons, gc.Commentf("the provided macaroons were not passed to the client factory"))
		return s.client, nil
	}
	s.client.EXPECT().ResolveWithPreferredChannel(curl, csparams.Channel("edge")).Return(resolvedCurl, csparams.Channel("stable"), []string{"focal"}, nil)
	s.client.EXPECT().Get(resolvedCurl, "/tmp/foo").Return(resolvedArchive, nil)

	gotArchive, gotOrigin, err := repo.DownloadCharm(curl, requestedOrigin, macaroons, "/tmp/foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotArchive, gc.Equals, resolvedArchive) // note: we are using gc.Equals to check the pointers here.
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *charmStoreRepositorySuite) TestGetDownloadURL(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("cs:ubuntu-lite")
	resolvedCurl := charm.MustParseURL("cs:ubuntu-lite")
	resolvedCurl.Series = "charm"
	requestedOrigin := corecharm.Origin{
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "edge"},
	}
	resolvedOrigin := corecharm.Origin{
		Type:    "charm",
		Source:  "charm-store",
		Channel: &charm.Channel{Risk: "stable"},
	}
	mac, err := macaroon.New(nil, []byte("id"), "", macaroon.LatestVersion)
	c.Assert(err, jc.ErrorIsNil)
	macaroons := macaroon.Slice{mac}

	repo := NewCharmStoreRepository(s.logger, "store-api-endpoint")
	repo.clientFactory = func(gotStoreURL string, gotChannel csparams.Channel, gotMacaroons macaroon.Slice) (CharmStoreClient, error) {
		c.Assert(gotStoreURL, gc.Equals, "store-api-endpoint", gc.Commentf("the provided store API endpoint was not passed to the client factory"))
		c.Assert(gotChannel, gc.Equals, csparams.Channel("edge"), gc.Commentf("the channel from the provided origin was not passed to the client factory"))
		c.Assert(gotMacaroons, gc.DeepEquals, macaroons, gc.Commentf("the provided macaroons were not passed to the client factory"))
		return s.client, nil
	}
	s.client.EXPECT().ResolveWithPreferredChannel(curl, csparams.Channel("edge")).Return(resolvedCurl, csparams.Channel("stable"), []string{"focal"}, nil)

	gotURL, gotOrigin, err := repo.GetDownloadURL(curl, requestedOrigin, macaroons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL.String(), gc.Equals, "cs:charm/ubuntu-lite")
	c.Assert(gotOrigin, gc.DeepEquals, resolvedOrigin)
}

func (s *charmStoreRepositorySuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = mocks.NewMockCharmStoreClient(ctrl)
	s.logger = mocks.NewMockLogger(ctrl)
	s.logger.EXPECT().Tracef(gomock.Any(), gomock.Any()).AnyTimes()
	return ctrl
}
