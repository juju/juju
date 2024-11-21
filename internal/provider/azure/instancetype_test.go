// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corearch "github.com/juju/juju/core/arch"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/testing"
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
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Standard_A2"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.AMD64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2",
		Name:     "Standard_A2",
		Arch:     corearch.AMD64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     700, // 7 * 100
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestStandardARM64(c *gc.C) {
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Standard_A2"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.ARM64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2",
		Name:     "Standard_A2",
		Arch:     corearch.ARM64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     700, // 7 * 100
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestStandardVersioned(c *gc.C) {
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Standard_A2_v4"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.AMD64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2_v4",
		Name:     "Standard_A2_v4",
		Arch:     corearch.AMD64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     696, // 7 * 100 - 4
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestStandardPromo(c *gc.C) {
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Standard_A2_v4_Promo"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.AMD64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Standard_A2_v4_Promo",
		Name:     "Standard_A2_v4_Promo",
		Arch:     corearch.AMD64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     40300, // len(costs),
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestBasic(c *gc.C) {
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Basic_A2"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.AMD64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Basic_A2",
		Name:     "Basic_A2",
		Arch:     corearch.AMD64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     40300, // len(costs),
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestBasicARM64(c *gc.C) {
	vm := armcompute.VirtualMachineSize{
		Name:           to.Ptr("Basic_A2"),
		MemoryInMB:     to.Ptr(int32(100)),
		NumberOfCores:  to.Ptr(int32(2)),
		OSDiskSizeInMB: to.Ptr(int32(1024 * 1024)),
	}
	inst := newInstanceType(corearch.ARM64, vm)
	c.Assert(inst, jc.DeepEquals, instances.InstanceType{
		Id:       "Basic_A2",
		Name:     "Basic_A2",
		Arch:     corearch.ARM64,
		VirtType: to.Ptr("Hyper-V"),
		CpuCores: 2,
		Mem:      100,
		Cost:     40300, // len(costs),
		RootDisk: 1000 * 1000,
	})
}

func (s *InstanceTypeSuite) TestDeleteInstanceFamily(c *gc.C) {
	instanceTypes := map[string]instances.InstanceType{
		"D6_v4":          {Name: "Standard_D6_v4"},
		"Standard_D6_v4": {Name: "Standard_D6_v4"},
		"Standard_D6_v5": {Name: "Standard_D6_v5"},
		"D6_v5":          {Name: "Standard_D6_v5"},
		"Standard_A2_v2": {Name: "Standard_A2_v2"},
		"A2_v2":          {Name: "Standard_A2_v2"},
	}
	deleteInstanceFamily(instanceTypes, "Standard_D6_v5")
	c.Assert(instanceTypes, jc.DeepEquals, map[string]instances.InstanceType{
		"Standard_A2_v2": {Name: "Standard_A2_v2"},
		"A2_v2":          {Name: "Standard_A2_v2"},
	})
}
