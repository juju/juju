// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"fmt"
	"sort"

	"launchpad.net/juju-core/constraints"
)

// InstanceType holds all relevant attributes of the various instance types.
type InstanceType struct {
	Id       string
	Name     string
	Arches   []string
	CpuCores uint64
	Mem      uint64
	Cost     uint64
	RootDisk uint64
	// These attributes are not supported by all clouds.
	VirtType *string // The type of virtualisation used by the hypervisor, must match the image.
	CpuPower *uint64
	Tags     []string
}

func CpuPower(power uint64) *uint64 {
	return &power
}

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
	if cons.CpuPower != nil && itype.CpuPower != nil && *itype.CpuPower < *cons.CpuPower {
		return nothing, false
	}
	if cons.Mem != nil && itype.Mem < *cons.Mem {
		return nothing, false
	}
	if cons.RootDisk != nil && itype.RootDisk < *cons.RootDisk {
		return nothing, false
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 && !tagsMatch(*cons.Tags, itype.Tags) {
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

// minMemoryHeuristic is the assumed minimum amount of memory (in MB) we prefer in order to run a server (1GB)
const minMemoryHeuristic = 1024

// matchingTypesForConstraint returns instance types from allTypes which match cons.
func matchingTypesForConstraint(allTypes []InstanceType, cons constraints.Value) []InstanceType {
	var matchingTypes []InstanceType
	for _, itype := range allTypes {
		itype, ok := itype.match(cons)
		if !ok {
			continue
		}
		matchingTypes = append(matchingTypes, itype)
	}
	return matchingTypes
}

// getMatchingInstanceTypes returns all instance types matching ic.Constraints and available
// in ic.Region, sorted by increasing region-specific cost (if known).
func getMatchingInstanceTypes(ic *InstanceConstraint, allInstanceTypes []InstanceType) ([]InstanceType, error) {
	region := ic.Region
	var itypes []InstanceType

	// Rules used to select instance types:
	// - non memory constraints like cpu-cores etc are always honoured
	// - if no mem constraint specified, try opinionated default with enough mem to run a server.
	// - if no matches and no mem constraint specified, try again and return any matching instance
	//   with the largest memory
	cons := ic.Constraints
	if ic.Constraints.Mem == nil {
		minMem := uint64(minMemoryHeuristic)
		cons.Mem = &minMem
	}
	itypes = matchingTypesForConstraint(allInstanceTypes, cons)

	// No matches using opinionated default, so if no mem constraint specified,
	// look for matching instance with largest memory.
	if len(itypes) == 0 && ic.Constraints.Mem == nil {
		itypes = matchingTypesForConstraint(allInstanceTypes, ic.Constraints)
		if len(itypes) > 0 {
			sort.Sort(byMemory(itypes))
			itypes = []InstanceType{itypes[len(itypes)-1]}
		}
	}
	// If we have matching instance types, we can return those, sorted by cost.
	if len(itypes) > 0 {
		sort.Sort(byCost(itypes))
		return itypes, nil
	}

	// No luck, so report the error.
	return nil, fmt.Errorf("no instance types in %s matching constraints %q", region, ic.Constraints)
}

// tagsMatch returns if the tags in wanted all exist in have.
// Note that duplicates of tags are disregarded in both lists
func tagsMatch(wanted, have []string) bool {
	machineTags := map[string]struct{}{}
	for _, tag := range have {
		machineTags[tag] = struct{}{}
	}
	for _, tag := range wanted {
		if _, ok := machineTags[tag]; !ok {
			return false
		}
	}
	return true
}

// byCost is used to sort a slice of instance types by Cost.
type byCost []InstanceType

func (bc byCost) Len() int { return len(bc) }

func (bc byCost) Less(i, j int) bool {
	inst0, inst1 := &bc[i], &bc[j]
	if inst0.Cost != inst1.Cost {
		return inst0.Cost < inst1.Cost
	}
	if inst0.Mem != inst1.Mem {
		return inst0.Mem < inst1.Mem
	}
	if inst0.CpuPower != nil &&
		inst1.CpuPower != nil &&
		*inst0.CpuPower != *inst1.CpuPower {
		return *inst0.CpuPower < *inst1.CpuPower
	}
	if inst0.CpuCores != inst1.CpuCores {
		return inst0.CpuCores < inst1.CpuCores
	}
	if inst0.RootDisk != inst1.RootDisk {
		return inst0.RootDisk < inst1.RootDisk
	}
	// we intentionally don't compare tags, since we can't know how tags compare against each other
	return false
}

func (bc byCost) Swap(i, j int) {
	bc[i], bc[j] = bc[j], bc[i]
}

//byMemory is used to sort a slice of instance types by the amount of RAM they have.
type byMemory []InstanceType

func (s byMemory) Len() int      { return len(s) }
func (s byMemory) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byMemory) Less(i, j int) bool {
	inst0, inst1 := &s[i], &s[j]
	if inst0.Mem != inst1.Mem {
		return s[i].Mem < s[j].Mem
	}
	// Memory is equal, so use cost as a tie breaker.
	// Result is in descending order of cost so instance with lowest cost is used.
	return inst0.Cost > inst1.Cost
}
