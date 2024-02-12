// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/agent/meterstatus"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/password"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type MeterStatusIntegrationSuite struct {
	jujutesting.ApiServerSuite

	status meterstatus.MeterStatusClient
	mm     *state.MetricsManager
	unit   *state.Unit
}

var _ = gc.Suite(&MeterStatusIntegrationSuite{})

func (s *MeterStatusIntegrationSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.unit = f.MakeUnit(c, nil)

	password, err := password.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	state := s.OpenModelAPIAs(c, s.ControllerModelUUID(), s.unit.UnitTag(), password, "")
	s.status = meterstatus.NewClient(state, s.unit.UnitTag())
	c.Assert(s.status, gc.NotNil)

	// Ask for the MetricsManager as part of setup, so the metrics
	// document is created before any of the tests care.
	s.mm, err = s.ControllerModel(c).State().MetricsManager()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MeterStatusIntegrationSuite) TestMeterStatus(c *gc.C) {
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

func (s *MeterStatusIntegrationSuite) TestWatchMeterStatus(c *gc.C) {
	w, err := s.status.WatchMeterStatus()
	c.Assert(err, jc.ErrorIsNil)
	wc := watchertest.NewNotifyWatcherC(c, w)
	defer wc.AssertStops()

	// Initial event.
	wc.AssertOneChange()

	err = s.unit.SetMeterStatus("AMBER", "ok")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// Non-change is not reported.
	err = s.unit.SetMeterStatus("AMBER", "ok")
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	err = s.mm.SetLastSuccessfulSend(time.Now())
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertOneChange()

	// meter status does not change on every failed
	// attempt to send metrics - on three consecutive
	// fails, we get a meter status change
	err = s.mm.IncrementConsecutiveErrors()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mm.IncrementConsecutiveErrors()
	c.Assert(err, jc.ErrorIsNil)

	err = s.mm.IncrementConsecutiveErrors()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertOneChange()
}
