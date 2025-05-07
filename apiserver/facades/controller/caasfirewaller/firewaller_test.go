// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/caasfirewaller"
	charmscommon "github.com/juju/juju/apiserver/internal/charms"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher/watchertest"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type firewallerSuite struct {
	coretesting.BaseSuite

	st                  *mockState
	applicationsChanges chan []string
	appExposedChanges   chan struct{}

	resources       *common.Resources
	watcherRegistry *facademocks.MockWatcherRegistry
	authorizer      *apiservertesting.FakeAuthorizer

	facade facadeSidecar

	charmService *MockCharmService
	appService   *MockApplicationService

	modelTag names.ModelTag
}

var _ = tc.Suite(&firewallerSuite{})

func (s *firewallerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.modelTag = coretesting.ModelTag

	s.applicationsChanges = make(chan []string, 1)
	s.appExposedChanges = make(chan struct{}, 1)

	appExposedWatcher := watchertest.NewMockNotifyWatcher(s.appExposedChanges)
	s.st = &mockState{
		application: mockApplication{
			watcher: appExposedWatcher,
		},
		applicationsWatcher: watchertest.NewMockStringsWatcher(s.applicationsChanges),
		appExposedWatcher:   appExposedWatcher,
	}
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, s.st.applicationsWatcher) })
	s.AddCleanup(func(c *tc.C) { workertest.DirtyKill(c, s.st.appExposedWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
}

func (s *firewallerSuite) TestPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}

	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(s.modelTag, s.charmService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(s.modelTag, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	_, err = caasfirewaller.NewFacade(
		s.resources,
		s.watcherRegistry,
		s.authorizer,
		s.st,
		commonCharmsAPI,
		appCharmInfoAPI,
		s.appService,
	)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *firewallerSuite) TestWatchApplications(c *tc.C) {
	defer s.setupMocks(c).Finish()

	applicationNames := []string{"db2", "hadoop"}
	s.applicationsChanges <- applicationNames
	result, err := s.facade.WatchApplications(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, tc.IsNil)
	c.Assert(result.StringsWatcherId, tc.Equals, "1")
	c.Assert(result.Changes, jc.DeepEquals, applicationNames)
}

func (s *firewallerSuite) TestWatchApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.appExposedChanges <- struct{}{}

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	results, err := s.facade.Watch(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: "permission denied",
		Code:    "unauthorized access",
	})

	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "1")
}

func (s *firewallerSuite) TestIsExposed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.appService.EXPECT().IsApplicationExposed(gomock.Any(), "gitlab").Return(true, nil)

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

func (s *firewallerSuite) TestLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.appService.EXPECT().GetApplicationLife(gomock.Any(), "gitlab").Return(life.Alive, nil)

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

func (s *firewallerSuite) TestApplicationConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.facade.ApplicationsConfig(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[1].Error, jc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})
	c.Assert(results.Results[0].Config, jc.DeepEquals, map[string]interface{}{"foo": "bar"})
}

func (s *firewallerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.charmService = NewMockCharmService(ctrl)
	s.appService = NewMockApplicationService(ctrl)

	commonCharmsAPI, err := charmscommon.NewCharmInfoAPI(s.modelTag, s.charmService, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	appCharmInfoAPI, err := charmscommon.NewApplicationCharmInfoAPI(s.modelTag, nil, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	facade, err := caasfirewaller.NewFacade(
		s.resources,
		s.watcherRegistry,
		s.authorizer,
		s.st,
		commonCharmsAPI,
		appCharmInfoAPI,
		s.appService,
	)
	c.Assert(err, jc.ErrorIsNil)

	s.facade = facade

	return ctrl
}

type facadeSidecar interface {
	IsExposed(ctx context.Context, args params.Entities) (params.BoolResults, error)
	ApplicationsConfig(ctx context.Context, args params.Entities) (params.ApplicationGetConfigResults, error)
	WatchApplications(ctx context.Context) (params.StringsWatchResult, error)
	Life(ctx context.Context, args params.Entities) (params.LifeResults, error)
	Watch(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error)
	ApplicationCharmInfo(ctx context.Context, args params.Entity) (params.Charm, error)
}
