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
	c.Assert(status, gc.Equals, state.MeterNotSet)
	c.Assert(info, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetMeterStatus("GREEN", "Additional information.")
	c.Assert(err, jc.ErrorIsNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, state.MeterGreen)
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
	c.Assert(status, gc.Equals, state.MeterNotSet)
	c.Assert(info, gc.Equals, "")
	c.Assert(err, jc.ErrorIsNil)

	err = s.unit.SetMeterStatus("this-is-not-a-valid-status", "Additional information.")
	c.Assert(err, gc.NotNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, state.MeterNotSet)
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
		c.Assert(status, gc.Equals, state.MeterNotSet)
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
	c.Assert(code, gc.Equals, state.MeterNotAvailable)
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

func assertMetricsManagerAmberState(c *gc.C, metricsManager *state.MetricsManager) {
	err := metricsManager.SetLastSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := metricsManager.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	code, _ := metricsManager.MeterStatus()
	c.Assert(code, gc.Equals, state.MeterAmber)
}

func assertMetricsManagerRedState(c *gc.C, metricsManager *state.MetricsManager) {
	err := metricsManager.SetLastSuccessfulSend(time.Now().Add(-14 * 24 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := metricsManager.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	code, _ := metricsManager.MeterStatus()
	c.Assert(code, gc.Equals, state.MeterRed)
}

// TestMeterStatusMetricsManagerCombinations test every possible combination
// of meter status from the unit and the metrics manager.
func (s *MeterStateSuite) TestMeterStatusMetricsManagerCombinations(c *gc.C) {
	greenMetricsMangager := func() {}
	amberMetricsManager := func() {
		assertMetricsManagerAmberState(c, s.metricsManager)
	}
	redMetricsManager := func() {
		assertMetricsManagerRedState(c, s.metricsManager)
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
		about          string
		metricsManager func()
		unit           func()
		expectedCode   state.MeterStatusCode
		expectedInfo   string
	}{
		{
			"green metrics manager and green unit returns green overall",
			greenMetricsMangager,
			greenUnit,
			state.MeterGreen,
			"Unit",
		},
		{
			"amber metrics manager and amber unit returns amber overall",
			amberMetricsManager,
			amberUnit,
			state.MeterAmber,
			"Unit",
		},
		{
			"red metrics manager and red unit returns red overall",
			redMetricsManager,
			redUnit,
			state.MeterRed,
			"failed to send metrics, exceeded grace period",
		},
		{

			"red metrics manager and amber unit returns red overall",
			redMetricsManager,
			amberUnit,
			state.MeterRed,
			"failed to send metrics, exceeded grace period",
		},

		{
			"red metrics manager and green unit returns red overall",
			redMetricsManager,
			greenUnit,
			state.MeterRed,
			"failed to send metrics, exceeded grace period",
		},
		{
			"amber metrics manager and red unit returns red overall",
			amberMetricsManager,
			redUnit,
			state.MeterRed,
			"Unit",
		},
		{
			"amber metrics manager and green unit returns amber overall",
			amberMetricsManager,
			greenUnit,
			state.MeterAmber,
			"failed to send metrics",
		},
		{
			"green metrics manager and red unit returns red overall",
			greenMetricsMangager,
			redUnit,
			state.MeterRed,
			"Unit",
		},
		{
			"green metrics manager and amber unit returns amber overall",
			greenMetricsMangager,
			amberUnit,
			state.MeterAmber,
			"Unit",
		},
	}

	for i, test := range tests {
		c.Logf("running test %d %s", i, test.about)
		test.metricsManager()
		test.unit()
		code, info, err := s.unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(code, gc.Equals, test.expectedCode)
		c.Check(info, gc.Equals, test.expectedInfo)
	}
}

func (s *MeterStateSuite) TestMeterStatusCombination(c *gc.C) {
	var (
		Red          = state.MeterStatus{state.MeterRed, ""}
		Amber        = state.MeterStatus{state.MeterAmber, ""}
		Green        = state.MeterStatus{state.MeterGreen, ""}
		NotSet       = state.MeterStatus{state.MeterNotSet, ""}
		NotAvailable = state.MeterStatus{state.MeterNotAvailable, ""}
	)
	c.Assert(state.CombineMeterStatus(Red, Red), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Red, Amber), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Red, Green), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Red, NotSet), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Red, NotAvailable), gc.DeepEquals, NotAvailable)

	c.Assert(state.CombineMeterStatus(Amber, Red), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Amber, Amber), gc.DeepEquals, Amber)
	c.Assert(state.CombineMeterStatus(Amber, Green), gc.DeepEquals, Amber)
	c.Assert(state.CombineMeterStatus(Amber, NotSet), gc.DeepEquals, Amber)
	c.Assert(state.CombineMeterStatus(Amber, NotAvailable), gc.DeepEquals, NotAvailable)

	c.Assert(state.CombineMeterStatus(Green, Red), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(Green, Amber), gc.DeepEquals, Amber)
	c.Assert(state.CombineMeterStatus(Green, Green), gc.DeepEquals, Green)
	c.Assert(state.CombineMeterStatus(Green, NotSet), gc.DeepEquals, NotSet)
	c.Assert(state.CombineMeterStatus(Green, NotAvailable), gc.DeepEquals, NotAvailable)

	c.Assert(state.CombineMeterStatus(NotSet, Red), gc.DeepEquals, Red)
	c.Assert(state.CombineMeterStatus(NotSet, Amber), gc.DeepEquals, Amber)
	c.Assert(state.CombineMeterStatus(NotSet, Green), gc.DeepEquals, NotSet)
	c.Assert(state.CombineMeterStatus(NotSet, NotSet), gc.DeepEquals, NotSet)
	c.Assert(state.CombineMeterStatus(NotSet, NotAvailable), gc.DeepEquals, NotAvailable)

	c.Assert(state.CombineMeterStatus(NotAvailable, Red), gc.DeepEquals, NotAvailable)
	c.Assert(state.CombineMeterStatus(NotAvailable, Amber), gc.DeepEquals, NotAvailable)
	c.Assert(state.CombineMeterStatus(NotAvailable, Green), gc.DeepEquals, NotAvailable)
	c.Assert(state.CombineMeterStatus(NotAvailable, NotSet), gc.DeepEquals, NotAvailable)
	c.Assert(state.CombineMeterStatus(NotAvailable, NotAvailable), gc.DeepEquals, NotAvailable)
}
