// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

func TestCAASProvisionerSuite(t *testing.T) {
	tc.Run(t, &CAASProvisionerSuite{})
}

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	clock               clock.Clock
	applicationsChanges chan []string
	scaleChanges        chan struct{}

	watcherRegistry *mocks.MockWatcherRegistry
	resources       *common.Resources
	authorizer      *apiservertesting.FakeAuthorizer

	applicationService *MockApplicationService
	facade             *caasunitprovisioner.Facade
}

func (s *CAASProvisionerSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.scaleChanges = make(chan struct{}, 1)

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
	s.PatchValue(&jujuversion.OfficialBuild, 0)
}

func (s *CAASProvisionerSuite) setUpFacade(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.watcherRegistry = mocks.NewMockWatcherRegistry(ctrl)

	var err error
	facade, err := caasunitprovisioner.NewFacade(
		s.watcherRegistry, s.resources, s.authorizer, s.applicationService, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorIsNil)
	s.facade = facade
	return ctrl
}

func (s *CAASProvisionerSuite) TestPermission(c *tc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasunitprovisioner.NewFacade(
		nil, s.resources, s.authorizer, nil, s.clock, loggertesting.WrapCheckLog(c))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplicationsScale(c *tc.C) {
	defer s.setUpFacade(c).Finish()

	s.scaleChanges <- struct{}{}

	w := watchertest.NewMockNotifyWatcher(s.scaleChanges)
	s.watcherRegistry.EXPECT().Register(w).Return("1", nil)
	s.applicationService.EXPECT().WatchApplicationScale(gomock.Any(), "gitlab").Return(w, nil)

	results, err := s.facade.WatchApplicationsScale(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[1].Error, tc.DeepEquals, &params.Error{
		Message: `"unit-gitlab-0" is not a valid application tag`,
	})

	c.Assert(results.Results[0].NotifyWatcherId, tc.Equals, "1")
}

func (s *CAASProvisionerSuite) TestApplicationScale(c *tc.C) {
	defer s.setUpFacade(c).Finish()

	s.applicationService.EXPECT().GetApplicationScale(gomock.Any(), "gitlab").Return(5, nil)

	results, err := s.facade.ApplicationsScale(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.IntResults{
		Results: []params.IntResult{{
			Result: 5,
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
}
