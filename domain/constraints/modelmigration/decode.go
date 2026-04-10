// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

// DecodeConstraints decodes a description.Constraints into constraints.Value.
func DecodeConstraints(cons description.Constraints) constraints.Value {
	res := constraints.Value{}

	if cons == nil {
		return res
	}

	if allocate := cons.AllocatePublicIP(); allocate {
		res.AllocatePublicIP = &allocate
	}
	if arch := cons.Architecture(); arch != "" {
		res.Arch = &arch
	}
	if container := instance.ContainerType(cons.Container()); container != "" {
		res.Container = &container
	}
	if cpuCores := cons.CpuCores(); cpuCores != 0 {
		res.CpuCores = &cpuCores
	}
	if cpuPower := cons.CpuPower(); cpuPower != 0 {
		res.CpuPower = &cpuPower
	}
	if instanceType := cons.InstanceType(); instanceType != "" {
		res.InstanceType = &instanceType
	}
	if memory := cons.Memory(); memory != 0 {
		res.Mem = &memory
	}
	if imageID := cons.ImageID(); imageID != "" {
		res.ImageID = &imageID
	}
	if rootDisk := cons.RootDisk(); rootDisk != 0 {
		res.RootDisk = &rootDisk
	}
	if rootDiskSource := cons.RootDiskSource(); rootDiskSource != "" {
		res.RootDiskSource = &rootDiskSource
	}
	if spaces := cons.Spaces(); len(spaces) > 0 {
		res.Spaces = &spaces
	}
	if tags := cons.Tags(); len(tags) > 0 {
		res.Tags = &tags
	}
	if virtType := cons.VirtType(); virtType != "" {
		res.VirtType = &virtType
	}
	if zones := cons.Zones(); len(zones) > 0 {
		res.Zones = &zones
	}

	return res
}
