// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"

	"github.com/juju/charm/v12"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

type firewallerSuite struct {
	coretesting.BaseSuite

	st                  *mockState
	applicationsChanges chan []string
	openPortsChanges    chan []string
	appExposedChanges   chan struct{}

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer

	newFunc func(c *gc.C, resources facade.Resources,
		authorizer facade.Authorizer,
		st *mockState,
	) (facadeSidecar, error)

	facade facadeSidecar
}

var _ = gc.Suite(&firewallerSuite{
	newFunc: func(c *gc.C, resources facade.Resources,
		authorizer facade.Authorizer,
		st *mockState,
	) (facadeSidecar, error) {
		commonState := &mockCommonStateShim{st}
		commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(commonState, authorizer)
		c.Assert(err, jc.ErrorIsNil)
		appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(commonState, authorizer)
		c.Assert(err, jc.ErrorIsNil)
		return caasfirewaller.NewFacadeForTest(
			resources,
			authorizer,
			st,
			commonCharmsAPI,
			appCharmInfoAPI,
		)
	},
})

func (s *firewallerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.appExposedChanges = make(chan struct{}, 1)
	s.openPortsChanges = make(chan []string, 1)
	appExposedWatcher := statetesting.NewMockNotifyWatcher(s.appExposedChanges)
	s.st = &mockState{
		application: mockApplication{
			life:    state.Alive,
			watcher: appExposedWatcher,
			charm: mockCharm{
				meta: &charm.Meta{
					Deployment: &charm.Deployment{},
				},
				manifest: &charm.Manifest{},
				url:      "ch:gitlab",
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

	// charm.FormatV2.
	s.st.application.charm.manifest.Bases = []charm.Base{
		{
			Name: "ubuntu",
			Channel: charm.Channel{
				Risk:  "stable",
				Track: "20.04",
			},
		},
	}
}

func (s *firewallerSuite) TestWatchOpenedPorts(c *gc.C) {
	openPortsChanges := []string{"port1", "port2"}
	s.openPortsChanges <- openPortsChanges

	results, err := s.facade.WatchOpenedPorts(context.Background(), params.Entities{
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

func (s *firewallerSuite) TestGetApplicationOpenedPorts(c *gc.C) {
	s.st.application.appPortRanges = network.GroupedPortRanges{
		"": []network.PortRange{
			{
				FromPort: 80,
				ToPort:   80,
				Protocol: "tcp",
			},
		},
		"endport-1": []network.PortRange{
			{
				FromPort: 8888,
				ToPort:   8888,
				Protocol: "tcp",
			},
		},
	}

	results, err := s.facade.GetOpenedPorts(context.Background(), params.Entity{
		Tag: "application-gitlab",
	})
	c.Assert(err, jc.ErrorIsNil)
	result := results.Results[0]
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.ApplicationPortRanges, gc.DeepEquals, []params.ApplicationOpenedPorts{
		{
			PortRanges: []params.PortRange{
				{FromPort: 80, ToPort: 80, Protocol: "tcp"},
			},
		},
		{
			Endpoint: "endport-1",
			PortRanges: []params.PortRange{
				{FromPort: 8888, ToPort: 8888, Protocol: "tcp"},
			},
		},
	})
}

type facadeSidecar interface {
	IsExposed(ctx context.Context, args params.Entities) (params.BoolResults, error)
	ApplicationsConfig(ctx context.Context, args params.Entities) (params.ApplicationGetConfigResults, error)
	WatchApplications(ctx context.Context) (params.StringsWatchResult, error)
	Life(ctx context.Context, args params.Entities) (params.LifeResults, error)
	Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error)
	ApplicationCharmInfo(ctx context.Context, args params.Entity) (params.Charm, error)
	WatchOpenedPorts(ctx context.Context, args params.Entities) (params.StringsWatchResults, error)
	GetOpenedPorts(ctx context.Context, arg params.Entity) (params.ApplicationOpenedPortsResults, error)
}

func (s *firewallerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := s.newFunc(c, s.resources, s.authorizer, s.st)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *firewallerSuite) TestWatchApplications(c *gc.C) {
	applicationNames := []string{"db2", "hadoop"}
	s.applicationsChanges <- applicationNames
	result, err := s.facade.WatchApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)
}

func (s *firewallerSuite) TestWatchApplication(c *gc.C) {
	s.appExposedChanges <- struct{}{}

	results, err := s.facade.Watch(context.Background(), params.Entities{
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

func (s *firewallerSuite) TestIsExposed(c *gc.C) {
	s.st.application.exposed = true
	results, err := s.facade.IsExposed(context.Background(), params.Entities{
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

func (s *firewallerSuite) TestLife(c *gc.C) {
	results, err := s.facade.Life(context.Background(), params.Entities{
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

func (s *firewallerSuite) TestApplicationConfig(c *gc.C) {
	results, err := s.facade.ApplicationsConfig(context.Background(), params.Entities{
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
