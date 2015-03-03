// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type MeterStateSuite struct {
	ConnSuite
	unit           *state.Unit
	factory        *factory.Factory
	metricsManager *state.MetricsManager
}

var _ = gc.Suite(&MeterStateSuite{})

func (s *MeterStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.factory = factory.NewFactory(s.State)
	s.unit = s.factory.MakeUnit(c, nil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
	var err error
	s.metricsManager, err = s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MeterStateSuite) TestMeterStatus(c *gc.C) {
	status, info, err := s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetMeterStatus("GREEN", "Additional information.")
	c.Assert(err, jc.ErrorIsNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "GREEN")
	c.Assert(info, gc.Equals, "Additional information.")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MeterStateSuite) TestMeterStatusIncludesEnvUUID(c *gc.C) {
	jujuDB := s.MgoSuite.Session.DB("juju")
	meterStatus := jujuDB.C("meterStatus")
	var docs []bson.M
	err := meterStatus.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(docs, gc.HasLen, 1)
	c.Assert(docs[0]["env-uuid"], gc.Equals, s.State.EnvironUUID())
}

func (s *MeterStateSuite) TestSetMeterStatusIncorrect(c *gc.C) {
	err := s.unit.SetMeterStatus("NOT SET", "Additional information.")
	c.Assert(err, gc.NotNil)
	status, info, err := s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetMeterStatus("this-is-not-a-valid-status", "Additional information.")
	c.Assert(err, gc.NotNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MeterStateSuite) TestSetMeterStatusWhenDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, s.unit, contentionErr, contentionErr, func() error {
		err := s.unit.SetMeterStatus("GREEN", "Additional information.")
		if err != nil {
			return err
		}
		status, info, err := s.unit.GetMeterStatus()
		c.Assert(status, gc.Equals, "NOT SET")
		c.Assert(info, gc.Equals, "")
		c.Assert(err, jc.ErrorIsNil)
		return nil
	})
}

func (s *MeterStateSuite) TestMeterStatusRemovedWithUnit(c *gc.C) {
	err := s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	code, info, err := s.unit.GetMeterStatus()
	c.Assert(err, gc.ErrorMatches, "cannot retrieve meter status for unit .*: not found")
	c.Assert(code, gc.Equals, "NOT AVAILABLE")
	c.Assert(info, gc.Equals, "")
}

func (s *MeterStateSuite) TestMeterStatusWatcherRespondstoMeterStatus(c *gc.C) {
	watcher := s.unit.WatchMeterStatus()
	err := s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	assertMeterStatusChanged(c, watcher)
}

func (s *MeterStateSuite) TestMeterStatusWatcherRespondsToMetricsManager(c *gc.C) {
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	watcher := s.unit.WatchMeterStatus()
	err = mm.SetLastSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := mm.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	code, _ := mm.MeterStatus()
	c.Assert(code, gc.Equals, state.MeterAmber) // Confirm meter status has changed
	assertMeterStatusChanged(c, watcher)
}

func assertMeterStatusChanged(c *gc.C, w state.NotifyWatcher) {
	for i := 0; i < 2; i++ {
		select {
		case <-w.Changes():
		case <-time.After(testing.LongWait):
			c.Fatalf("expected event from watcher by now")
		}
	}
}

func (s *MeterStateSuite) TestMeterStatusWithAmberMetricsManager(c *gc.C) {
	for i := 0; i < 3; i++ {
		err := s.metricsManager.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	err := s.metricsManager.SetMetricsManagerSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	code, _ := s.metricsManager.MeterStatus()
	c.Assert(code, gc.Equals, "AMBER")
	err = s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	code, info, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(code, gc.Equals, "AMBER")
	c.Assert(info, gc.Equals, "failed to send metrics")
}

// TestMeterStatusMetricsManagerCombinations test every possible combination
// of meter status from the unit and the metrics manager.
func (s *MeterStateSuite) TestMeterStatusMetricsManagerCombinations(c *gc.C) {
	greenMetricsMangager := func() {}
	amberMetricsManager := func() {
		err := s.metricsManager.SetLastSuccessfulSend(time.Now())
		c.Assert(err, jc.ErrorIsNil)
		for i := 0; i < 3; i++ {
			err := s.metricsManager.IncrementConsecutiveErrors()
			c.Assert(err, jc.ErrorIsNil)
		}
		code, _ := s.metricsManager.MeterStatus()
		c.Assert(code, gc.Equals, state.MeterAmber)
	}
	redMetricsManager := func() {
		err := s.metricsManager.SetLastSuccessfulSend(time.Now().Add(-14 * 24 * time.Hour))
		c.Assert(err, jc.ErrorIsNil)
		for i := 0; i < 3; i++ {
			err := s.metricsManager.IncrementConsecutiveErrors()
			c.Assert(err, jc.ErrorIsNil)
		}
		code, _ := s.metricsManager.MeterStatus()
		c.Assert(code, gc.Equals, state.MeterRed)
	}
	greenUnit := func() {
		err := s.unit.SetMeterStatus("GREEN", "Unit")
		c.Assert(err, jc.ErrorIsNil)
	}
	amberUnit := func() {
		err := s.unit.SetMeterStatus("AMBER", "Unit")
		c.Assert(err, jc.ErrorIsNil)
	}
	redUnit := func() {
		err := s.unit.SetMeterStatus("RED", "Unit")
		c.Assert(err, jc.ErrorIsNil)
	}

	tests := []struct {
		metricsManager func()
		unit           func()
		expectedCode   string
		expectedInfo   string
	}{
		{
			greenMetricsMangager,
			greenUnit,
			"GREEN",
			"Unit",
		},
		{
			amberMetricsManager,
			amberUnit,
			"AMBER",
			"Unit",
		},
		{
			redMetricsManager,
			redUnit,
			"RED",
			"failed to send metrics, exceeded grace period",
		},
		{
			redMetricsManager,
			amberUnit,
			"RED",
			"failed to send metrics, exceeded grace period",
		},

		{
			redMetricsManager,
			greenUnit,
			"RED",
			"failed to send metrics, exceeded grace period",
		},
		{
			amberMetricsManager,
			redUnit,
			"RED",
			"Unit",
		},
		{
			amberMetricsManager,
			greenUnit,
			"AMBER",
			"failed to send metrics",
		},
		{
			greenMetricsMangager,
			redUnit,
			"RED",
			"Unit",
		},
		{
			greenMetricsMangager,
			amberUnit,
			"AMBER",
			"Unit",
		},
	}

	for i, test := range tests {
		c.Logf("running test %d", i)
		test.metricsManager()
		test.unit()
		code, info, err := s.unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(code, gc.Equals, test.expectedCode)
		c.Check(info, gc.Equals, test.expectedInfo)
	}
	err := s.metricsManager.SetMetricsManagerSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	code, _ := s.metricsManager.MeterStatus()
	c.Assert(code, gc.Equals, "AMBER")
	err = s.unit.SetMeterStatus("RED", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	code, info, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(code, gc.Equals, "RED")
	c.Assert(info, gc.Equals, "Information.")
}
