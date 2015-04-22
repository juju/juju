// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"

	"github.com/altoros/gosigma"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
)

type sigmaConstraints struct {
	driveTemplate string
	driveSize     uint64
	cores         uint64 //number of cpu cores
	power         uint64 //cpu power in MHz
	mem           uint64 //memory size in GB
}

const (
	defaultCPUPower = 2000
	defaultDriveGB  = 5
	defaultMemoryGB = 2
)

// newConstraints creates new CloudSigma constraints from juju common constraints
func newConstraints(bootstrap bool, jc constraints.Value, img *imagemetadata.ImageMetadata) *sigmaConstraints {
	var sc = sigmaConstraints{
		driveTemplate: img.Id,
	}

	if size := jc.RootDisk; bootstrap && size == nil {
		sc.driveSize = defaultDriveGB * gosigma.Gigabyte
	} else if size != nil {
		sc.driveSize = *size * gosigma.Megabyte
	}

	if c := jc.CpuCores; c != nil {
		sc.cores = *c
	} else {
		sc.cores = 1
	}

	if p := jc.CpuPower; p != nil {
		sc.power = *p
	} else {
		if sc.cores == 1 {
			// The default of cpu power is 2000 Mhz
			sc.power = defaultCPUPower
		} else {
			// The maximum amount of cpu per smp is 2300
			sc.power = sc.cores * defaultCPUPower
		}
	}

	if m := jc.Mem; m != nil {
		sc.mem = *m * gosigma.Megabyte
	} else {
		sc.mem = defaultMemoryGB * gosigma.Gigabyte
	}

	return &sc
}

func (c *sigmaConstraints) String() string {
	s := fmt.Sprintf("template=%s,drive=%dG", c.driveTemplate, c.driveSize/gosigma.Gigabyte)
	if c.cores > 0 {
		s += fmt.Sprintf(",cores=%d", c.cores)
	}
	if c.power > 0 {
		s += fmt.Sprintf(",power=%d", c.power)
	}
	if c.mem > 0 {
		s += fmt.Sprintf(",mem=%dG", c.mem/gosigma.Gigabyte)
	}
	return s
}
