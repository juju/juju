package environs

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
	"sort"
)

// InstanceType holds all relevant attributes of the various instance types.
type InstanceType struct {
	Id       string
	Name     string
	Arches   []string
	CpuCores uint64
	CpuPower uint64
	Mem      uint64
	// hvm instance types must be launched with hvm images.
	Hvm bool
}

// all instance types can run amd64 images, and some can also run i386 ones.
var (
	Amd64 = []string{"amd64"}
	Both  = []string{"amd64", "i386"}
)

type InstanceTypeCost map[string]uint64
type RegionCosts map[string]InstanceTypeCost

// match returns true if itype can satisfy the supplied constraints. If so,
// it also returns a copy of itype with any arches that do not match the
// constraints filtered out.
func (itype InstanceType) match(cons constraints.Value) (InstanceType, bool) {
	nothing := InstanceType{}
	if cons.Arch != nil {
		itype.Arches = filterArches(itype.Arches, []string{*cons.Arch})
	}
	if len(itype.Arches) == 0 {
		return nothing, false
	}
	if cons.CpuCores != nil && itype.CpuCores < *cons.CpuCores {
		return nothing, false
	}
	if cons.CpuPower != nil && itype.CpuPower > 0 && itype.CpuPower < *cons.CpuPower {
		return nothing, false
	}
	if cons.Mem != nil && itype.Mem < *cons.Mem {
		return nothing, false
	}
	return itype, true
}

// filterArches returns every element of src that also exists in filter.
func filterArches(src, filter []string) (dst []string) {
	for _, arch := range src {
		for _, match := range filter {
			if arch == match {
				dst = append(dst, arch)
				break
			}
		}
	}
	return dst
}

// defaultCpuPower is larger the smallest instance's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing the smallest instance from being chosen unless
// the user has clearly indicated that they are willing to accept poor performance.
var defaultCpuPower uint64 = 100

// GetInstanceTypes returns all instance types matching cons and available
// in region, sorted by increasing region-specific cost (if known).
func GetInstanceTypes(region string, cons constraints.Value,
	allinstanceTypes []InstanceType, allRegionCosts RegionCosts) ([]InstanceType, error) {
	if cons.CpuPower == nil {
		v := defaultCpuPower
		cons.CpuPower = &v
	}
	allCosts := allRegionCosts[region]
	if len(allCosts) == 0 && len(allRegionCosts) > 0 {
		return nil, fmt.Errorf("no instance types found in %s", region)
	}
	var costs []uint64
	var itypes []InstanceType
	for _, itype := range allinstanceTypes {
		cost, ok := allCosts[itype.Name]
		if !ok && len(allRegionCosts) > 0 {
			continue
		}
		itype, ok := itype.match(cons)
		if !ok {
			continue
		}
		costs = append(costs, cost)
		itypes = append(itypes, itype)
	}
	if len(itypes) == 0 {
		return nil, fmt.Errorf("no instance types in %s matching constraints %q", region, cons)
	}
	sort.Sort(byCost{itypes, costs})
	return itypes, nil
}

// byCost is used to sort a slice of instance types as a side effect of
// sorting a matching slice of costs in USDe-3/hour.
type byCost struct {
	itypes []InstanceType
	costs  []uint64
}

func (bc byCost) Len() int           { return len(bc.costs) }
func (bc byCost) Less(i, j int) bool { return bc.costs[i] < bc.costs[j] }
func (bc byCost) Swap(i, j int) {
	bc.costs[i], bc.costs[j] = bc.costs[j], bc.costs[i]
	bc.itypes[i], bc.itypes[j] = bc.itypes[j], bc.itypes[i]
}
