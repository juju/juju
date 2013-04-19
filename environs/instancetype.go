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
	// Clustered instance types must be launched with clustered images.
	Clustered bool
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
// This only comes into effect if the cloud instance supports reporting CPU power
// for it's instance types.
var defaultCpuPower uint64 = 100

// getMatchingInstanceTypes returns all instance types matching ic.Constraints and available
// in ic.Region, sorted by increasing region-specific cost (if known).
// If no costs are specified, then we use the RAM amount as the cost on the
// assumption that it costs less to run an instance with a smaller RAM requirement.
func getMatchingInstanceTypes(ic *InstanceConstraint, allinstanceTypes []InstanceType, allRegionCosts RegionCosts) ([]InstanceType, error) {
	cons := ic.Constraints
	region := ic.Region
	defaultInstanceTypeName := ic.DefaultInstanceType
	if cons.CpuPower == nil {
		v := defaultCpuPower
		cons.CpuPower = &v
	}
	regionCosts := allRegionCosts[region]
	if len(regionCosts) == 0 && len(allRegionCosts) > 0 {
		return nil, fmt.Errorf("no instance types found in %s", region)
	}
	var costs []uint64
	var itypes []InstanceType
	var defaultInstanceType *InstanceType

	// Iterate over allInstanceTypes, finding matching ones and assigning a cost to each.
	for _, itype := range allinstanceTypes {
		if itype.Name == defaultInstanceTypeName {
			itcopy := itype
			defaultInstanceType = &itcopy
		}
		cost, ok := regionCosts[itype.Name]
		// If there are no explicit costs available, just use the instance type memory.
		if !ok {
			if len(allRegionCosts) > 0 {
				continue
			} else {
				cost = itype.Mem
			}
		}
		itype, ok := itype.match(cons)
		if !ok {
			continue
		}
		costs = append(costs, cost)
		itypes = append(itypes, itype)
	}
	// If we have matching instance types, we can return those.
	if len(itypes) > 0 {
		sort.Sort(byCost{itypes, costs})
		return itypes, nil
	}

	// No matches, so return the default if specified.
	if defaultInstanceType != nil {
		return []InstanceType{*defaultInstanceType}, nil
	}

	// No luck, so report the error.
	suffix := "and no default specified"
	if defaultInstanceTypeName != "" {
		suffix = fmt.Sprintf("and default %s is invalid", defaultInstanceTypeName)
	}
	return nil, fmt.Errorf("no instance types in %s matching constraints %q, %s", region, cons, suffix)
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
