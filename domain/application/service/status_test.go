// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

var now = time.Now()

func (s *statusSuite) TestEncodeCloudContainerStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output application.StatusInfo[application.CloudContainerStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Running,
			},
			output: application.StatusInfo[application.CloudContainerStatusType]{
				Status: application.CloudContainerStatusRunning,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Running,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: application.StatusInfo[application.CloudContainerStatusType]{
				Status:  application.CloudContainerStatusRunning,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
				Since:   &now,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeCloudContainerStatus(&test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, &test.output)
	}
}

func (s *statusSuite) TestEncodeUnitAgentStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output application.StatusInfo[application.UnitAgentStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Idle,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusIdle,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Allocating,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusAllocating,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Executing,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusExecuting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Failed,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusFailed,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Lost,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusLost,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Rebooting,
			},
			output: application.StatusInfo[application.UnitAgentStatusType]{
				Status: application.UnitAgentStatusRebooting,
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeUnitAgentStatus(&test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(output, jc.DeepEquals, &test.output)
		result, err := decodeUnitAgentStatus(&application.UnitStatusInfo[application.UnitAgentStatusType]{
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
	c.Check(input, jc.DeepEquals, &corestatus.StatusInfo{
		Status: corestatus.Idle,
	})
}

func (s *statusSuite) TestEncodeWorkloadStatus(c *gc.C) {
	testCases := []struct {
		input  corestatus.StatusInfo
		output application.StatusInfo[application.WorkloadStatusType]
	}{
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unset,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusUnset,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Unknown,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusUnknown,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Maintenance,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusMaintenance,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Waiting,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusWaiting,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Blocked,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusBlocked,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Active,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusActive,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status: corestatus.Terminated,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status: application.WorkloadStatusTerminated,
			},
		},
		{
			input: corestatus.StatusInfo{
				Status:  corestatus.Active,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			},
			output: application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
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
		result, err := decodeUnitWorkloadStatus(&application.UnitStatusInfo[application.WorkloadStatusType]{
			StatusInfo: *output,
			Present:    true,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Check(result, jc.DeepEquals, &test.input)
	}
}
