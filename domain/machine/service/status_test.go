// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/status"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/testhelpers"
)

type statusSuite struct {
	testhelpers.IsolationSuite
}

func TestStatusSuite(t *testing.T) {
	tc.Run(t, &statusSuite{})
}

func (s *statusSuite) TestEncodeMachineStatus(c *tc.C) {
	testCases := []struct {
		input  status.StatusInfo
		output domainstatus.StatusInfo[domainstatus.MachineStatusType]
	}{
		{
			input: status.StatusInfo{
				Status: status.Started,
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusStarted,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Stopped,
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusStopped,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Error,
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusError,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Pending,
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusPending,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Down,
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusDown,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Down,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
			output: domainstatus.StatusInfo[domainstatus.MachineStatusType]{
				Status: domainstatus.MachineStatusDown,
				Data:   []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d", i)
		output, err := encodeMachineStatus(test.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(output, tc.DeepEquals, test.output)
		result, err := decodeMachineStatus(output)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.input)
	}
}

func (s *statusSuite) TestEncodeInstanceStatus(c *tc.C) {
	testCases := []struct {
		input  status.StatusInfo
		output domainstatus.StatusInfo[domainstatus.InstanceStatusType]
	}{
		{
			input: status.StatusInfo{
				Status: status.Unset,
			},
			output: domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
				Status: domainstatus.InstanceStatusUnset,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Running,
			},
			output: domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
				Status: domainstatus.InstanceStatusRunning,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Provisioning,
			},
			output: domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
				Status: domainstatus.InstanceStatusAllocating,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.ProvisioningError,
			},
			output: domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
				Status: domainstatus.InstanceStatusProvisioningError,
			},
		},
		{
			input: status.StatusInfo{
				Status: status.Running,
				Data: map[string]interface{}{
					"foo": "bar",
				},
			},
			output: domainstatus.StatusInfo[domainstatus.InstanceStatusType]{
				Status: domainstatus.InstanceStatusRunning,
				Data:   []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d", i)
		output, err := encodeInstanceStatus(test.input)
		c.Assert(err, tc.ErrorIsNil)
		result, err := decodeInstanceStatus(output)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(result, tc.DeepEquals, test.input)
	}
}
