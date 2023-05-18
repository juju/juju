// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
)

type ApplicationSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) status(value status.Status, when time.Time) status.StatusInfo {
	return status.StatusInfo{
		Status: value,
		Since:  &when,
	}
}

func (s *ApplicationSuite) TestStatusWhenSet(c *gc.C) {
	appStatus := s.status(status.Active, time.Now())
	ctx := status.AppContext{
		AppStatus: appStatus,
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestStatusWhenUnsetNoUnits(c *gc.C) {
	now := time.Now()
	ctx := status.AppContext{
		AppStatus: s.status(status.Unset, now),
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, status.StatusInfo{
		Status: status.Unknown,
		Since:  &now,
	})
}

func (s *ApplicationSuite) TestStatusWhenUnsetWithUnits(c *gc.C) {
	appStatus := s.status(status.Unset, time.Now())
	// Application status derivation uses the status.DeriveStatus method
	// which defines the relative priorities of the status values.
	expected := s.status(status.Waiting, time.Now().Add(-time.Minute))
	ctx := status.AppContext{
		AppStatus: appStatus,
		UnitCtx: []status.UnitContext{{
			WorkloadStatus: expected,
		}},
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, expected)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorRunning(c *gc.C) {
	appStatus := s.status(status.Active, time.Now())
	ctx := status.AppContext{
		AppStatus:      appStatus,
		OperatorStatus: s.status(status.Running, time.Now()),
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorActive(c *gc.C) {
	appStatus := s.status(status.Blocked, time.Now())
	ctx := status.AppContext{
		AppStatus:      appStatus,
		OperatorStatus: s.status(status.Active, time.Now()),
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, appStatus)
}

func (s *ApplicationSuite) TestDisplayStatusOperatorWaiting(c *gc.C) {
	expected := s.status(status.Waiting, time.Now())
	expected.Message = status.MessageInstallingAgent
	ctx := status.AppContext{
		AppStatus:      s.status(status.Active, time.Now()),
		OperatorStatus: expected,
	}
	c.Assert(status.DisplayApplicationStatus(ctx), jc.DeepEquals, expected)
}
