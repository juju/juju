// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type MeterStateSuite struct {
	ConnSuite
	unit           *state.Unit
	metricsManager *state.MetricsManager
}

var _ = gc.Suite(&MeterStateSuite{})

func (s *MeterStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)
	c.Assert(s.unit.Base(), jc.DeepEquals, state.Base{OS: "ubuntu", Channel: "12.10/stable"})
	var err error
	s.metricsManager, err = s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	// Before we get into the tests, ensure that all the creation events have flowed through the system.
	s.WaitForModelWatchersIdle(c, s.Model.UUID())
}

func (s *MeterStateSuite) TestMeterStatus(c *gc.C) {
	status, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)
	err = s.unit.SetMeterStatus("GREEN", "Additional information.")
	c.Assert(err, jc.ErrorIsNil)
	status, err = s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterGreen)
}

func (s *MeterStateSuite) TestMeterStatusIncludesModelUUID(c *gc.C) {
	jujuDB := s.MgoSuite.Session.DB("juju")
	meterStatus := jujuDB.C("meterStatus")
	var docs []bson.M
	err := meterStatus.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	// we now expect two meter status docs - one for the unit and one
	// for the model - both should have the model-uuid filled in.
	c.Assert(docs, gc.HasLen, 2)
	c.Assert(docs[0]["model-uuid"], gc.Equals, s.State.ModelUUID())
	c.Assert(docs[1]["model-uuid"], gc.Equals, s.State.ModelUUID())
}

func (s *MeterStateSuite) TestSetMeterStatusIncorrect(c *gc.C) {
	err := s.unit.SetMeterStatus("NOT SET", "Additional information.")
	c.Assert(err, gc.ErrorMatches, `meter status "NOT SET" not valid`)
	status, err := s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)

	err = s.unit.SetMeterStatus("this-is-not-a-valid-status", "Additional information.")
	c.Assert(err, gc.ErrorMatches, `meter status "this-is-not-a-valid-status" not valid`)
	status, err = s.unit.GetMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(status.Code, gc.Equals, state.MeterNotSet)
}

func (s *MeterStateSuite) TestSetMeterStatusWhenDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, state.NewObjectStore(c, s.State), s.unit, ".*"+contentionErr, contentionErr, func() error {
		err := s.unit.SetMeterStatus("GREEN", "Additional information.")
		if err != nil {
			return err
		}
		status, err := s.unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(status.Code, gc.Equals, state.MeterNotSet)
		return nil
	})
}

func (s *MeterStateSuite) TestMeterStatusRemovedWithUnit(c *gc.C) {
	err := s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.Remove(state.NewObjectStore(c, s.State))
	c.Assert(err, jc.ErrorIsNil)
	status, err := s.unit.GetMeterStatus()
	c.Assert(err, gc.ErrorMatches, "cannot retrieve meter status for unit .*: not found")
	c.Assert(status.Code, gc.Equals, state.MeterNotAvailable)
}

func (s *MeterStateSuite) TestMeterStatusWatcherRespondstoMeterStatus(c *gc.C) {
	watcher := s.unit.WatchMeterStatus()
	wc := statetesting.NewNotifyWatcherC(c, watcher)
	wc.AssertOneChange()
	err := s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *MeterStateSuite) TestMeterStatusWatcherRespondsToMetricsManager(c *gc.C) {
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	watcher := s.unit.WatchMeterStatus()
	wc := statetesting.NewNotifyWatcherC(c, watcher)
	wc.AssertOneChange()
	err = mm.SetLastSuccessfulSend(s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	for i := 0; i < 3; i++ {
		err := mm.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	status := mm.MeterStatus()
	c.Assert(status.Code, gc.Equals, state.MeterAmber)
	wc.AssertOneChange()
}

func (s *MeterStateSuite) TestMeterStatusWatcherRespondsToMetricsManagerAndStatus(c *gc.C) {
	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	watcher := s.unit.WatchMeterStatus()
	wc := statetesting.NewNotifyWatcherC(c, watcher)
	wc.AssertOneChange()
	err = mm.SetLastSuccessfulSend(s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
	for i := 0; i < 3; i++ {
		err := mm.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	status := mm.MeterStatus()
	c.Assert(status.Code, gc.Equals, state.MeterAmber)
	wc.AssertOneChange()
	err = s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()
}

func (s *MeterStateSuite) assertMetricsManagerAmberState(c *gc.C, metricsManager *state.MetricsManager) {
	err := metricsManager.SetLastSuccessfulSend(s.Clock.Now())
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := metricsManager.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	status := metricsManager.MeterStatus()
	c.Assert(status.Code, gc.Equals, state.MeterAmber, gc.Commentf("got meter status %#v", status))
}

// TODO (mattyw) This function could be moved into a metricsmanager testing package.
func (s *MeterStateSuite) assertMetricsManagerRedState(c *gc.C, metricsManager *state.MetricsManager) {
	// To enter the red state we need to set a last successful send as over 1 week ago
	err := metricsManager.SetLastSuccessfulSend(s.Clock.Now().Add(-8 * 24 * time.Hour))
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := metricsManager.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	status := metricsManager.MeterStatus()
	c.Assert(status.Code, gc.Equals, state.MeterRed, gc.Commentf("got meter status %#v", status))
}

// TestMeterStatusMetricsManagerCombinations test every possible combination
// of meter status from the unit and the metrics manager.
func (s *MeterStateSuite) TestMeterStatusMetricsManagerCombinations(c *gc.C) {
	greenMetricsMangager := func() {}
	amberMetricsManager := func() {
		s.assertMetricsManagerAmberState(c, s.metricsManager)
	}
	redMetricsManager := func() {
		s.assertMetricsManagerRedState(c, s.metricsManager)
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
	}{{
		"green metrics manager and green unit returns green overall",
		greenMetricsMangager,
		greenUnit,
		state.MeterGreen,
		"Unit",
	}, {
		"amber metrics manager and amber unit returns amber overall",
		amberMetricsManager,
		amberUnit,
		state.MeterAmber,
		"Unit",
	}, {
		"red metrics manager and red unit returns red overall",
		redMetricsManager,
		redUnit,
		state.MeterRed,
		"failed to send metrics, exceeded grace period",
	}, {

		"red metrics manager and amber unit returns red overall",
		redMetricsManager,
		amberUnit,
		state.MeterRed,
		"failed to send metrics, exceeded grace period",
	}, {
		"red metrics manager and green unit returns red overall",
		redMetricsManager,
		greenUnit,
		state.MeterRed,
		"failed to send metrics, exceeded grace period",
	}, {
		"amber metrics manager and red unit returns red overall",
		amberMetricsManager,
		redUnit,
		state.MeterRed,
		"Unit",
	}, {
		"amber metrics manager and green unit returns amber overall",
		amberMetricsManager,
		greenUnit,
		state.MeterAmber,
		"failed to send metrics",
	}, {
		"green metrics manager and red unit returns red overall",
		greenMetricsMangager,
		redUnit,
		state.MeterRed,
		"Unit",
	}, {
		"green metrics manager and amber unit returns amber overall",
		greenMetricsMangager,
		amberUnit,
		state.MeterAmber,
		"Unit",
	}}

	for i, test := range tests {
		c.Logf("running test %d %s", i, test.about)
		test.metricsManager()
		test.unit()
		status, err := s.unit.GetMeterStatus()
		c.Assert(err, jc.ErrorIsNil)
		c.Check(status.Code, gc.Equals, test.expectedCode, gc.Commentf("got meter status %#v", status))
	}
}

func (s *MeterStateSuite) TestMeterStatusCombination(c *gc.C) {
	var (
		red          = state.MeterStatus{state.MeterRed, ""}
		amber        = state.MeterStatus{state.MeterAmber, ""}
		green        = state.MeterStatus{state.MeterGreen, ""}
		notSet       = state.MeterStatus{state.MeterNotSet, ""}
		notAvailable = state.MeterStatus{state.MeterNotAvailable, ""}
	)
	c.Assert(state.CombineMeterStatus(red, red).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(red, amber).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(red, green).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(red, notSet).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(red, notAvailable).Code, gc.Equals, notAvailable.Code)

	c.Assert(state.CombineMeterStatus(amber, red).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(amber, amber).Code, gc.Equals, amber.Code)
	c.Assert(state.CombineMeterStatus(amber, green).Code, gc.Equals, amber.Code)
	c.Assert(state.CombineMeterStatus(amber, notSet).Code, gc.Equals, amber.Code)
	c.Assert(state.CombineMeterStatus(amber, notAvailable).Code, gc.Equals, notAvailable.Code)

	c.Assert(state.CombineMeterStatus(green, red).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(green, amber).Code, gc.Equals, amber.Code)
	c.Assert(state.CombineMeterStatus(green, green).Code, gc.Equals, green.Code)
	c.Assert(state.CombineMeterStatus(green, notSet).Code, gc.Equals, notSet.Code)
	c.Assert(state.CombineMeterStatus(green, notAvailable).Code, gc.Equals, notAvailable.Code)

	c.Assert(state.CombineMeterStatus(notSet, red).Code, gc.Equals, red.Code)
	c.Assert(state.CombineMeterStatus(notSet, amber).Code, gc.Equals, amber.Code)
	c.Assert(state.CombineMeterStatus(notSet, green).Code, gc.Equals, notSet.Code)
	c.Assert(state.CombineMeterStatus(notSet, notSet).Code, gc.Equals, notSet.Code)
	c.Assert(state.CombineMeterStatus(notSet, notAvailable).Code, gc.Equals, notAvailable.Code)

	c.Assert(state.CombineMeterStatus(notAvailable, red).Code, gc.Equals, notAvailable.Code)
	c.Assert(state.CombineMeterStatus(notAvailable, amber).Code, gc.Equals, notAvailable.Code)
	c.Assert(state.CombineMeterStatus(notAvailable, green).Code, gc.Equals, notAvailable.Code)
	c.Assert(state.CombineMeterStatus(notAvailable, notSet).Code, gc.Equals, notAvailable.Code)
	c.Assert(state.CombineMeterStatus(notAvailable, notAvailable).Code, gc.Equals, notAvailable.Code)
}
