// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/vsphere/internal/vsphereclient"
)

type environAvailzonesSuite struct {
	EnvironFixture
}

var _ = gc.Suite(&environAvailzonesSuite{})

func makeFolders(host string) *object.DatacenterFolders {
	return &object.DatacenterFolders{
		HostFolder: &object.Folder{
			Common: object.Common{
				InventoryPath: host,
			},
		},
	}
}

func (s *environAvailzonesSuite) TestAvailabilityZones(c *gc.C) {
	emptyResource := newComputeResource("empty")
	emptyResource.Summary.(*mockSummary).EffectiveCpu = 0
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: emptyResource, Path: "/DC/host/empty"},
		{Resource: newComputeResource("z1"), Path: "/DC/host/z1"},
		{Resource: newComputeResource("z2"), Path: "/DC/host/z2"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {
			makeResourcePool("pool-1", "/DC/host/z1/Resources"),
		},
		"/DC/host/z2/...": {
			// Check we don't get broken by trailing slashes.
			makeResourcePool("pool-2", "/DC/host/z2/Resources/"),
			makeResourcePool("pool-3", "/DC/host/z2/Resources/child"),
			makeResourcePool("pool-4", "/DC/host/z2/Resources/child/nested"),
			makeResourcePool("pool-5", "/DC/host/z2/Resources/child/nested/other/"),
			makeResourcePool("pool-6", "/DC/host/z2/Other/thing"),
		},
	}

	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zones), gc.Equals, 6)
	// No zones for the empty resource.
	c.Assert(zones[0].Name(), gc.Equals, "z1")
	c.Assert(zones[1].Name(), gc.Equals, "z2")
	c.Assert(zones[2].Name(), gc.Equals, "z2/child")
	c.Assert(zones[3].Name(), gc.Equals, "z2/child/nested")
	c.Assert(zones[4].Name(), gc.Equals, "z2/child/nested/other")
	c.Assert(zones[5].Name(), gc.Equals, "z2/Other/thing")
}

func (s *environAvailzonesSuite) TestAvailabilityZonesInFolder(c *gc.C) {
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("z1"), Path: "/DC/host/Folder/z1"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/Folder/z1/...": {
			makeResourcePool("pool-1", "/DC/host/Folder/z1/Resources"),
			makeResourcePool("pool-2", "/DC/host/Folder/z1/Resources/ResPool1"),
			makeResourcePool("pool-3", "/DC/host/Folder/z1/Resources/ResPool2"),
		},
	}

	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zones), gc.Equals, 3)
	c.Assert(zones[0].Name(), gc.Equals, "Folder/z1")
	c.Assert(zones[1].Name(), gc.Equals, "Folder/z1/ResPool1")
	c.Assert(zones[2].Name(), gc.Equals, "Folder/z1/ResPool2")
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNames(c *gc.C) {
	z1 := newComputeResource("z1")
	z2 := newComputeResource("z2")
	z3 := newComputeResource("z3")
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: z1, Path: "/DC/host/z1"},
		{Resource: z2, Path: "/DC/host/z2"},
		{Resource: z3, Path: "/DC/host/z3"},
	}

	childPool := makeResourcePool("rp-child", "/DC/host/z3/Resources/child")
	childRef := childPool.Reference()
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/z1/...": {makeResourcePool("rp-z1", "/DC/host/z1/Resources")},
		"/DC/host/z2/...": {makeResourcePool("rp-z2", "/DC/host/z2/Resources")},
		"/DC/host/z3/...": {
			makeResourcePool("rp-z3", "/DC/host/z3/Resources"),
			childPool,
		},
	}

	s.client.virtualMachines = []*mo.VirtualMachine{
		buildVM("inst-0").resourcePool(z2.ResourcePool).vm(),
		buildVM("inst-1").resourcePool(z1.ResourcePool).vm(),
		buildVM("inst-2").vm(),
		buildVM("inst-3").resourcePool(&childRef).vm(),
	}
	ids := []instance.Id{"inst-0", "inst-1", "inst-2", "inst-3", "inst-4"}

	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.InstanceAvailabilityZoneNames(s.callCtx, ids)
	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(zones, jc.DeepEquals, map[instance.Id]string{
		"inst-0": "z2",
		"inst-1": "z1",
		"inst-3": "z3/child",
	})
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNamesNoInstances(c *gc.C) {
	s.client.folders = makeFolders("/DC/host")
	zonedEnviron := s.env.(common.ZonedEnviron)
	_, err := zonedEnviron.InstanceAvailabilityZoneNames(s.callCtx, []instance.Id{"inst-0"})
	c.Assert(err, gc.Equals, environs.ErrNoInstances)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZones(c *gc.C) {
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("test-available"), Path: "/DC/host/test-available"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/test-available/...": {makeResourcePool("pool-23", "/DC/host/test-available/Resources")},
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
	s.client.folders = makeFolders("/DC/host")
	c.Assert(s.env, gc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		s.callCtx,
		environs.StartInstanceParams{Placement: "zone=test-unknown"})
	c.Assert(err, gc.ErrorMatches, `availability zone "test-unknown" not found`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZonesInvalidPlacement(c *gc.C) {
	s.client.folders = makeFolders("/DC/host")
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
