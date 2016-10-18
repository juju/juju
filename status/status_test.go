// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"time"

	"github.com/juju/juju/status"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type statusHistorySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&statusHistorySuite{})

func (h *statusHistorySuite) TestStatusSquashing(c *gc.C) {
	since := time.Now()
	statuses := status.History{
		{
			Status: status.Active,
			Info:   "unique status one",
			Since:  &since,
		},
		{
			Status: status.Active,
			Info:   "unique status two",
			Since:  &since,
		},
		{
			Status: status.Active,
			Info:   "unique status three",
			Since:  &since,
		},
		{
			Status: status.Executing,
			Info:   "repeated status one",
			Since:  &since,
		},
		{
			Status: status.Idle,
			Info:   "repeated status two",
			Since:  &since,
		},
		{
			Status: status.Executing,
			Info:   "repeated status one",
			Since:  &since,
		},
		{
			Status: status.Idle,
			Info:   "repeated status two",
			Since:  &since,
		},
		{
			Status: status.Executing,
			Info:   "repeated status one",
			Since:  &since,
		},
		{
			Status: status.Idle,
			Info:   "repeated status two",
			Since:  &since,
		},
	}
	newStatuses := statuses.SquashLogs(2)
	c.Assert(newStatuses, gc.HasLen, 6)

	newStatuses[5].Since = &since
	expectedStatuses := status.History{
		{
			Status: status.Active,
			Info:   "unique status one",
			Since:  &since,
		},
		{
			Status: status.Active,
			Info:   "unique status two",
			Since:  &since,
		},
		{
			Status: status.Active,
			Info:   "unique status three",
			Since:  &since,
		},
		{
			Status: status.Executing,
			Info:   "repeated status one",
			Since:  &since,
		},
		{
			Status: status.Idle,
			Info:   "repeated status two",
			Since:  &since,
		},
		{
			Status: status.Idle,
			Info:   "last 2 statuses repeated 2 times",
			Since:  &since,
		},
	}

	c.Assert(newStatuses, gc.DeepEquals, expectedStatuses)
}
