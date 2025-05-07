// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/machine"
)

type statusSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&statusSuite{})

func (s *statusSuite) TestEncodeMachineStatus(c *tc.C) {
	testCases := []struct {
		input  status.StatusInfo
		output machine.StatusInfo[machine.MachineStatusType]
	}{
		{
			input: status.StatusInfo{
				Status: status.Started,
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusStarted,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Stopped,
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusStopped,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Error,
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusError,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Pending,
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusPending,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Down,
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusDown,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Down,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
			output: machine.StatusInfo[machine.MachineStatusType]{
				Status: machine.MachineStatusDown,
				Data:   []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d", i)
		output, err := encodeMachineStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, tc.DeepEquals, test.output)
		result, err := decodeMachineStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodeInstanceStatus(c *tc.C) {
	testCases := []struct {
		input  status.StatusInfo
		output machine.StatusInfo[machine.InstanceStatusType]
	}{
		{
			input: status.StatusInfo{
				Status: status.Unset,
			},
			output: machine.StatusInfo[machine.InstanceStatusType]{
				Status: machine.InstanceStatusUnset,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Running,
			},
			output: machine.StatusInfo[machine.InstanceStatusType]{
				Status: machine.InstanceStatusRunning,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Provisioning,
			},
			output: machine.StatusInfo[machine.InstanceStatusType]{
				Status: machine.InstanceStatusAllocating,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.ProvisioningError,
			},
			output: machine.StatusInfo[machine.InstanceStatusType]{
				Status: machine.InstanceStatusProvisioningError,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Running,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
			output: machine.StatusInfo[machine.InstanceStatusType]{
				Status: machine.InstanceStatusRunning,
				Data:   []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d", i)
		output, err := encodeInstanceStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		result, err := decodeInstanceStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.input)
	}
}
