// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type MeterStateSuite struct {
	ConnSuite
	unit    *state.Unit
	factory *factory.Factory
}

var _ = gc.Suite(&MeterStateSuite{})

func (s *MeterStateSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.factory = factory.NewFactory(s.State)
	s.unit = s.factory.MakeUnit(c, nil)
	c.Assert(s.unit.Series(), gc.Equals, "quantal")
}

func (s *UnitSuite) TestMeterStatus(c *gc.C) {
	status, info, err := s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, gc.IsNil)
	err = s.unit.SetMeterStatus("GREEN", "Additional information.")
	c.Assert(err, gc.IsNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "GREEN")
	c.Assert(info, gc.Equals, "Additional information.")
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestSetMeterStatusIncorrect(c *gc.C) {
	err := s.unit.SetMeterStatus("NOT SET", "Additional information.")
	c.Assert(err, gc.NotNil)
	status, info, err := s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, gc.IsNil)

	err = s.unit.SetMeterStatus("this-is-not-a-valid-status", "Additional information.")
	c.Assert(err, gc.NotNil)
	status, info, err = s.unit.GetMeterStatus()
	c.Assert(status, gc.Equals, "NOT SET")
	c.Assert(info, gc.Equals, "")
	c.Assert(err, gc.IsNil)
}

func (s *UnitSuite) TestSetMeterStatusWhenDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	testWhenDying(c, s.unit, contentionErr, contentionErr, func() error {
		err := s.unit.SetMeterStatus("GREEN", "Additional information.")
		if err != nil {
			return err
		}
		status, info, err := s.unit.GetMeterStatus()
		c.Assert(status, gc.Equals, "NOT SET")
		c.Assert(info, gc.Equals, "")
		c.Assert(err, gc.IsNil)
		return nil
	})
}

func (s *UnitSuite) TestMeterStatusRemovedWithUnit(c *gc.C) {
	err := s.unit.SetMeterStatus("GREEN", "Information.")
	c.Assert(err, gc.IsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.unit.Remove()
	c.Assert(err, gc.IsNil)
	code, info, err := s.unit.GetMeterStatus()
	c.Assert(err, gc.ErrorMatches, "cannot retrieve meter status for unit .*: not found")
	c.Assert(code, gc.Equals, "NOT AVAILABLE")
	c.Assert(info, gc.Equals, "")
}
