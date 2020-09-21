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
	charmRepo  *mocks.MockCharmrepoForDeploy
	charmsAPI  *mocks.MockCharmsAPI
	apiVersion int
}

var _ = gc.Suite(&resolveSuite{apiVersion: 2})
var _ = gc.Suite(&resolveSuite{apiVersion: 3})

func (s *resolveSuite) TestResolveCharm(c *gc.C) {
	c.Logf("CharmsAPI version %d", s.apiVersion)
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, nil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmRepo, s.apiVersion, s.charmsAPI)
	obtainedURL, obtainedOrigin, obtainedSeries, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedSeries, jc.SameContents, []string{"bionic", "focal"})
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveCharmNotCSCharm(c *gc.C) {
	c.Logf("CharmsAPI version %d", s.apiVersion)
	if s.apiVersion != 2 {
		c.Skip("Test not applicable to CharmsAPI v3")
	}

	curl, err := charm.ParseURL("local:bionic/testme-3")
	c.Assert(err, jc.ErrorIsNil)

	origin := commoncharm.Origin{
		Source: commoncharm.OriginLocal,
		Risk:   "beta",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmRepo, s.apiVersion, s.charmsAPI)
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveCharmFailResolveWithChannel(c *gc.C) {
	c.Logf("CharmsAPI version %d", s.apiVersion)
	defer s.setupMocks(c).Finish()

	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.BetaChannel, csparams.EdgeChannel, errors.New("fail the test"))
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "beta",
	}

	charmAdapter := store.NewCharmAdaptor(s.charmRepo, s.apiVersion, s.charmsAPI)
	_, obtainedOrigin, _, err := charmAdapter.ResolveCharm(curl, origin)
	c.Assert(err, gc.NotNil)
	c.Assert(obtainedOrigin.Risk, gc.Equals, string(csparams.NoChannel))
}

func (s *resolveSuite) TestResolveBundle(c *gc.C) {
	c.Logf("CharmsAPI version %d", s.apiVersion)
	defer s.setupMocks(c).Finish()
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, csparams.EdgeChannel, nil)

	curl.Series = "bundle"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmRepo, s.apiVersion, s.charmsAPI)
	obtainedURL, obtainedChannel, err := charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedChannel.Risk, gc.Equals, string(csparams.EdgeChannel))
	c.Assert(obtainedURL, gc.Equals, curl)
}

func (s *resolveSuite) TestResolveNotBundle(c *gc.C) {
	c.Logf("CharmsAPI version %d", s.apiVersion)
	defer s.setupMocks(c).Finish()
	curl, err := charm.ParseURL("cs:testme-3")
	c.Assert(err, jc.ErrorIsNil)
	s.expectCharmResolutionCall(curl, csparams.EdgeChannel, csparams.EdgeChannel, nil)

	curl.Series = "bionic"
	origin := commoncharm.Origin{
		Source: commoncharm.OriginCharmStore,
		Risk:   "edge",
	}
	charmAdapter := store.NewCharmAdaptor(s.charmRepo, s.apiVersion, s.charmsAPI)
	_, _, err = charmAdapter.ResolveBundleURL(curl, origin)
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
}

func (s *resolveSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmRepo = mocks.NewMockCharmrepoForDeploy(ctrl)
	s.charmsAPI = mocks.NewMockCharmsAPI(ctrl)
	return ctrl
}

func (s *resolveSuite) expectCharmResolutionCall(curl *charm.URL, in, out csparams.Channel, err error) {
	if s.apiVersion == 2 {
		s.charmRepo.EXPECT().ResolveWithPreferredChannel(
			gomock.AssignableToTypeOf(&charm.URL{}),
			in,
		).Return(curl, out, []string{"bionic", "focal"}, err)
		return
	}

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
