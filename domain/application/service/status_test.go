// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/status"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

var now = time.Now()

func (s *statusSuite) TestEncodeK8sPodStatus(c *gc.C) {
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
				Since:   &now,
			},
			output: status.StatusInfo[status.K8sPodStatusType]{
				Status:  status.K8sPodStatusRunning,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeK8sPodStatus(&test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, &test.output)
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
		output, err := encodeUnitAgentStatus(&test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, &test.output)
		result, err := decodeUnitAgentStatus(&status.UnitStatusInfo[status.UnitAgentStatusType]{
			StatusInfo: *output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, &test.input)
	}
}

func (s *statusSuite) TestEncodingUnitAgentStatusError(c *gc.C) {
	output, err := encodeUnitAgentStatus(&corestatus.StatusInfo{
		Status: corestatus.Error,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(output, jc.DeepEquals, &status.StatusInfo[status.UnitAgentStatusType]{
		Status: status.UnitAgentStatusError,
	})

	// If the agent is in an error state, the workload should also
	// be in an error state. In that case, the workload status will
	// take precedence and we'll set the unit agent status to idle.
	// This follows the same patter that already exists.

	input, err := decodeUnitAgentStatus(&status.UnitStatusInfo[status.UnitAgentStatusType]{
		StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusError,
		},
		Present: true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(input, jc.DeepEquals, &corestatus.StatusInfo{
		Status: corestatus.Idle,
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
		output, err := encodeWorkloadStatus(&test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, &test.output)
		result, err := decodeUnitWorkloadStatus(&status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: *output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, &test.input)
	}
}
