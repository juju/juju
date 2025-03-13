// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

var now = time.Now()

func (s *statusSuite) TestEncodeCloudContainerStatus(c *gc.C) {
	testCases := []struct {
		input  *status.StatusInfo
		output *application.StatusInfo[application.CloudContainerStatusType]
	}{
		{
			input: &status.StatusInfo{
				Status: status.Waiting,
			},
			output: &application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusWaiting,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Blocked,
			},
			output: &application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusBlocked,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Running,
			},
			output: &application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusRunning,
			},
		},
		{
			input: &status.StatusInfo{
				Status:  status.Running,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: &application.StatusInfo[application.CloudContainerStatusType]{
				Status:  application.CloudContainerStatusRunning,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeCloudContainerStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, test.output)
		result, err := decodeCloudContainerStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodeUnitAgentStatus(c *gc.C) {
	testCases := []struct {
		input  *status.StatusInfo
		output *application.StatusInfo[application.UnitAgentStatusType]
	}{
		{
			input: &status.StatusInfo{
				Status: status.Idle,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusIdle,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Allocating,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Executing,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusExecuting,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Failed,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusFailed,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Lost,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusLost,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Rebooting,
			},
			output: &application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusRebooting,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeUnitAgentStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, test.output)
		result, err := decodeUnitAgentStatus(&application.UnitStatusInfo[application.UnitAgentStatusType]{
			StatusInfo: *output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodingUnitAgentStatusError(c *gc.C) {
	output, err := encodeUnitAgentStatus(&status.StatusInfo{
		Status: status.Error,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output, jc.DeepEquals, &application.StatusInfo[application.UnitAgentStatusType]{
		Status: application.UnitAgentStatusError,
	})

	// If the agent is in an error state, the workload should also
	// be in an error state. In that case, the workload status will
	// take precedence and we'll set the unit agent status to idle.
	// This follows the same patter that already exists.

	input, err := decodeUnitAgentStatus(&application.UnitStatusInfo[application.UnitAgentStatusType]{
		StatusInfo: application.StatusInfo[application.UnitAgentStatusType]{
			Status: application.UnitAgentStatusError,
		},
		Present: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(input, jc.DeepEquals, &status.StatusInfo{
		Status: status.Idle,
	})
}

func (s *statusSuite) TestEncodeWorkloadStatus(c *gc.C) {
	testCases := []struct {
		input  *status.StatusInfo
		output *application.StatusInfo[application.WorkloadStatusType]
	}{
		{
			input: &status.StatusInfo{
				Status: status.Unset,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusUnset,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Unknown,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusUnknown,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Maintenance,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusMaintenance,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Waiting,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusWaiting,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Blocked,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusBlocked,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Active,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Terminated,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusTerminated,
			},
		},
		{
			input: &status.StatusInfo{
				Status:  status.Active,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeWorkloadStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, test.output)
		result, err := decodeUnitWorkloadStatus(&application.UnitStatusInfo[application.WorkloadStatusType]{
			StatusInfo: *output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestReduceWorkloadStatusesEmpty(c *gc.C) {
	info, err := reduceUnitWorkloadStatuses(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Unknown,
	})

	info, err = reduceUnitWorkloadStatuses([]application.UnitStatusInfo[application.WorkloadStatusType]{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Unknown,
	})
}

func (s *statusSuite) TestReduceWorkloadStatusesBringsAllDetails(c *gc.C) {
	value := application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "I'm active",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}
	info, err := reduceUnitWorkloadStatuses([]application.UnitStatusInfo[application.WorkloadStatusType]{{
		StatusInfo: value,
		Present:    true,
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Active,
		Message: "I'm active",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestReduceWorkloadStatusesPriority(c *gc.C) {
	for _, t := range []struct {
		status1  application.WorkloadStatusType
		status2  application.WorkloadStatusType
		expected status.Status
	}{
		// Waiting trumps active
		{status1: application.WorkloadStatusActive, status2: application.WorkloadStatusWaiting, expected: status.Waiting},

		// Maintenance trumps active
		{status1: application.WorkloadStatusMaintenance, status2: application.WorkloadStatusWaiting, expected: status.Maintenance},

		// Blocked trumps active
		{status1: application.WorkloadStatusActive, status2: application.WorkloadStatusBlocked, expected: status.Blocked},

		// Blocked trumps waiting
		{status1: application.WorkloadStatusWaiting, status2: application.WorkloadStatusBlocked, expected: status.Blocked},

		// Blocked trumps maintenance
		{status1: application.WorkloadStatusMaintenance, status2: application.WorkloadStatusBlocked, expected: status.Blocked},
	} {
		value, err := reduceUnitWorkloadStatuses([]application.UnitStatusInfo[application.WorkloadStatusType]{
			{
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status: t.status1,
				},
				Present: true,
			},
			{
				StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
					Status: t.status2,
				},
				Present: true,
			},
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(value, gc.NotNil)
		c.Check(value.Status, gc.Equals, t.expected)
	}
}

func (s *statusSuite) TestUnitDisplayStatusNoContainer(c *gc.C) {
	workloadStatus := &application.StatusInfo[application.WorkloadStatusType]{
		Status:  application.WorkloadStatusActive,
		Message: "I'm active",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(&application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: *workloadStatus,
		Present:    true,
	}, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Active,
		Message: "I'm active",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusWorkloadTerminatedBlockedMaintenanceDominates(c *gc.C) {
	containerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status: application.CloudContainerStatusBlocked,
	}

	workloadStatus := &application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusTerminated,
			Message: "msg",
			Data:    []byte(`{"key":"value"}`),
			Since:   &now,
		},
		Present: true,
	}

	expected := &status.StatusInfo{
		Status:  status.Terminated,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = application.WorkloadStatusBlocked
	expected.Status = status.Blocked
	info, err = unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = application.WorkloadStatusMaintenance
	expected.Status = status.Maintenance
	info, err = unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *statusSuite) TestUnitDisplayStatusContainerBlockedDominates(c *gc.C) {
	workloadStatus := &application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusWaiting,
		},
		Present: true,
	}

	containerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusBlocked,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Blocked,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusContainerWaitingDominatesActiveWorkload(c *gc.C) {
	workloadStatus := &application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusActive,
		},
		Present: true,
	}

	containerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusWaiting,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Waiting,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusContainerRunningDominatesWaitingWorkload(c *gc.C) {
	workloadStatus := &application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
			Status: application.WorkloadStatusWaiting,
		},
		Present: true,
	}

	containerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Running,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestUnitDisplayStatusDefaultsToWorkload(c *gc.C) {
	workloadStatus := &application.UnitStatusInfo[application.WorkloadStatusType]{
		StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
			Status:  application.WorkloadStatusActive,
			Message: "I'm an active workload",
		},
		Present: true,
	}

	containerStatus := &application.StatusInfo[application.CloudContainerStatusType]{
		Status:  application.CloudContainerStatusRunning,
		Message: "I'm a running container",
	}

	info, err := unitDisplayStatus(workloadStatus, containerStatus)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status:  status.Active,
		Message: "I'm an active workload",
	})
}

const (
	unitName1 = coreunit.Name("unit-1")
	unitName2 = coreunit.Name("unit-2")
	unitName3 = coreunit.Name("unit-3")
)

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsNoContainers(c *gc.C) {
	workloadStatuses := map[coreunit.Name]application.UnitStatusInfo[application.WorkloadStatusType]{
		unitName1: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Active,
	})

	info, err = applicationDisplayStatusFromUnits(
		workloadStatuses,
		make(map[coreunit.Name]application.StatusInfo[application.CloudContainerStatusType]),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Active,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsEmpty(c *gc.C) {
	info, err := applicationDisplayStatusFromUnits(nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Unknown,
	})

	info, err = applicationDisplayStatusFromUnits(
		map[coreunit.Name]application.UnitStatusInfo[application.WorkloadStatusType]{},
		map[coreunit.Name]application.StatusInfo[application.CloudContainerStatusType]{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Unknown,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceContainer(c *gc.C) {
	workloadStatuses := map[coreunit.Name]application.UnitStatusInfo[application.WorkloadStatusType]{
		unitName1: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]application.StatusInfo[application.CloudContainerStatusType]{
		unitName1: {Status: application.CloudContainerStatusRunning},
		unitName2: {Status: application.CloudContainerStatusBlocked},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceWorkload(c *gc.C) {
	workloadStatuses := map[coreunit.Name]application.UnitStatusInfo[application.WorkloadStatusType]{
		unitName1: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusMaintenance,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]application.StatusInfo[application.CloudContainerStatusType]{
		unitName1: {Status: application.CloudContainerStatusRunning},
		unitName2: {Status: application.CloudContainerStatusBlocked},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Maintenance,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPrioritisesUnitWithGreatestStatusPrecedence(c *gc.C) {
	workloadStatuses := map[coreunit.Name]application.UnitStatusInfo[application.WorkloadStatusType]{
		unitName1: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
			Present: true,
		},
		unitName2: {
			StatusInfo: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusMaintenance,
			},
			Present: true,
		},
	}

	containerStatuses := map[coreunit.Name]application.StatusInfo[application.CloudContainerStatusType]{
		unitName1: {Status: application.CloudContainerStatusBlocked},
		unitName2: {Status: application.CloudContainerStatusRunning},
	}

	info, err := applicationDisplayStatusFromUnits(workloadStatuses, containerStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, &status.StatusInfo{
		Status: status.Blocked,
	})
}
