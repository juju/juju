// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type ApplicationStatusSuite struct {
	ConnSuite
	application *state.Application
}

var _ = gc.Suite(&ApplicationStatusSuite{})

func (s *ApplicationStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.application = s.Factory.MakeApplication(c, nil)
}

func (s *ApplicationStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *ApplicationStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.application.Status()
	c.Check(err, jc.ErrorIsNil)
	checkInitialWorkloadStatus(c, statusInfo)
}

func (s *ApplicationStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ApplicationStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) checkGetSetStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"$ping": map[string]interface{}{
				"foo.bar": 123,
			},
		},
		Since: &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	application, err := s.State.Application(s.application.Name())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := application.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Active)
	c.Check(statusInfo.Message, gc.Equals, "healthy")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$ping": map[string]interface{}{
			"foo.bar": 123,
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *ApplicationStatusSuite) TestGetSetStatusDying(c *gc.C) {
	_, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ApplicationStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.application.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: application not found`)

	statusInfo, err := s.application.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: application not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *ApplicationStatusSuite) TestSetStatusSince(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.application.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, gc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), jc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	now = now.Add(1 * time.Second)
	sInfo = status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = s.application.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *ApplicationStatusSuite) TestDeriveStatus(c *gc.C) {
	// NOTE(fwereade): as detailed in the code, this implementation is not sane.
	// The specified behaviour is arguably sane, but the code is in the wrong
	// place.

	// Create a unit with each possible status.
	addUnit := func(unitStatus status.Status) *state.Unit {
		unit, err := s.application.AddUnit(state.AddUnitParams{})
		c.Assert(err, gc.IsNil)
		now := testing.ZeroTime()
		sInfo := status.StatusInfo{
			Status:  unitStatus,
			Message: "blam",
			Since:   &now,
		}
		err = unit.SetStatus(sInfo)
		c.Assert(err, gc.IsNil)
		return unit
	}
	blockedUnit := addUnit(status.Blocked)
	waitingUnit := addUnit(status.Waiting)
	maintenanceUnit := addUnit(status.Maintenance)
	terminatedUnit := addUnit(status.Terminated)
	activeUnit := addUnit(status.Active)
	unknownUnit := addUnit(status.Unknown)

	// ...and create one with error status by setting it on the agent :-/.
	errorUnit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.IsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "blam",
		Since:   &now,
	}
	err = errorUnit.Agent().SetStatus(sInfo)
	c.Assert(err, gc.IsNil)

	// For each status, in order of severity, check the application status is
	// derived from that unit status; then remove that unit and proceed to
	// the next severity.
	checkAndRemove := func(unit *state.Unit, status status.Status) {
		info, err := s.application.Status()
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.Status, gc.Equals, status)

		err = unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
	checkAndRemove(errorUnit, status.Error)
	checkAndRemove(blockedUnit, status.Blocked)
	checkAndRemove(waitingUnit, status.Waiting)
	checkAndRemove(maintenanceUnit, status.Maintenance)
	checkAndRemove(activeUnit, status.Active)
	checkAndRemove(terminatedUnit, status.Terminated)
	checkAndRemove(unknownUnit, status.Unknown)
}

func (s *ApplicationStatusSuite) TestApplicationStatusOverridesDerivedStatus(c *gc.C) {
	unit, err := s.application.AddUnit(state.AddUnitParams{})
	c.Assert(err, gc.IsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Blocked,
		Message: "pow",
		Since:   &now,
	}
	err = unit.SetStatus(sInfo)
	c.Assert(err, gc.IsNil)
	sInfo = status.StatusInfo{
		Status:  status.Maintenance,
		Message: "zot",
		Since:   &now,
	}
	err = s.application.SetStatus(sInfo)
	c.Assert(err, gc.IsNil)

	info, err := s.application.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(info.Status, gc.Equals, status.Maintenance)
}
