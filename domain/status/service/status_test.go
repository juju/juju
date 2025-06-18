// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
)

type statusSuite struct {
	now time.Time
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &statusSuite{})
}

func (s *statusSuite) SetUpTest(c *tc.C) {
	s.now = time.Now()
}

func (s *statusSuite) TestEncodeK8sPodStatus(c *tc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output status.StatusInfo[status.K8sPodStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Running,
			},
			output: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusRunning,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Running,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &s.now,
			},
			output: status.StatusInfo[status.K8sPodStatusType]{
				Status:  status.K8sPodStatusRunning,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &s.now,
			},
		},
	}

	for i, test := range testCases {
		c.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			output, err := encodeK8sPodStatus(test.input)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Assert(t, output, tc.DeepEquals, test.output)
			result, err := decodeK8sPodStatus(output)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Assert(t, result, tc.DeepEquals, test.input)
		})
	}
}

func (s *statusSuite) TestEncodeUnitAgentStatus(c *tc.C) {
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
		c.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			output, err := encodeUnitAgentStatus(test.input)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, output, tc.DeepEquals, test.output)
			result, err := decodeUnitAgentStatus(output, true)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, result, tc.DeepEquals, test.input)
		})
	}
}

func (s *statusSuite) TestEncodingUnitAgentStatusError(c *tc.C) {
	output, err := encodeUnitAgentStatus(corestatus.StatusInfo{
		Status: corestatus.Error,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(output, tc.DeepEquals, status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusError,
	})

}

func (s *statusSuite) TestDecodeUnitDisplayAndAgentStatus(c *tc.C) {
	agent, workload, err := decodeUnitDisplayAndAgentStatus(status.FullUnitStatus{
		AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
			Status:  status.UnitAgentStatusError,
			Message: "hook failed: hook-name",
			Data:    []byte(`{"foo":"bar"}`),
			Since:   &s.now,
		},
		WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
			Status: status.WorkloadStatusMaintenance,
			Since:  &s.now,
		},
		K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
			Status: status.K8sPodStatusUnset,
		},
		Present: true,
	})

	// If the agent is in an error state, the workload should also
	// be in an error state. In that case, the workload domain will
	// take precedence and we'll set the unit agent domain to idle.
	// This follows the same patter that already exists.

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(agent, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Idle,
		Since:  &s.now,
	})
	c.Assert(workload, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Error,
		Since:   &s.now,
		Data:    map[string]interface{}{"foo": "bar"},
		Message: "hook failed: hook-name",
	})
}

func (s *statusSuite) TestEncodeWorkloadStatus(c *tc.C) {
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
				Since:   &s.now,
			},
			output: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusActive,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &s.now,
			},
		},
	}

	for i, test := range testCases {
		c.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {
			output, err := encodeWorkloadStatus(test.input)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, output, tc.DeepEquals, test.output)
			result, err := decodeUnitWorkloadStatus(output, true)
			tc.Assert(t, err, tc.ErrorIsNil)
			tc.Check(t, result, tc.DeepEquals, test.input)
		})
	}
}

func (s *statusSuite) TestSelectWorkloadOrK8sPodStatusWorkloadTerminatedBlockedMaintenanceDominates(c *tc.C) {
	containerStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status: status.K8sPodStatusBlocked,
	}

	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusTerminated,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &s.now,
	}

	expected := corestatus.StatusInfo{
		Status:  corestatus.Terminated,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &s.now,
	}

	info, err := selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusBlocked
	expected.Status = corestatus.Blocked
	info, err = selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)

	workloadStatus.Status = status.WorkloadStatusMaintenance
	expected.Status = corestatus.Maintenance
	info, err = selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, expected)
}

func (s *statusSuite) TestSelectWorkloadOrK8sPodStatusContainerBlockedDominates(c *tc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusWaiting,
	}

	containerStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusBlocked,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &s.now,
	}

	info, err := selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Blocked,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &s.now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrK8sPodStatusContainerWaitingDominatesActiveWorkload(c *tc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusActive,
	}

	containerStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusWaiting,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &s.now,
	}

	info, err := selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Waiting,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &s.now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrK8sPodStatusContainerRunningDominatesWaitingWorkload(c *tc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status: status.WorkloadStatusWaiting,
	}

	containerStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "msg",
		Data:    []byte(`{"key":"value"}`),
		Since:   &s.now,
	}

	info, err := selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Running,
		Message: "msg",
		Data:    map[string]interface{}{"key": "value"},
		Since:   &s.now,
	})
}

func (s *statusSuite) TestSelectWorkloadOrK8sPodStatusDefaultsToWorkload(c *tc.C) {
	workloadStatus := status.StatusInfo[status.WorkloadStatusType]{
		Status:  status.WorkloadStatusActive,
		Message: "I'm an active workload",
	}

	containerStatus := status.StatusInfo[status.K8sPodStatusType]{
		Status:  status.K8sPodStatusRunning,
		Message: "I'm a running container",
	}

	info, err := selectWorkloadOrK8sPodStatus(workloadStatus, containerStatus, true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status:  corestatus.Active,
		Message: "I'm an active workload",
	})
}

const (
	unitName1 = coreunit.Name("unit-1")
	unitName2 = coreunit.Name("unit-2")
	unitName3 = coreunit.Name("unit-3")
)

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsNoContainers(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Active,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsEmpty(c *tc.C) {
	info, err := applicationDisplayStatusFromUnits(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})

	info, err = applicationDisplayStatusFromUnits(
		status.FullUnitStatuses{},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Unknown,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceContainer(c *tc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusRunning,
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
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusBlocked,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPicksGreatestPrecedenceWorkload(c *tc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusRunning,
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
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusBlocked,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Maintenance,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsPrioritisesUnitWithGreatestStatusPrecedence(c *tc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status: status.WorkloadStatusActive,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status: status.UnitAgentStatusIdle,
			},
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusBlocked,
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
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status: status.K8sPodStatusRunning,
			},
			Present: true,
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Blocked,
	})
}

func (s *statusSuite) TestApplicationDisplayStatusFromUnitsWithError(c *tc.C) {
	fullStatuses := status.FullUnitStatuses{
		unitName1: {
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  status.WorkloadStatusMaintenance,
				Data:    []byte(`{"foo":"bar"}`),
				Message: "boink",
				Since:   &s.now,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  status.UnitAgentStatusError,
				Data:    []byte(`{"foo":"baz"}`),
				Message: "hook failed: hook-name",
				Since:   &s.now,
			},
		},
	}

	info, err := applicationDisplayStatusFromUnits(fullStatuses)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(info, tc.DeepEquals, corestatus.StatusInfo{
		Status: corestatus.Error,
		Data: map[string]interface{}{
			"foo": "baz",
		},
		Message: "hook failed: hook-name",
		Since:   &s.now,
	})
}
