// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/juju/arch"
)

var (
	vtype  = "kvm"
	arches = []string{arch.AMD64}
)

// Instance types are not associated with disks in GCE, so we do not
// set RootDisk.

// TODO(ericsnow) Dynamically generate the type specs from the official
// JSON file.

// Shared-core machine types.
var allInstanceTypes = []instances.InstanceType{
	{ // Standard machine types
		Name:     "n1-standard-1",
		Arches:   arches,
		CpuCores: 1,
		CpuPower: instances.CpuPower(275),
		Mem:      3750,
		VirtType: &vtype,
	}, {
		Name:     "n1-standard-2",
		Arches:   arches,
		CpuCores: 2,
		CpuPower: instances.CpuPower(550),
		Mem:      7500,
		VirtType: &vtype,
	}, {
		Name:     "n1-standard-4",
		Arches:   arches,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1100),
		Mem:      15000,
		VirtType: &vtype,
	}, {
		Name:     "n1-standard-8",
		Arches:   arches,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2200),
		Mem:      30000,
		VirtType: &vtype,
	}, {
		Name:     "n1-standard-16",
		Arches:   arches,
		CpuCores: 16,
		CpuPower: instances.CpuPower(4400),
		Mem:      60000,
		VirtType: &vtype,
	},

	{ // High memory machine types
		Name:     "n1-highmem-2",
		Arches:   arches,
		CpuCores: 2,
		CpuPower: instances.CpuPower(550),
		Mem:      13000,
		VirtType: &vtype,
	}, {
		Name:     "n1-highmem-4",
		Arches:   arches,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1100),
		Mem:      26000,
		VirtType: &vtype,
	}, {
		Name:     "n1-highmem-8",
		Arches:   arches,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2200),
		Mem:      52000,
		VirtType: &vtype,
	}, {
		Name:     "n1-highmem-16",
		Arches:   arches,
		CpuCores: 16,
		CpuPower: instances.CpuPower(4400),
		Mem:      104000,
		VirtType: &vtype,
	},

	{ // High CPU machine types
		Name:     "n1-highcpu-2",
		Arches:   arches,
		CpuCores: 2,
		CpuPower: instances.CpuPower(550),
		Mem:      1800,
		VirtType: &vtype,
	}, {
		Name:     "n1-highcpu-4",
		Arches:   arches,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1100),
		Mem:      3600,
		VirtType: &vtype,
	}, {
		Name:     "n1-highcpu-8",
		Arches:   arches,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2200),
		Mem:      7200,
		VirtType: &vtype,
	}, {
		Name:     "n1-highcpu-16",
		Arches:   arches,
		CpuCores: 16,
		CpuPower: instances.CpuPower(4400),
		Mem:      14400,
		VirtType: &vtype,
	},

	{ // Micro and small machine types
		Name:     "f1-micro",
		Arches:   arches,
		CpuCores: 1,
		CpuPower: instances.CpuPower(0),
		Mem:      600,
		VirtType: &vtype,
	}, {
		Name:     "g1-small",
		Arches:   arches,
		CpuCores: 1,
		CpuPower: instances.CpuPower(138),
		Mem:      1700,
		VirtType: &vtype,
	},
}
