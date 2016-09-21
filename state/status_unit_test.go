// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type UnitStatusSuite struct {
	ConnSuite
	unit *state.Unit
}

var _ = gc.Suite(&UnitStatusSuite{})

func (s *UnitStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)
}

func (s *UnitStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *UnitStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.unit.Status()
	c.Check(err, jc.ErrorIsNil)
	checkInitialWorkloadStatus(c, statusInfo)
}

func (s *UnitStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *UnitStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) checkGetSetStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "healthy",
		Data: map[string]interface{}{
			"$ping": map[string]interface{}{
				"foo.bar": 123,
			}},
		Since: &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Check(err, jc.ErrorIsNil)

	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := unit.Status()
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

func (s *UnitStatusSuite) TestGetSetStatusDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) TestGetSetStatusDead(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *UnitStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.unit.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "not really",
		Since:   &now,
	}
	err = s.unit.SetStatus(sInfo)
	c.Check(err, gc.ErrorMatches, `cannot set status: unit not found`)

	statusInfo, err := s.unit.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: unit not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *UnitStatusSuite) TestSetUnitStatusSince(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "",
		Since:   &now,
	}
	err := s.unit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := s.unit.Status()
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
	err = s.unit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = s.unit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *UnitStatusSuite) TestStatusHistoryInitial(c *gc.C) {
	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 1})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 1)

	checkInitialWorkloadStatus(c, history[0])
}

func (s *UnitStatusSuite) TestStatusHistoryShort(c *gc.C) {
	primeUnitStatusHistory(c, s.unit, 5, 0)

	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 6)

	checkInitialWorkloadStatus(c, history[5])
	history = history[:5]
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 4-i, 0)
	}
}

func (s *UnitStatusSuite) TestStatusHistoryLong(c *gc.C) {
	primeUnitStatusHistory(c, s.unit, 25, 0)

	history, err := s.unit.StatusHistory(status.StatusHistoryFilter{Size: 15})
	c.Check(err, jc.ErrorIsNil)
	c.Check(history, gc.HasLen, 15)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 24-i, 0)
	}
}
