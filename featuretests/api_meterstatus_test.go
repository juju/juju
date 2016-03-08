// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/meterstatus"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	factory "github.com/juju/juju/testing/factory"
	"github.com/juju/juju/watcher/watchertest"
)

type meterStatusIntegrationSuite struct {
	jujutesting.JujuConnSuite

	status meterstatus.MeterStatusClient
	unit   *state.Unit
}

var _ = gc.Suite(&meterStatusIntegrationSuite{})

func (s *meterStatusIntegrationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	f := factory.NewFactory(s.State)
	s.unit = f.MakeUnit(c, nil)

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	state := s.OpenAPIAs(c, s.unit.UnitTag(), password)
	s.status = meterstatus.NewClient(state, s.unit.UnitTag())
	c.Assert(s.status, gc.NotNil)
}

func (s *meterStatusIntegrationSuite) TestMeterStatus(c *gc.C) {
	code, info, err := s.status.MeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(code, gc.Equals, "AMBER")
	c.Assert(info, gc.Equals, "not set")

	err = s.unit.SetMeterStatus("RED", "some status")
	c.Assert(err, jc.ErrorIsNil)

	code, info, err = s.status.MeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(code, gc.Equals, "RED")
	c.Assert(info, gc.Equals, "some status")
}

func (s *meterStatusIntegrationSuite) TestWatchMeterStatus(c *gc.C) {
	w, err := s.status.WatchMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	wc.AssertOneChange()

	err = s.unit.SetMeterStatus("GREEN", "ok")
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetMeterStatus("AMBER", "ok")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.unit.SetMeterStatus("AMBER", "ok")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	mm, err := s.State.MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
	err = mm.SetLastSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	for i := 0; i < 3; i++ {
		err := mm.IncrementConsecutiveErrors()
		c.Assert(err, jc.ErrorIsNil)
	}
	status := mm.MeterStatus()
	c.Assert(status.Code, gc.Equals, state.MeterAmber) // Confirm meter status has changed
	wc.AssertOneChange()
}
