// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"

	"github.com/Altoros/gosigma"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
)

// This file contains implementation of CloudSigma instance constraints
type sigmaConstraints struct {
	driveTemplate string
	driveSize     uint64
	cores         uint64
	power         uint64
	mem           uint64
}

const defaultCPUPower = 2000
// newConstraints creates new CloudSigma constraints from juju common constraints
func newConstraints(bootstrap bool, jc constraints.Value, img *imagemetadata.ImageMetadata) (*sigmaConstraints, error) {
	var sc sigmaConstraints

	sc.driveTemplate = img.Id

	if size := jc.RootDisk; bootstrap && size == nil {
		sc.driveSize = 5 * gosigma.Gigabyte
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
		sc.mem = 2 * gosigma.Gigabyte
	}

	return &sc, nil
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
