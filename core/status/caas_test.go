// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
)

type UnitCloudStatusSuite struct{}

var _ = gc.Suite(&UnitCloudStatusSuite{})

func (s *UnitCloudStatusSuite) TestContainerOrUnitStatusChoice(c *gc.C) {

	var checks = []struct {
		cloudContainerStatus status.StatusInfo
		unitStatus           status.StatusInfo
		expectWorkload       bool
		messageCheck         string
	}{
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Error,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Error,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Running,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: status.MessageWaitForContainer,
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: status.MessageWaitForContainer,
			},
			expectWorkload: true,
			messageCheck:   status.MessageWaitForContainer,
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: "waiting for the movie to start",
			},
			expectWorkload: true,
			messageCheck:   "waiting for the movie to start",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Error,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Allocating,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "container",
			},
			unitStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Waiting, Message: status.MessageInitializingAgent},
			expectWorkload:       true,
			messageCheck:         status.MessageWaitForContainer,
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Active, Message: "running"},
			expectWorkload:       true,
			messageCheck:         status.MessageWaitForContainer,
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Active, Message: "running"},
			expectWorkload:       false,
			messageCheck:         "running",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Waiting, Message: status.MessageInitializingAgent},
			expectWorkload:       false,
			messageCheck:         status.MessageInitializingAgent,
		},
	}

	for i, check := range checks {
		c.Logf("Check %d", i)
		c.Assert(status.UnitDisplayStatus(check.unitStatus, check.cloudContainerStatus, check.expectWorkload).Message, gc.Equals, check.messageCheck)
	}
}

func (s *UnitCloudStatusSuite) TestApplicatoinOpeartorStatusChoice(c *gc.C) {

	var checks = []struct {
		operatorStatus status.StatusInfo
		appStatus      status.StatusInfo
		expectWorkload bool
		messageCheck   string
	}{
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Terminated,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "operator",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Error,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "operator",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Allocating,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "operator",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Unknown,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "operator",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Running,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Active,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Terminated,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Terminated,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: "unit",
			},
			expectWorkload: false,
			messageCheck:   "installing agent",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Error,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: "unit",
			},
			expectWorkload: false,
			messageCheck:   "operator",
		},
		{
			operatorStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: "operator",
			},
			appStatus: status.StatusInfo{
				Status:  status.Maintenance,
				Message: "unit",
			},
			expectWorkload: true,
			messageCheck:   "unit",
		},
	}

	for i, check := range checks {
		c.Logf("Check %d", i)
		c.Assert(status.ApplicationDisplayStatus(check.appStatus, check.operatorStatus, check.expectWorkload).Message, gc.Equals, check.messageCheck)
	}
}
