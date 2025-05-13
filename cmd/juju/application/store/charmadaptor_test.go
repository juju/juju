// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub"
)

type resolveSuite struct {
	charmsAPI      *mocks.MockCharmsAPI
	downloadClient *mocks.MockDownloadBundleClient
	bundle         *mocks.MockBundle
	charmReader    *mocks.MockCharmReader
}

var _ = tc.Suite(&resolveSuite{})

func (s *resolveSuite) TestResolveCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-3")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdaptor := s.newCharmAdaptor()
	obtainedURL, obtainedOrigin, obtainedBases, err := charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, tc.Equals, "edge")
	c.Assert(obtainedBases, tc.SameContents, []base.Base{
		base.MustParseBaseFromString("ubuntu@18.04"),
		base.MustParseBaseFromString("ubuntu@20.04"),
	})
	c.Assert(obtainedURL, tc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmWithAPIError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCallWithAPIError(curl, "edge", errors.New("bad"))

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "beta",
	}
	charmAdaptor := s.newCharmAdaptor()
	_, _, _, err = charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, tc.ErrorMatches, `bad`)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *tc.C) {
	c.Skip("FIXME: this test passes - is it supposed to?")
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCallWithAPIError(curl, "edge", errors.New("bad"))

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdaptor := s.newCharmAdaptor()
	_, obtainedOrigin, _, err := charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, tc.NotNil)
	c.Assert(obtainedOrigin.Risk, tc.Equals, "")
}

func (s *resolveSuite) TestResolveBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectBundleResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
		Type:   "bundle",
	}
	charmAdaptor := s.newCharmAdaptor()
	obtainedURL, obtainedChannel, err := charmAdaptor.ResolveBundleURL(context.Background(), curl, origin)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, tc.Equals, "edge")
	c.Assert(obtainedURL, tc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, tc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
	}
	charmAdaptor := s.newCharmAdaptor()
	_, _, err = charmAdaptor.ResolveBundleURL(context.Background(), curl, origin)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *resolveSuite) TestCharmHubGetBundle(c *tc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme-1")
	c.Assert(err, tc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Type:   "bundle",
		Risk:   "edge",
	}
	s.expectedCharmHubGetBundle(c, curl, origin)

	charmAdaptor := s.newCharmAdaptor()
	bundle, err := charmAdaptor.GetBundle(context.Background(), curl, origin, "/tmp/bundle.bundle")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bundle, tc.DeepEquals, s.bundle)
}

func (s *resolveSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
	s.downloadClient = mocks.NewMockDownloadBundleClient(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	s.charmReader = mocks.NewMockCharmReader(ctrl)
	return ctrl
}

func (s *resolveSuite) newCharmAdaptor() *CharmAdaptor {
	return &CharmAdaptor{
		charmsAPI: s.charmsAPI,
		bundleRepoFn: func(curl *charm.URL) (BundleFactory, error) {
			return chBundleFactory{
				charmsAPI:   s.charmsAPI,
				charmReader: s.charmReader,
				downloadBundleClientFunc: func(ctx context.Context) (DownloadBundleClient, error) {
					return s.downloadClient, nil
				},
			}, nil
		},
	}
}

func (s *resolveSuite) expectBundleResolutionCall(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "bundle",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any(), gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCall(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "charm",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any(), gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCallWithAPIError(curl *charm.URL, out string, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   out,
		Type:   "charm",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:    curl,
		Origin: origin,
		SupportedBases: []base.Base{
			base.MustParseBaseFromString("ubuntu@18.04"),
			base.MustParseBaseFromString("ubuntu@20.04"),
		},
		Error: err,
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any(), gomock.Any()).Return(retVal, nil)
}

func (s *resolveSuite) expectedCharmHubGetBundle(c *tc.C, curl *charm.URL, origin commoncharm.Origin) {
	surl := "http://messhuggah.com"
	s.charmsAPI.EXPECT().GetDownloadInfo(gomock.Any(), curl, origin).Return(apicharm.DownloadInfo{
		URL: surl,
	}, nil)
	url, err := url.Parse(surl)
	c.Assert(err, tc.ErrorIsNil)
	s.downloadClient.EXPECT().Download(gomock.Any(), url, "/tmp/bundle.bundle", gomock.Any()).Return(&charmhub.Digest{}, nil)
	s.charmReader.EXPECT().ReadBundleArchive("/tmp/bundle.bundle").Return(s.bundle, nil)
}
