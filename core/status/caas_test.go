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
			messageCheck: "unit",
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
			messageCheck: "container",
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
			messageCheck: "container",
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
			messageCheck: "unit",
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
			messageCheck: "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus: status.StatusInfo{
				Status:  status.Waiting,
				Message: status.MessageWaitForContainer,
			},
			messageCheck: status.MessageWaitForContainer,
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
			messageCheck: "waiting for the movie to start",
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
			messageCheck: "container",
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
			messageCheck: "container",
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
			messageCheck: "container",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus: status.StatusInfo{
				Status:  status.Blocked,
				Message: "unit",
			},
			messageCheck: "unit",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Active, Message: "running"},
			messageCheck:         "running",
		},
		{
			cloudContainerStatus: status.StatusInfo{},
			unitStatus:           status.StatusInfo{Status: status.Waiting, Message: status.MessageInitializingAgent},
			messageCheck:         status.MessageInitializingAgent,
		},
	}

	for i, check := range checks {
		c.Logf("Check %d", i)
		c.Assert(status.UnitDisplayStatus(check.unitStatus, check.cloudContainerStatus).Message, gc.Equals, check.messageCheck)
	}
}
