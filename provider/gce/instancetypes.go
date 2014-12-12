// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs/instances"
)

var vtype = "docker"

// TODO(wwitzel3) do we need to set a RootDisk?
var allInstanceTypes = []instances.InstanceType{
	{
		Name:     "n1-standard-1",
		CpuCores: 1,
		CpuPower: instances.CpuPower(100),
		Mem:      3750,
		VirtType: &vtype,
	}, {
		Name:     "n1-highcpu-2",
		CpuCores: 2,
		CpuPower: instances.CpuPower(100),
		Mem:      1800,
		VirtType: &vtype,
	},
}
