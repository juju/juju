// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type ServiceStatusSuite struct {
	ConnSuite
	service *state.Application
}

var _ = gc.Suite(&ServiceStatusSuite{})

func (s *ServiceStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.service = s.Factory.MakeApplication(c, nil)
}

func (s *ServiceStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *ServiceStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.service.Status()
	c.Check(err, jc.ErrorIsNil)
	checkInitialWorkloadStatus(c, statusInfo)
}

func (s *ServiceStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.service.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ServiceStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.service.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) checkGetSetStatus(c *gc.C) {
	now := time.Now()
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
	err := s.service.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	service, err := s.State.Application(s.service.Name())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := service.Status()
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

func (s *ServiceStatusSuite) TestGetSetStatusDying(c *gc.C) {
	_, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.service.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: application not found`)

	statusInfo, err := s.service.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: application not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *ServiceStatusSuite) TestSetStatusSince(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.service.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := s.service.Status()
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
	err = s.service.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = s.service.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *ServiceStatusSuite) TestDeriveStatus(c *gc.C) {
	// NOTE(fwereade): as detailed in the code, this implementation is not sane.
	// The specified behaviour is arguably sane, but the code is in the wrong
	// place.

	// Create a unit with each possible status.
	addUnit := func(unitStatus status.Status) *state.Unit {
		unit, err := s.service.AddUnit()
		c.Assert(err, gc.IsNil)
		now := time.Now()
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
	errorUnit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "blam",
		Since:   &now,
	}
	err = errorUnit.Agent().SetStatus(sInfo)
	c.Assert(err, gc.IsNil)

	// For each status, in order of severity, check the service status is
	// derived from that unit status; then remove that unit and proceed to
	// the next severity.
	checkAndRemove := func(unit *state.Unit, status status.Status) {
		info, err := s.service.Status()
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
	checkAndRemove(terminatedUnit, status.Terminated)
	checkAndRemove(activeUnit, status.Active)
	checkAndRemove(unknownUnit, status.Unknown)
}

func (s *ServiceStatusSuite) TestServiceStatusOverridesDerivedStatus(c *gc.C) {
	unit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	now := time.Now()
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
	err = s.service.SetStatus(sInfo)
	c.Assert(err, gc.IsNil)

	info, err := s.service.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(info.Status, gc.Equals, status.Maintenance)
}
