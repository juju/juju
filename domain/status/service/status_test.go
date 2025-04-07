// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

var now = time.Now()

func (s *statusSuite) TestEncodeCloudContainerStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.CloudContainerStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Running,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusRunning,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Running,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: status.StatusInfo[status.CloudContainerStatusType]{
				Status:  status.CloudContainerStatusRunning,
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
		input  corestatus.StatusInfo
		output status.StatusInfo[status.UnitAgentStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Idle,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Allocating,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusAllocating,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Executing,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusExecuting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Failed,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusFailed,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Lost,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusLost,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Rebooting,
			},
			output: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusRebooting,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeUnitAgentStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, test.output)
		result, err := decodeUnitAgentStatus(output, true)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodingUnitAgentStatusError(c *gc.C) {
	output, err := encodeUnitAgentStatus(corestatus.StatusInfo{
		Status: corestatus.Error,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output, jc.DeepEquals, status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusError,
	})

}

func (s *statusSuite) TestDecodeUnitDisplayAndAgentStatus(c *gc.C) {
	agent, workload, err := decodeUnitDisplayAndAgentStatus(status.FullUnitStatus{
		AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
			Status:  status.UnitAgentStatusError,
			Message: "hook failed: hook-name",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &now,
		},
		WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusMaintenance,
			Since:  &now,
		},
		ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
			Status: status.CloudContainerStatusUnset,
		},
		Present: true,
	})

	// If the agent is in an error state, the workload should also
	// be in an error state. In that case, the workload domain will
	// take precedence and we'll set the unit agent domain to idle.
	// This follows the same patter that already exists.

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agent, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Idle,
		Since:  &now,
	})
	c.Assert(workload, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Error,
		Since:   &now,
		Data:    map[string]interface{}{"foo": "bar"},
		Message: "hook failed: hook-name",
	})
}

func (s *statusSuite) TestEncodeWorkloadStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.WorkloadStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unset,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusUnset,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unknown,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusUnknown,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Maintenance,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Active,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Terminated,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusTerminated,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Active,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
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
		result, err := decodeUnitWorkloadStatus(output, true)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestSelectWorkloadOrContainerStatusWorkloadTerminatedBlockedMaintenanceDominates(c *gc.C) {
	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status: status.CloudContainerStatusBlocked,
	}

	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	expected := corestatus.StatusInfo{
		Status:  corestatus.Terminated,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	}

	info, err := selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusBlocked
	expected.Status = corestatus.Blocked
	info, err = selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusMaintenance
	expected.Status = corestatus.Maintenance
	info, err = selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, expected)
}

func (s *statusSuite) TestSelectWorkloadOrContainerStatusContainerBlockedDominates(c *gc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusWaiting,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusBlocked,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrContainerStatusContainerWaitingDominatesActiveWorkload(c *gc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusActive,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusWaiting,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Waiting,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrContainerStatusContainerRunningDominatesWaitingWorkload(c *gc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusWaiting,
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &now,
	}

	info, err := selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Running,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrContainerStatusDefaultsToWorkload(c *gc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "I'm an active workload",
	}

	containerStatus := status.StatusInfo[status.CloudContainerStatusType]{
		Status:  status.CloudContainerStatusRunning,
		Message: "I'm a running container",
	}

	info, err := selectWorkloadOrContainerStatus(workloadStatus, containerStatus, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "I'm an active workload",
	})
}

const (
	unitName1 = coreunit.Name("unit-1")
	unitName2 = coreunit.Name("unit-2")
	unitName3 = coreunit.Name("unit-3")
)

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsNoContainers(c *gc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			Present: true,
		},
		unitName2: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Active,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsEmpty(c *gc.C) {
	info, err := applicationDisplayStatusFromUnits(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})

	info, err = applicationDisplayStatusFromUnits(
		status.FullUnitStatuses{},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceContainer(c *gc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusRunning,
			},
			Present: true,
		},
		unitName2: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusBlocked,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceWorkload(c *gc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusRunning,
			},
			Present: true,
		},
		unitName2: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusBlocked,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Maintenance,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPrioritisesUnitWithGreatestStatusPrecedence(c *gc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusBlocked,
			},
			Present: true,
		},
		unitName2: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusMaintenance,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			ContainerStatus: status.StatusInfo[status.CloudContainerStatusType]{
				Status: status.CloudContainerStatusRunning,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsWithError(c *gc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusMaintenance,
				Data:    []byte(`{"foo":"bar"}`),
				Message: "boink",
				Since:   &now,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Data:    []byte(`{"foo":"baz"}`),
				Message: "hook failed: hook-name",
				Since:   &now,
			},
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(info, jc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Error,
		Data: map[string]interface{}{
			"foo": "baz",
		},
		Message: "hook failed: hook-name",
		Since:   &now,
	})
}
