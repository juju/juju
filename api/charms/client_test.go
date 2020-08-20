// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"github.com/golang/mock/gomock"
	charm "github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/charms"
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

func (s *charmsMockSuite) TestCharmInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	url := "local:quantal/dummy-1"
	args := params.CharmURL{URL: url}
	info := new(params.Charm)

	p := params.Charm{
		Revision: 1,
		URL:      url,
		Config: map[string]params.CharmOption{
			"config": {
				Type:        "type",
				Description: "config-type option",
			},
		},
		LXDProfile: &params.CharmLXDProfile{
			Description: "LXDProfile",
			Devices: map[string]map[string]string{
				"tun": {
					"path": "/dev/net/tun",
					"type": "unix-char",
				},
			},
		},
	}

	mockFacadeCaller.EXPECT().FacadeCall("CharmInfo", args, info).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.CharmInfo(url)
	c.Assert(err, gc.IsNil)

	want := &charms.CharmInfo{
		Revision: 1,
		URL:      url,
		Config: &charm.Config{
			Options: map[string]charm.Option{
				"config": {
					Type:        "type",
					Description: "config-type option",
				},
			},
		},
		LXDProfile: &charm.LXDProfile{
			Description: "LXDProfile",
			Config:      map[string]string{},
			Devices: map[string]map[string]string{
				"tun": {
					"path": "/dev/net/tun",
					"type": "unix-char",
				},
			},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	curl := charm.MustParseURL("cs:a-charm")
	curl2 := charm.MustParseURL("cs:focal/dummy-1")
	facadeArgs := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Channel: string(csparams.NoChannel)},
			{Reference: curl2.String(), Channel: string(csparams.EdgeChannel)},
			{Reference: curl2.String(), Channel: string(csparams.EdgeChannel)},
		},
	}
	resolve := new(params.ResolveCharmWithChannelResults)
	p := params.ResolveCharmWithChannelResults{
		Results: []params.ResolveCharmWithChannelResult{
			{
				URL:             curl.String(),
				Channel:         string(csparams.StableChannel),
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			}, {
				URL:             curl2.String(),
				Channel:         string(csparams.EdgeChannel),
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			},
			{
				URL:             curl2.String(),
				Channel:         string(csparams.EdgeChannel),
				SupportedSeries: []string{"focal"},
			},
		}}

	mockFacadeCaller.EXPECT().FacadeCall("ResolveCharms", facadeArgs, resolve).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)

	args := []charms.CharmToResolve{
		{URL: curl, Channel: csparams.NoChannel},
		{URL: curl2, Channel: csparams.EdgeChannel},
		{URL: curl2, Channel: csparams.EdgeChannel},
	}
	got, err := client.ResolveCharms(args)
	c.Assert(err, gc.IsNil)

	want := []charms.ResolvedCharm{
		{
			URL:             curl,
			Channel:         csparams.StableChannel,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Channel:         csparams.EdgeChannel,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Channel:         csparams.EdgeChannel,
			SupportedSeries: []string{"focal"},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}
