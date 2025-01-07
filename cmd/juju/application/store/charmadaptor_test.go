// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"context"
	"net/url"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

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
	charmAdaptor := s.newCharmAdaptor()
	obtainedURL, obtainedOrigin, obtainedBases, err := charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, "edge")
	c.Assert(obtainedBases, jc.SameContents, []base.Base{
		base.MustParseBaseFromString("ubuntu@18.04"),
		base.MustParseBaseFromString("ubuntu@20.04"),
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
	charmAdaptor := s.newCharmAdaptor()
	_, _, _, err = charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdaptor := s.newCharmAdaptor()
	_, obtainedOrigin, _, err := charmAdaptor.ResolveCharm(context.Background(), curl, origin, false)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, "")
}

func (s *resolveSuite) TestResolveBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectBundleResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
		Type:   "bundle",
	}
	charmAdaptor := s.newCharmAdaptor()
	obtainedURL, obtainedChannel, err := charmAdaptor.ResolveBundleURL(context.Background(), curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, "edge")
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("ch:testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, "edge", nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Risk:   "edge",
	}
	charmAdaptor := s.newCharmAdaptor()
	_, _, err = charmAdaptor.ResolveBundleURL(context.Background(), curl, origin)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
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

	charmAdaptor := s.newCharmAdaptor()
	bundle, err := charmAdaptor.GetBundle(context.Background(), curl, origin, "/tmp/bundle.bundle")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bundle, gc.DeepEquals, s.bundle)
}

func (s *resolveSuite) setupMocks(c *gc.C) *gomock.Controller {
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

func (s *resolveSuite) expectedCharmHubGetBundle(c *gc.C, curl *charm.URL, origin commoncharm.Origin) {
	surl := "http://messhuggah.com"
	s.charmsAPI.EXPECT().GetDownloadInfo(gomock.Any(), curl, origin).Return(apicharm.DownloadInfo{
		URL: surl,
	}, nil)
	url, err := url.Parse(surl)
	c.Assert(err, jc.ErrorIsNil)
	s.downloadClient.EXPECT().Download(gomock.Any(), url, "/tmp/bundle.bundle", gomock.Any()).Return(&charmhub.Digest{}, nil)
	s.charmReader.EXPECT().ReadBundleArchive("/tmp/bundle.bundle").Return(s.bundle, nil)
}
