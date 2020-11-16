// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	apicharm "github.com/juju/juju/api/charms"
	commoncharm "github.com/juju/juju/api/common/charm"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/store"
	"github.com/juju/juju/cmd/juju/application/store/mocks"
)

type resolveSuite struct {
	charmsAPI *mocks.MockCharmsAPI
	charmRepo *mocks.MockCharmrepoForDeploy
}

var _ = gc.Suite(&resolveSuite{})

func (s *resolveSuite) TestResolveCharm(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	})
	obtainedURL, obtainedOrigin, obtainedSeries, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmWithFallback(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	})
	obtainedURL, obtainedOrigin, obtainedSeries, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveCharmFailResolveWithChannel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, errors.New("fail the test"))
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveCharmFailResolveWithChannelWithFallback(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmFallbackResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, errors.New("fail the test"))
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmsAPI, func() (store.CharmrepoForDeploy, error) {
		return s.charmRepo, nil
	})
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveBundleWithFallback(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
	})
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *gc.C) {
	defer s.setupMocks(c).Finish()

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
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) TestResolveNotBundleWithFallback(c *gc.C) {
	defer s.setupMocks(c).Finish()
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
	})
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmRepo = mocks.NewMockCharmrepoForDeploy(ctrl)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
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

func (s *resolveSuite) expectCharmFallbackResolutionCall(curl *charm.URL, in, out csparams.Channel, err error) {
	s.charmsAPI.EXPECT().ResolveCharms(gomock.Any()).Return(nil, errors.NotSupportedf("ResolveCharms"))
	s.charmRepo.EXPECT().ResolveWithPreferredChannel(
		gomock.AssignableToTypeOf(&charm.URL{}),
		in,
	).Return(curl, out, []string{"bionic", "focal"}, err)
}
