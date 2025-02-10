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

func (s *statusSuite) TestEncodeunitWorkloadStatus(c *gc.C) {
	testCases := []struct {
		input  *status.StatusInfo
		output *application.StatusInfo[application.UnitWorkloadStatusType]
	}{
		{
			input: &status.StatusInfo{
				Status: status.Unset,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusUnset,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Unknown,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusUnknown,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Maintenance,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusMaintenance,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Waiting,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusWaiting,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Blocked,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusBlocked,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Active,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusActive,
			},
		},
		{
			input: &status.StatusInfo{
				Status: status.Terminated,
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status: application.UnitWorkloadStatusTerminated,
			},
		},
		{
			input: &status.StatusInfo{
				Status:  status.Active,
				Message: "I'm active!",
				Data:    map[string]interface{}{"foo": "bar"},
			},
			output: &application.StatusInfo[application.UnitWorkloadStatusType]{
				Status:  application.UnitWorkloadStatusActive,
				Message: "I'm active!",
				Data:    []byte(`{"foo":"bar"}`),
			},
		},
	}

	for i, test := range testCases {
		c.Logf("test %d: %v", i, test.input)
		output, err := encodeUnitWorkloadStatus(test.input)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(output, jc.DeepEquals, test.output)
		result, err := decodeUnitWorkloadStatus(output)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, jc.DeepEquals, test.input)
	}
}
