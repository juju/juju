// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-11-01/compute"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/testing"
)

type InstanceTypeSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&InstanceTypeSuite{})

func (s *InstanceTypeSuite) TestNoDupes(c *gc.C) {
	names := set.NewStrings()
	for _, n := range machineSizeCost {
		if names.Contains(n) {
			c.Fatalf("duplicate size name %q", n)
		}
		names.Add(n)
	}
}

func (s *InstanceTypeSuite) TestStandard(c *gc.C) {
	vm := compute.VirtualMachineSize{
		Name:           to.StringPtr("Standard_A2"),
		MemoryInMB:     to.Int32Ptr(100),
		NumberOfCores:  to.Int32Ptr(2),
		OsDiskSizeInMB: to.Int32Ptr(1024 * 1024),
	}
	inst := newInstanceType(vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2",
		Name:     "Standard_A2",
		Arches:   []string{"amd64"},
		VirtType: to.StringPtr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     700, // 7 * 100
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestStandardVersioned(c *gc.C) {
	vm := compute.VirtualMachineSize{
		Name:           to.StringPtr("Standard_A2_v4"),
		MemoryInMB:     to.Int32Ptr(100),
		NumberOfCores:  to.Int32Ptr(2),
		OsDiskSizeInMB: to.Int32Ptr(1024 * 1024),
	}
	inst := newInstanceType(vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2_v4",
		Name:     "Standard_A2_v4",
		Arches:   []string{"amd64"},
		VirtType: to.StringPtr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     696, // 7 * 100 - 4
		RootDisk: 1000 * 1000,
	})
}
