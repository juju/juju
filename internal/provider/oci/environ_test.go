// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

import (
	"testing"

	"github.com/juju/tc"
	ociCore "github.com/oracle/oci-go-sdk/v65/core"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/instances"
)

type environSuite struct {
}

func TestEnvironSuite(t *testing.T) {
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
			maxCpuCores: new(uint64(32)),
			maxMem:      new(uint64(512 * 1024)),
			cpuCores:    1,
			mem:         1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: new(float32(instances.MinCpuCores)),
			},
		},
		{
			name:        "flexible shape, only MaxCpuCores, no constraints => default minimum cpus",
			maxCpuCores: new(uint64(32)),
			cpuCores:    1,
			mem:         1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: new(float32(instances.MinCpuCores)),
			},
		},
		{
			name:     "flexible shape, only MaxMem, no constraints => default minimum cpus",
			maxMem:   new(uint64(512 * 1024)),
			cpuCores: 1,
			mem:      1024,
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: new(float32(instances.MinCpuCores)),
			},
		},
		{
			name:        "flexible shape, cpu constraints",
			maxCpuCores: new(uint64(32)),
			maxMem:      new(uint64(512 * 1024)),
			cpuCores:    1,
			mem:         1024,
			constraints: "cores=31",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus: new(float32(31)),
			},
		},
		{
			name:        "flexible shape, mem constraints",
			maxCpuCores: new(uint64(32)),
			maxMem:      new(uint64(512 * 1024)),
			cpuCores:    1,
			mem:         1024,
			constraints: "mem=64G",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus:       new(float32(instances.MinCpuCores)),
				MemoryInGBs: new(float32(64)),
			},
		},
		{
			name:        "flexible shape, cpu and mem constraints",
			maxCpuCores: new(uint64(32)),
			maxMem:      new(uint64(512 * 1024)),
			cpuCores:    1,
			mem:         1024,
			constraints: "cores=31 mem=64G",
			want: &ociCore.LaunchInstanceShapeConfigDetails{
				Ocpus:       new(float32(31)),
				MemoryInGBs: new(float32(64)),
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
