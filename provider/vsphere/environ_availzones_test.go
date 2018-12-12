// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/vim25/mo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

type environAvailzonesSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environAvailzonesSuite{})

func (s *environAvailzonesSuite) TestAvailabilityZones(c *gc.C) {
	s.client.computeResources = []*mo.ComputeResource{
		newComputeResource("z1"),
		newComputeResource("z2"),
	}

	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zones), gc.Equals, 2)
	c.Assert(zones[0].Name(), gc.Equals, "z1")
	c.Assert(zones[1].Name(), gc.Equals, "z2")
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	z1 := newComputeResource("z1")
	z2 := newComputeResource("z2")
	s.client.computeResources = []*mo.ComputeResource{z1, z2}

	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").resourcePool(z2.ResourcePool).vm(),
		buildVM("inst-1").resourcePool(z1.ResourcePool).vm(),
		buildVM("inst-2").vm(),
	}
	ids := []instance.Id{"inst-0", "inst-1", "inst-2", "inst-3"}

	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.InstanceAvailabilityZoneNames(s.callCtx, ids)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(zones, jc.DeepEquals, []string{"z2", "z1", "", ""})
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNamesNoInstances(c *gc.C) {
	zonedEnviron := s.env.(common.ZonedEnviron)
	_, err := zonedEnviron.InstanceAvailabilityZoneNames(s.callCtx, []instance.Id{"inst-0"})
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZones(c *gc.C) {
	s.client.computeResources = []*mo.ComputeResource{
		newComputeResource("test-available"),
	}

	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{Placement: "zone=test-available"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"test-available"})
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZonesUnknown(c *gc.C) {
	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{Placement: "zone=test-unknown"})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unknown" not found`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZonesInvalidPlacement(c *gc.C) {
	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{
			Placement: "invalid-placement",
		})
	c.Assert(err, gc.ErrorMatches, `unknown placement directive: invalid-placement`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAvailzonesSuite) TestAvailabilityZonesPermissionError(c *gc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.ProviderCallContext) error {
		zonedEnv := s.env.(common.ZonedEnviron)
		_, err := zonedEnv.AvailabilityZones(ctx)
		return err
	})
}
