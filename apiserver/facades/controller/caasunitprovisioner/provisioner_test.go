// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner_test

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/caasunitprovisioner"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

var _ = gc.Suite(&CAASProvisionerSuite{})

type CAASProvisionerSuite struct {
	coretesting.BaseSuite

	clock               clock.Clock
	st                  *mockState
	applicationsChanges chan []string
	scaleChanges        chan struct{}
	settingsChanges     chan []string

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
	facade     *caasunitprovisioner.Facade
}

func (s *CAASProvisionerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.applicationsChanges = make(chan []string, 1)
	s.scaleChanges = make(chan struct{}, 1)
	s.settingsChanges = make(chan []string, 1)
	s.st = &mockState{
		application: mockApplication{
			tag:             names.NewApplicationTag("gitlab"),
			life:            state.Alive,
			scaleWatcher:    statetesting.NewMockNotifyWatcher(s.scaleChanges),
			settingsWatcher: statetesting.NewMockStringsWatcher(s.settingsChanges),
			scale:           5,
		},
	}
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.scaleWatcher) })
	s.AddCleanup(func(c *gc.C) { workertest.DirtyKill(c, s.st.application.settingsWatcher) })

	s.resources = common.NewResources()
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	s.clock = testclock.NewClock(time.Now())
	s.PatchValue(&jujuversion.OfficialBuild, 0)

	facade, err := caasunitprovisioner.NewFacade(
		s.resources,
		s.authorizer,
		s.st,
		s.clock,
		loggo.GetLogger("juju.apiserver.controller.caasunitprovisioner"),
		nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.facade = facade
}

func (s *CAASProvisionerSuite) TestPermission(c *gc.C) {
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: names.NewMachineTag("0"),
	}
	_, err := caasunitprovisioner.NewFacade(
		s.resources,
		s.authorizer,
		s.st,
		s.clock,
		loggo.GetLogger("juju.apiserver.controller.caasunitprovisioner"),
		nil,
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *CAASProvisionerSuite) TestWatchApplicationsScale(c *gc.C) {

	s.scaleChanges <- struct{}{}

	results, err := s.facade.WatchApplicationsScale(context.Background(), params.Entities{
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

	c.Assert(results.Results[0].NotifyWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.application.scaleWatcher)
}

func (s *CAASProvisionerSuite) TestWatchApplicationsConfigSetingsHash(c *gc.C) {
	s.settingsChanges <- []string{"hash"}

	results, err := s.facade.WatchApplicationsTrustHash(context.Background(), params.Entities{
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

	c.Assert(results.Results[0].StringsWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	c.Assert(resource, gc.Equals, s.st.application.settingsWatcher)
}

func (s *CAASProvisionerSuite) TestApplicationScale(c *gc.C) {
	results, err := s.facade.ApplicationsScale(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "application-gitlab"},
			{Tag: "unit-gitlab-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.IntResults{
		Results: []params.IntResult{{
			Result: 5,
		}, {
			Error: &params.Error{
				Message: `"unit-gitlab-0" is not a valid application tag`,
			},
		}},
	})
	s.st.CheckCallNames(c, "Application")
}
