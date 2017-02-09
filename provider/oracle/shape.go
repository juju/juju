package oracle

import (
	"fmt"

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

// findShape searches through the oracle api and finds the best shape that is complaint with
// the bootstrap constraints based on ram and cpu
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

	// take the best decision on the shape based on the cpu core number
	// and if the we found two cpu elements with the same cpu core number
	// we will take the more closest one on the ram/mem value
	for key, val := range shapes {
		fmt.Println(val)
		// because of the oracle api having the cpu core number being a float
		// we should cast it here avoiding extra casting between the code
		cpus = uint64(val.Cpus)

		// compute the minimum distrance between two cpus
		if *cons.CpuCores < cpus {
			min = cpus - *cons.CpuCores
		} else {
			min = *cons.CpuCores - cpus
		}

		// if we started, this means we don't have
		// any distrance already checked
		// and add the current one as if it's the best one
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
			// we must check the memory count
		} else if min == n {
			// compute the difference between the memory from each point
			// and previously point
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

			// if the previously point is greater than
			// the current memory point this means the current one is
			// more appropiate for the selection
			if bestMin > min {
				bestFit.cpus = cpus
				bestFit.ram = val.Ram
				bestFit.name = val.Name
			}
		}
	}

	if bestFit == nil {
		return bestFit, errors.NotFoundf(
			"cannot find any shape that is complaint with the constraint provider",
		)
	}

	return bestFit, nil
}
