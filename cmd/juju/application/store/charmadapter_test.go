// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"net/url"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apicharm "github.com/juju/juju/api/client/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
)

type resolveSuite struct {
	charmsAPI      *mocks.MockCharmsAPI
	charmRepo      *mocks.MockCharmrepoForDeploy
	downloadClient *mocks.MockDownloadBundleClient
	bundle         *mocks.MockBundle
}

var _ = gc.Suite(&resolveSuite{})

func (s *resolveSuite) TearDownTest(c *gc.C) {
	s.charmsAPI = nil
	s.charmRepo = nil
	s.downloadClient = nil
	s.bundle = nil
}

func (s *resolveSuite) TestResolveCharm(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedOrigin, obtainedSeries, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmWithAPIError(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("testme")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCallWithAPIError(curl, csparams.EdgeChannel, errors.New("bad"))

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, _, err = charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s *resolveSuite) TestResolveCharmWithFallback(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedOrigin, obtainedSeries, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:             curl,
		Origin:          origin,
		SupportedSeries: []string{"bionic", "focal"},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, nil)
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.BetaChannel))
}

func (s *resolveSuite) TestResolveCharmFailWithFallbackSuccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, errors.New("fail the test"))
	s.expectCharmFallbackCall(curl, csparams.BetaChannel, csparams.EdgeChannel, nil)
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, gc.IsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
}

func (s *resolveSuite) TestResolveCharmFailResolveWithChannelWithFallback(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, errors.New("fail the test"))
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin, false)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveBundle(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, nil)

	curl.Series = "bundle"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveBundleWithFallback(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.EdgeChannel, csparams.EdgeChannel, nil)

	curl.Series = "bundle"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, nil)

	curl.Series = "bionic"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) TestResolveNotBundleWithFallback(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.EdgeChannel, csparams.EdgeChannel, nil)

	curl.Series = "bionic"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) TestCharmStoreGetBundle(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("cs:testme-1")
	c.Assert(err, jc.ErrorIsNil)

	s.expectedCharmStoreGetBundle(curl)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	bundle, err := charmAdapter.GetBundle(curl, origin, "/tmp/")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bundle, gc.DeepEquals, s.bundle)
}

func (s *resolveSuite) TestCharmHubGetBundle(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	curl, err := charm.ParseURL("ch:testme-1")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmHub,
		Type:   "bundle",
		Risk:   "edge",
	}
	s.expectedCharmHubGetBundle(c, curl, origin)

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	}, func() (store.DownloadBundleClient, error) {
		return s.downloadClient, nil
	})
	bundle, err := charmAdapter.GetBundle(curl, origin, "/tmp/")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bundle, gc.DeepEquals, s.bundle)
}

func (s *resolveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmRepo = mocks.NewMockCharmrepoForDeploy(ctrl)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
	s.downloadClient = mocks.NewMockDownloadBundleClient(ctrl)
	s.bundle = mocks.NewMockBundle(ctrl)
	return ctrl
}

func (s *resolveSuite) expectCharmResolutionCall(curl *charm.URL, out csparams.Channel, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   string(out),
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:             curl,
		Origin:          origin,
		SupportedSeries: []string{"bionic", "focal"},
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, err)
}

func (s *resolveSuite) expectCharmResolutionCallWithAPIError(curl *charm.URL, out csparams.Channel, err error) {
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   string(out),
	}
	retVal := []apicharm.ResolvedCharm{{
		URL:             curl,
		Origin:          origin,
		SupportedSeries: []string{"bionic", "focal"},
		Error:           err,
	}}
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(retVal, nil)
}

func (s *resolveSuite) expectCharmFallbackResolutionCall(curl *charm.URL, in, out csparams.Channel, err error) {
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(nil, errors.NotSupportedf("ResolveCharms"))
	s.expectCharmFallbackCall(curl, in, out, err)
}

func (s *resolveSuite) expectCharmFallbackCall(curl *charm.URL, in, out csparams.Channel, err error) {
	s.charmRepo.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		in,
	).Return(curl, out, []string{"bionic", "focal"}, err)
}

func (s *resolveSuite) expectedCharmStoreGetBundle(curl *charm.URL) {
	s.charmRepo.EXPECT().GetBundle(curl, "/tmp/").Return(s.bundle, nil)
}

func (s *resolveSuite) expectedCharmHubGetBundle(c *gc.C, curl *charm.URL, origin commoncharm.Origin) {
	surl := "http://messhuggah.com"
	s.charmsAPI.EXPECT().GetDownloadInfo(curl, origin, nil).Return(apicharm.DownloadInfo{
		URL: surl,
	}, nil)
	url, err := url.Parse(surl)
	c.Assert(err, jc.ErrorIsNil)
	s.downloadClient.EXPECT().DownloadAndReadBundle(gomock.Any(), url, "/tmp/", gomock.Any()).Return(s.bundle, nil)
}
