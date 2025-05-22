// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	stdtesting "testing"

	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/instances"
)

type environSuite struct {
}

func TestEnvironSuite(t *stdtesting.T) {
	tc.Run(t, &environSuite{})
}

func (s *environSuite) TestEnsureShapeConfig(c *tc.C) {
	type test struct {
		name                string
		maxCpuCores, maxMem *uint64
		cpuCores, mem       uint64
		constraints         string
		want                *ociCore.LaunchInstanceShapeConfigDetails
	}

	tests := []test{
		{
			name:     "not flexible shape, no constraints",
			cpuCores: 1,
			mem:      1024,
			want:     nil,
		},
		{
			name:        "flexible shape, no constraints => default minimum cpus",
			maxCpuCores: makeUint64Pointer(32),
			maxMem:      makeUint64Pointer(512 * 1024),
			cpuCores:    1,
			mem:         1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: makeFloat32Pointer(float32(instances.MinCpuCores)),
			},
		},
		{
			name:        "flexible shape, only MaxCpuCores, no constraints => default minimum cpus",
			maxCpuCores: makeUint64Pointer(32),
			cpuCores:    1,
			mem:         1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: makeFloat32Pointer(float32(instances.MinCpuCores)),
			},
		},
		{
			name:     "flexible shape, only MaxMem, no constraints => default minimum cpus",
			maxMem:   makeUint64Pointer(512 * 1024),
			cpuCores: 1,
			mem:      1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: makeFloat32Pointer(float32(instances.MinCpuCores)),
			},
		},
		{
			name:        "flexible shape, cpu constraints",
			maxCpuCores: makeUint64Pointer(32),
			maxMem:      makeUint64Pointer(512 * 1024),
			cpuCores:    1,
			mem:         1024,
			constraints: "cores=31",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: makeFloat32Pointer(31),
			},
		},
		{
			name:        "flexible shape, mem constraints",
			maxCpuCores: makeUint64Pointer(32),
			maxMem:      makeUint64Pointer(512 * 1024),
			cpuCores:    1,
			mem:         1024,
			constraints: "mem=64G",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus:       makeFloat32Pointer(float32(instances.MinCpuCores)),
				MemoryInGBs: makeFloat32Pointer(64),
			},
		},
		{
			name:        "flexible shape, cpu and mem constraints",
			maxCpuCores: makeUint64Pointer(32),
			maxMem:      makeUint64Pointer(512 * 1024),
			cpuCores:    1,
			mem:         1024,
			constraints: "cores=31 mem=64G",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus:       makeFloat32Pointer(31),
				MemoryInGBs: makeFloat32Pointer(64),
			},
		},
	}

	for _, test := range tests {
		c.Logf("test '%s'", test.name)
		instanceSpec := instances.InstanceType{
			MaxCpuCores: test.maxCpuCores,
			MaxMem:      test.maxMem,
			CpuCores:    test.cpuCores,
			Mem:         test.mem,
		}
		cons, err := constraints.Parse(test.constraints)
		c.Assert(err, tc.ErrorIsNil)
		instanceDetails := ociCore.LaunchInstanceDetails{}
		ensureShapeConfig(instanceSpec, cons, &instanceDetails)
		c.Check(instanceDetails.ShapeConfig, tc.DeepEquals, test.want)
	}
}

func makeUint64Pointer(val uint64) *uint64 {
	return &val
}

func makeFloat32Pointer(val float32) *float32 {
	return &val
}
