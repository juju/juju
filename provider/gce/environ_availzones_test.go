// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type environAZSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environAZSuite{})

func (s *environAZSuite) TestAvailabilityZones(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp),
	}

	zones, err := s.Env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, gc.HasLen, 1)
	c.Check(zones[0].Name(), gc.Equals, "a-zone")
	c.Check(zones[0].Available(), jc.IsTrue)
}

func (s *environAZSuite) TestAvailabilityZonesAPI(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{}

	_, err := s.Env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, gc.Equals, "home")
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}

	ids := []instance.Id{instance.Id("spam")}
	zones, err := s.Env.InstanceAvailabilityZoneNames(ids)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestInstanceAvailabilityZoneNamesAPIs(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}

	ids := []instance.Id{instance.Id("spam")}
	_, err := s.Env.InstanceAvailabilityZoneNames(ids)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeEnviron.CheckCalls(c, []gce.FakeCall{{
		FuncName: "GetInstances", Args: gce.FakeCallArgs{"env": s.Env},
	}})
}

func (s *environAZSuite) TestParseAvailabilityZones(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesAPI(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp),
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeEnviron.CheckCalls(c, []gce.FakeCall{{
		FuncName: "GetInstances", Args: gce.FakeCallArgs{"env": s.Env},
	}})

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, gc.Equals, "home")
}

func (s *environAZSuite) TestParseAvailabilityZonesPlacement(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"a-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesPlacementAPI(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp),
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, gc.Equals, "home")
}

func (s *environAZSuite) TestParseAvailabilityZonesUnavailable(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusDown),
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)

	c.Check(err, gc.ErrorMatches, `.*availability zone "a-zone" is DOWN`)
}

func (s *environAZSuite) TestParseAvailabilityZonesDistGroup(c *gc.C) {
	s.StartInstArgs.DistributionGroup = func() ([]instance.Id, error) {
		// TODO(ericsnow) What goes here?
		return []instance.Id{}, nil
	}
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesOccupied(c *gc.C) {
	s.FakeEnviron.Insts = []instance.Instance{s.Instance}
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("home-zone", google.StatusUp),
		google.NewZone("home-zone-b", google.StatusUp),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone-b"})
}

func (s *environAZSuite) TestParseAvailabilityZonesWrongRegion(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		// TODO(ericsnow) This doesn't work if the order is reversed!
		google.NewZone("a-zone", google.StatusUp),
		google.NewZone("home-zone", google.StatusUp),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesNoneFound(c *gc.C) {
	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
