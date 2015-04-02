// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type environAZSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environAZSuite{})

func (s *environAZSuite) TestAvailabilityZones(c *gc.C) {
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
		google.NewZone("b-zone", google.StatusUp, "", ""),
	}

	zones, err := s.Env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, gc.HasLen, 2)
	c.Check(zones[0].Name(), gc.Equals, "a-zone")
	c.Check(zones[0].Available(), jc.IsTrue)
	c.Check(zones[1].Name(), gc.Equals, "b-zone")
	c.Check(zones[1].Available(), jc.IsTrue)

}

func (s *environAZSuite) TestAvailabilityZonesDeprecated(c *gc.C) {
	zone := google.NewZone("a-zone", google.StatusUp, "DEPRECATED", "b-zone")

	c.Check(zone.Deprecated(), jc.IsTrue)
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
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesAPI(c *gc.C) {
	ids := []instance.Id{s.Instance.Id()}
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: ids,
	}}
	s.StartInstArgs.DistributionGroup = func() ([]instance.Id, error) {
		return ids, nil
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
	s.FakeEnviron.CheckCalls(c, []gce.FakeCall{})
	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "AvailabilityZoneAllocations",
		Args: gce.FakeCallArgs{
			"env":   s.Env,
			"group": ids,
		},
	}})
}

func (s *environAZSuite) TestParseAvailabilityZonesPlacement(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"a-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesPlacementAPI(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusUp, "", ""),
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeEnviron.CheckCalls(c, []gce.FakeCall{})
	s.FakeCommon.CheckCalls(c, []gce.FakeCall{})
	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "AvailabilityZones")
	c.Check(s.FakeConn.Calls[0].Region, gc.Equals, "home")
}

func (s *environAZSuite) TestParseAvailabilityZonesPlacementUnavailable(c *gc.C) {
	s.StartInstArgs.Placement = "zone=a-zone"
	s.FakeConn.Zones = []google.AvailabilityZone{
		google.NewZone("a-zone", google.StatusDown, "", ""),
	}

	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)

	c.Check(err, gc.ErrorMatches, `.*availability zone "a-zone" is DOWN`)
}

func (s *environAZSuite) TestParseAvailabilityZonesDistGroup(c *gc.C) {
	s.FakeCommon.AZInstances = []common.AvailabilityZoneInstances{{
		ZoneName:  "home-zone",
		Instances: []instance.Id{s.Instance.Id()},
	}}
	s.StartInstArgs.DistributionGroup = func() ([]instance.Id, error) {
		return []instance.Id{s.Instance.Id()}, nil
	}

	zones, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(zones, jc.DeepEquals, []string{"home-zone"})
}

func (s *environAZSuite) TestParseAvailabilityZonesNoneFound(c *gc.C) {
	_, err := gce.ParseAvailabilityZones(s.Env, s.StartInstArgs)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}
