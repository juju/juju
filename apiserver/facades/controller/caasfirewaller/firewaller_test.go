// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	statemocks "github.com/juju/juju/state/mocks"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type firewallerBaseSuite struct {
	coretesting.BaseSuite

	st                  *mockState
	applicationsChanges chan []string
	openPortsChanges    chan []string
	appExposedChanges   chan struct{}

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     facadeCommon

	deploymentMode charm.DeploymentMode

	newFunc func(c *gc.C, resources facade.Resources,
		authorizer facade.Authorizer,
		st *mockState,
	) (facadeCommon, error)
}

type firewallerLegacySuite struct {
	firewallerBaseSuite
}

var _ = gc.Suite(&firewallerLegacySuite{
	firewallerBaseSuite{
		deploymentMode: charm.ModeWorkload,
		newFunc: func(c *gc.C, resources facade.Resources,
			authorizer facade.Authorizer,
			st *mockState,
		) (facadeCommon, error) {
			return caasfirewaller.NewFacadeLegacyForTest(
				resources, authorizer, st,
				common.NewApplicationWatcherFacade(firewallerStateToAppWatcherState(st), resources, common.ApplicationFilterCAASLegacy),
			)
		},
	},
})

func firewallerStateToAppWatcherState(st *mockState) *mockAppWatcherState {
	return &mockAppWatcherState{
		app: &mockAppWatcherApplication{
			charm: mockAppWatcherCharm{meta: st.application.charm.meta},
		},
		watcher: st.applicationsWatcher,
	}
}

type firewallerEmbeddedSuite struct {
	firewallerBaseSuite

	appPortRanges *statemocks.MockApplicationPortRanges

	facade facadeEmbedded
}

var _ = gc.Suite(&firewallerEmbeddedSuite{
	firewallerBaseSuite: firewallerBaseSuite{
		deploymentMode: charm.ModeEmbedded,
		newFunc: func(c *gc.C, resources facade.Resources,
			authorizer facade.Authorizer,
			st *mockState,
		) (facadeCommon, error) {
			commonCharmsAPI, err := charmscommon.NewCharmsAPI(st, authorizer)
			c.Assert(err, jc.ErrorIsNil)
			return caasfirewaller.NewFacadeEmbeddedForTest(
				resources, authorizer, st, commonCharmsAPI,
				common.NewApplicationWatcherFacade(firewallerStateToAppWatcherState(st), resources, common.ApplicationFilterCAASEmbedded),
			)
		},
	},
})

func (s *firewallerEmbeddedSuite) SetUpTest(c *gc.C) {
	s.firewallerBaseSuite.SetUpTest(c)

	var ok bool
	s.facade, ok = s.firewallerBaseSuite.facade.(facadeEmbedded)
	c.Assert(ok, jc.IsTrue)
}

func (s *firewallerEmbeddedSuite) getCtrl(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.appPortRanges = statemocks.NewMockApplicationPortRanges(ctrl)
	s.st.application.appPortRanges = s.appPortRanges
	return ctrl
}

func (s *firewallerEmbeddedSuite) TestWatchOpenedPorts(c *gc.C) {
	openPortsChanges := []string{"port1", "port2"}
	s.openPortsChanges <- openPortsChanges

	results, err := s.facade.WatchOpenedPorts(params.Entities{
		Entities: []params.Entity{{
			Tag: "model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, openPortsChanges)
}

func (s *firewallerEmbeddedSuite) TestApplicationCharmURLs(c *gc.C) {
	results, err := s.facade.ApplicationCharmURLs(params.Entities{
		Entities: []params.Entity{{
			Tag: "application-gitlab",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "cs:gitlab")
}

func (s *firewallerEmbeddedSuite) TestGetApplicationOpenedPorts(c *gc.C) {
	ctrl := s.getCtrl(c)
	defer ctrl.Finish()

	gomock.InOrder(
		s.appPortRanges.EXPECT().ByEndpoint().Return(network.GroupedPortRanges{
			"": []network.PortRange{
				{
					FromPort: 80,
					ToPort:   8080,
					Protocol: "tcp",
				},
			},
			"endport-1": []network.PortRange{
				{
					FromPort: 8989,
					ToPort:   8888,
					Protocol: "tcp",
				},
			},
		}),
	)

	results, err := s.facade.GetApplicationOpenedPorts(params.Entity{
		Tag: "application-gitlab",
	})
	c.Assert(err, jc.ErrorIsNil)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.ApplicationPortRanges, gc.DeepEquals, []params.ApplicationOpenedPorts{
		{
			PortRanges: []params.PortRange{
				{FromPort: 80, ToPort: 8080, Protocol: "tcp"},
			},
		},
		{
			Endpoint: "endport-1",
			PortRanges: []params.PortRange{
				{FromPort: 8989, ToPort: 8888, Protocol: "tcp"},
			},
		},
	})
}

type facadeCommon interface {
	IsExposed(args params.Entities) (params.BoolResults, error)
	ApplicationsConfig(args params.Entities) (params.ApplicationGetConfigResults, error)
	WatchApplications() (params.StringsWatchResult, error)
	Life(args params.Entities) (params.LifeResults, error)
	Watch(args params.Entities) (params.NotifyWatchResults, error)
}

type facadeEmbedded interface {
	facadeCommon
	WatchOpenedPorts(args params.Entities) (params.StringsWatchResults, error)
	ApplicationCharmURLs(args params.Entities) (params.StringResults, error)
	GetApplicationOpenedPorts(arg params.Entity) (params.ApplicationOpenedPortsResults, error)
}

func (s *firewallerBaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.appExposedChanges = make(chan struct{}, 1)
	s.openPortsChanges = make(chan []string, 1)
	appExposedWatcher := statetesting.NewMockNotifyWatcher(s.appExposedChanges)
	s.st = &mockState{
		application: mockApplication{
			life:    state.Alive,
			watcher: appExposedWatcher,
			charm: mockAppWatcherCharm{
				meta: &charm.Meta{
					Deployment: &charm.Deployment{
						DeploymentMode: s.deploymentMode,
					},
				},
				url: &charm.URL{Schema: "cs", Name: "gitlab", Revision: -1},
			},
		},
		applicationsWatcher: statetesting.NewMockStringsWatcher(s.applicationsChanges),
		openPortsWatcher:    statetesting.NewMockStringsWatcher(s.openPortsChanges),
		appExposedWatcher:   appExposedWatcher,
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.openPortsWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.appExposedWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}

	facade, err := s.newFunc(c, s.resources, s.authorizer, s.st)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *firewallerBaseSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := s.newFunc(c, s.resources, s.authorizer, s.st)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *firewallerBaseSuite) TestWatchApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.applicationsChanges <- applicationNames
	result, err := s.facade.WatchApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)
}

func (s *firewallerBaseSuite) TestWatchApplication(c *gc.C) {
	s.appExposedChanges <- struct{}{}

	results, err := s.facade.Watch(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied",
		Code:    "unauthorized access",
	})

	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.appExposedWatcher)
}

func (s *firewallerBaseSuite) TestIsExposed(c *gc.C) {
	s.st.application.exposed = true
	results, err := s.facade.IsExposed(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{{
			Result: true,
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
}

func (s *firewallerBaseSuite) TestLife(c *gc.C) {
	results, err := s.facade.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{{
			Life: life.Alive,
		}, {
			Error: &params.Error{
				Code:    "unauthorized access",
				Message: "permission denied",
			},
		}},
	})
}

func (s *firewallerBaseSuite) TestApplicationConfig(c *gc.C) {
	results, err := s.facade.ApplicationsConfig(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	c.Assert(results.Results[0].Config, jc.DeepEquals, map[string]interface{}{"foo": "bar"})
}
