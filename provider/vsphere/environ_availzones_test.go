// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere"
)

type environAvailzonesSuite struct {
	vsphere.BaseSuite
}

var _ = gc.Suite(&environAvailzonesSuite{})

func (s *environAvailzonesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *environAvailzonesSuite) TestAvailabilityZones(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	s.FakeAvailabilityZones(client, "z1", "z2")
	zones, err := s.Env.AvailabilityZones()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zones), gc.Equals, 2)
	c.Assert(zones[0].Name(), gc.Equals, "z1")
	c.Assert(zones[1].Name(), gc.Equals, "z2")
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	vmName := common.MachineFullName(s.Env, "1")
	s.FakeInstancesWithResourcePool(client, vsphere.InstRp{Inst: vmName, Rp: "rp1"})
	s.FakeAvailabilityZonesWithResourcePool(client, vsphere.ZoneRp{Zone: "z1", Rp: "rp1"}, vsphere.ZoneRp{Zone: "z2", Rp: "rp2"})

	zones, err := s.Env.InstanceAvailabilityZoneNames([]instance.Id{instance.Id(vmName)})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zones), gc.Equals, 1)
	c.Assert(zones[0], gc.Equals, "z1")
}
