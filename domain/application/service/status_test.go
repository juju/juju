// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application"
)

type statusSuite struct{}

var _ = gc.Suite(&statusSuite{})

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
			},
			output: &application.StatusInfo[application.WorkloadStatusType]{
				Status:  application.WorkloadStatusActive,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeWorkloadStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, test.output)
		result, err := decodeWorkloadStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, jc.DeepEquals, test.input)
	}
}
