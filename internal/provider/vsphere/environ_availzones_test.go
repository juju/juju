// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	"context"

	"github.com/juju/tc"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/vsphere/internal/vsphereclient"
)

type environAvailzonesSuite struct {
	EnvironFixture
}

var _ = tc.Suite(&environAvailzonesSuite{})

func makeFolders(host string) *object.DatacenterFolders {
	return &object.DatacenterFolders{
		HostFolder: &object.Folder{
			Common: object.Common{
				InventoryPath: host,
			},
		},
	}
}

func (s *environAvailzonesSuite) TestAvailabilityZones(c *tc.C) {
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

	c.Assert(s.env, tc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(zones), tc.Equals, 6)
	// No zones for the empty resource.
	c.Assert(zones[0].Name(), tc.Equals, "z1")
	c.Assert(zones[1].Name(), tc.Equals, "z2")
	c.Assert(zones[2].Name(), tc.Equals, "z2/child")
	c.Assert(zones[3].Name(), tc.Equals, "z2/child/nested")
	c.Assert(zones[4].Name(), tc.Equals, "z2/child/nested/other")
	c.Assert(zones[5].Name(), tc.Equals, "z2/Other/thing")
}

func (s *environAvailzonesSuite) TestAvailabilityZonesInFolder(c *tc.C) {
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

	c.Assert(s.env, tc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)
	zones, err := zonedEnviron.AvailabilityZones(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(zones), tc.Equals, 3)
	c.Assert(zones[0].Name(), tc.Equals, "Folder/z1")
	c.Assert(zones[1].Name(), tc.Equals, "Folder/z1/ResPool1")
	c.Assert(zones[2].Name(), tc.Equals, "Folder/z1/ResPool2")
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNames(c *tc.C) {
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
	zones, err := zonedEnviron.InstanceAvailabilityZoneNames(c.Context(), ids)
	c.Assert(err, tc.Equals, environs.ErrPartialInstances)
	c.Assert(zones, tc.DeepEquals, map[instance.Id]string{
		"inst-0": "z2",
		"inst-1": "z1",
		"inst-3": "z3/child",
	})
}

func (s *environAvailzonesSuite) TestInstanceAvailabilityZoneNamesNoInstances(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	zonedEnviron := s.env.(common.ZonedEnviron)
	_, err := zonedEnviron.InstanceAvailabilityZoneNames(c.Context(), []instance.Id{"inst-0"})
	c.Assert(err, tc.Equals, environs.ErrNoInstances)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZones(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	s.client.computeResources = []vsphereclient.ComputeResource{
		{Resource: newComputeResource("test-available"), Path: "/DC/host/test-available"},
	}
	s.client.resourcePools = map[string][]*object.ResourcePool{
		"/DC/host/test-available/...": {makeResourcePool("pool-23", "/DC/host/test-available/Resources")},
	}

	c.Assert(s.env, tc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{Placement: "zone=test-available"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(zones, tc.DeepEquals, []string{"test-available"})
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZonesUnknown(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	c.Assert(s.env, tc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{Placement: "zone=test-unknown"})
	c.Assert(err, tc.ErrorMatches, `availability zone "test-unknown" not found`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAvailzonesSuite) TestDeriveAvailabilityZonesInvalidPlacement(c *tc.C) {
	s.client.folders = makeFolders("/DC/host")
	c.Assert(s.env, tc.Implements, new(common.ZonedEnviron))
	zonedEnviron := s.env.(common.ZonedEnviron)

	zones, err := zonedEnviron.DeriveAvailabilityZones(
		c.Context(),
		environs.StartInstanceParams{
			Placement: "invalid-placement",
		})
	c.Assert(err, tc.ErrorMatches, `unknown placement directive: invalid-placement`)
	c.Assert(zones, tc.HasLen, 0)
}

func (s *environAvailzonesSuite) TestAvailabilityZonesPermissionError(c *tc.C) {
	AssertInvalidatesCredential(c, s.client, func(ctx context.Context) error {
		zonedEnv := s.env.(common.ZonedEnviron)
		_, err := zonedEnv.AvailabilityZones(ctx)
		return err
	})
}
