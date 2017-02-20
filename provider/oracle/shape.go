// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"github.com/hoenirvili/go-oracle-cloud/response"
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
)

type shape struct {
	// name of the shape
	name string
	// cpu field shows the number of CPU threads
	cpus uint64
	// ram field shows the amount of memory in MB
	ram uint64
}

// find shape searches through the oracle api and finds the best shape
// that is complaint with the bootstrap constraints containing the ram and cpu
func findShape(shapes []response.Shape, cons constraints.Value) (*shape, error) {
	if shapes == nil {
		return nil, errors.NotValidf("cannot search through nil shape")
	}

	if !cons.HasMem() || !cons.HasCpuCores() {
		return nil, errors.NotValidf("cannot use empty constraints")
	}

	var (
		n, min, cpus, bestMin uint64
		bestFit               *shape
	)
	// take te best decision on the shape based on the cpu core number
	// if we found two shapes with the same cpu
	// we'll take the most the most closest ram value
	// based on the constraints mem value
	for key, val := range shapes {
		// because the oracle api has the cpu core number being a float
		// we cast it here avoiding extra casting between the code
		cpus = uint64(val.Cpus)

		// compute the minimum distance between two cpus<Paste>
		if *cons.CpuCores < cpus {
			min = cpus - *cons.CpuCores
		} else {
			min = *cons.CpuCores - cpus
		}

		// if we just started, this means we don't have any
		// computed distance checked and
		// we should add this one instead
		if key == 0 {
			n = min
			bestFit = &shape{
				cpus: cpus,
				ram:  val.Ram,
				name: val.Name,
			}
		} else if min < n {
			bestFit.cpus = cpus
			bestFit.ram = val.Ram
			bestFit.name = val.Name
			n = min
			// if we have the same number of cores
			// we must check the memory/ram count
		} else if min == n {
			// compute the difference between
			// the memory from each previous
			// value and current one
			if *cons.Mem < val.Ram {
				min = val.Ram - *cons.Mem
			} else {
				min = *cons.Mem - val.Ram
			}
			if *cons.Mem < bestFit.ram {
				bestMin = bestFit.ram - *cons.Mem
			} else {
				bestMin = *cons.Mem - bestFit.ram
			}

			// if the previous memory value is greater
			// than the current one this means the current
			// one is more appropriate for selection
			if bestMin > min {
				bestFit.cpus = cpus
				bestFit.ram = val.Ram
				bestFit.name = val.Name
			}
		}
	}

	if bestFit == nil {
		return bestFit, errors.NotFoundf(
			"cannot find any shape that is compliant with the constraint provider",
		)
	}

	return bestFit, nil
}
