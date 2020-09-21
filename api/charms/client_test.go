// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/golang/mock/gomock"
	charm "github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
	charmsClient *charms.Client
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestIsMeteredFalse(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	url := "local:quantal/dummy-1"
	args := params.CharmURL{URL: url}
	metered := new(params.IsMeteredResult)
	p := params.IsMeteredResult{Metered: true}

	mockFacadeCaller.EXPECT().FacadeCall("IsMetered", args, metered).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.IsMetered(url)
	c.Assert(err, gc.IsNil)
	c.Assert(got, jc.IsTrue)
}

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	curl := charm.MustParseURL("cs:a-charm")
	curl2 := charm.MustParseURL("cs:focal/dummy-1")
	no := string(csparams.NoChannel)
	edge := string(csparams.EdgeChannel)
	stable := string(csparams.StableChannel)

	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-store"}
	edgeChannelParamsOrigin := params.CharmOrigin{Source: "charm-store", Risk: edge}
	stableChannelParamsOrigin := params.CharmOrigin{Source: "charm-store", Risk: stable}

	facadeArgs := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Origin: noChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
		},
	}
	resolve := new(params.ResolveCharmWithChannelResults)
	p := params.ResolveCharmWithChannelResults{
		Results: []params.ResolveCharmWithChannelResult{
			{
				URL:             curl.String(),
				Origin:          stableChannelParamsOrigin,
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			}, {
				URL:             curl2.String(),
				Origin:          edgeChannelParamsOrigin,
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			},
			{
				URL:             curl2.String(),
				Origin:          edgeChannelParamsOrigin,
				SupportedSeries: []string{"focal"},
			},
		}}

	mockFacadeCaller.EXPECT().FacadeCall("ResolveCharms", facadeArgs, resolve).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)

	noChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: no}
	edgeChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: edge}
	stableChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: stable}
	args := []charms.CharmToResolve{
		{URL: curl, Origin: noChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
	}
	got, err := client.ResolveCharms(args)
	c.Assert(err, gc.IsNil)

	want := []charms.ResolvedCharm{
		{
			URL:             curl,
			Origin:          stableChannelOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Origin:          edgeChannelOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Origin:          edgeChannelOrigin,
			SupportedSeries: []string{"focal"},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestAddCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:testme-2")
	origin := apicharm.Origin{
		Source:   "charm-store",
		ID:       "",
		Hash:     "",
		Risk:     "stable",
		Revision: &curl.Revision,
		Track:    nil,
	}
	facadeArgs := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
	}
	result := new(params.CharmOriginResult)
	actualResult := params.CharmOriginResult{
		Origin: origin.ParamsCharmOrigin(),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddCharm", facadeArgs, result).SetArg(2, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.AddCharm(curl, origin, false)
	c.Assert(err, gc.IsNil)
	c.Assert(got, gc.DeepEquals, origin)
}

func (s *charmsMockSuite) TestAddCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:testme-2")
	origin := apicharm.Origin{
		Source:   "charm-store",
		ID:       "",
		Hash:     "",
		Risk:     "stable",
		Revision: &curl.Revision,
		Track:    nil,
	}
	facadeArgs := params.AddCharmWithAuth{
		URL:                curl.String(),
		CharmStoreMacaroon: &macaroon.Macaroon{},
		Origin:             origin.ParamsCharmOrigin(),
	}
	result := new(params.CharmOriginResult)
	actualResult := params.CharmOriginResult{
		Origin: origin.ParamsCharmOrigin(),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddCharmWithAuthorization", facadeArgs, result).SetArg(2, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.AddCharmWithAuthorization(curl, origin, &macaroon.Macaroon{}, false)
	c.Assert(err, gc.IsNil)
	c.Assert(got, gc.DeepEquals, origin)
}
