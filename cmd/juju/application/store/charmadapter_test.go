// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"net/url"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/core/series"
)

type resolveSuite struct {
	charmsAPI      *mocks.MockCharmsAPI
	downloadClient *mocks.MockDownloadBundleClient
	bundle         *mocks.MockBundle
}

var _ = gc.Suite(&resolveSuite{})

func (s *resolveSuite) TestResolveCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedOrigin, obtainedBases, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, "edge")
	c.Assert(obtainedBases, jc.SameContents, []series.Base{
		series.MustParseBaseFromString("ubuntu@18.04"),
		series.MustParseBaseFromString("ubuntu@20.04"),
	})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmWithAPIError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCallWithAPIError(curl, "edge", errors.New("bad"))

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, _, err = charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, "")
}

func (s *resolveSuite) TestResolveBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	curl.Series = "bundle"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, "edge")
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	curl.Series = "bionic"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) TestCharmHubGetBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-1")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Type:   "bundle",
		Risk:   "edge",
	}
	s.expectedCharmHubGetBundle(c, curl, origin)

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	bundle, err := charmAdapter.GetBundle(curl, origin, "/tmp/")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bundle, gc.DeepEquals, s.bundle)
}

func (s *resolveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
	s.downloadClient = mocks.NewMockDownloadBundleClient(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	return ctrl
}

func (s *resolveSuite) expectCharmResolutionCall(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []series.Base{
			series.MustParseBaseFromString("ubuntu@18.04"),
			series.MustParseBaseFromString("ubuntu@20.04"),
		},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCallWithAPIError(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []series.Base{
			series.MustParseBaseFromString("ubuntu@18.04"),
			series.MustParseBaseFromString("ubuntu@20.04"),
		},
		Error: err,
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, nil)
}

func (s *resolveSuite) expectedCharmHubGetBundle(c *gc.C, curl *charm.URL, origin commoncharm.Origin) {
	surl := "http://messhuggah.com"
	s.charmsAPI.EXPECT().GetDownloadInfo(curl, origin).Return(apicharm.DownloadInfo{
		URL: surl,
	}, nil)
	url, err := url.Parse(surl)
	c.Assert(err, jc.ErrorIsNil)
	s.downloadClient.EXPECT().DownloadAndReadBundle(gomock.Any(), url, "/tmp/", gomock.Any()).Return(s.bundle, nil)
}
