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
	VType    *string // The type of virtualisation used by the hypervisor, must match the image.
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

// getMatchingInstanceTypes returns all instance types matching ic.Constraints and available
// in ic.Region, sorted by increasing region-specific cost (if known).
func getMatchingInstanceTypes(ic *InstanceConstraint, allInstanceTypes []InstanceType) ([]InstanceType, error) {
	cons := ic.Constraints
	region := ic.Region
	var itypes []InstanceType

	// Iterate over allInstanceTypes, finding matching ones.
	for _, itype := range allInstanceTypes {
		itype, ok := itype.match(cons)
		if !ok {
			continue
		}
		itypes = append(itypes, itype)
	}

	if len(itypes) == 0 {
		// No matching instance types were found, so the fallback is to:
		// 1. Sort by memory and find the smallest matching both the required architecture
		//    and our own heuristic: minimum amount of memory required to run a realistic server, or
		// 2. Sort by memory in reverse order and return the largest one, which will hopefully work,
		//    albeit not the best match
		archCons := constraints.Value{Arch: ic.Constraints.Arch}
		for _, itype := range allInstanceTypes {
			itype, ok := itype.match(archCons)
			if !ok {
				continue
			}
			itypes = append(itypes, itype)
		}
		sort.Sort(byMemory(itypes))
		var fallbackType *InstanceType
		// 1. check for smallest instance type that can realistically run a server
		for _, itype := range itypes {
			if itype.Mem >= minMemoryHeuristic {
				itcopy := itype
				fallbackType = &itcopy
				break
			}
		}
		if fallbackType == nil && len(itypes) > 0 {
			// 2. just get the one with the largest memory
			fallbackType = &itypes[len(itypes)-1]
		}
		if fallbackType != nil {
			itypes = []InstanceType{*fallbackType}
		}
	}
	// If we have matching instance types, we can return those, sorted by cost.
	if len(itypes) > 0 {
		sort.Sort(byCost(itypes))
		return itypes, nil
	}

	// No luck, so report the error.
	return nil, fmt.Errorf("no instance types in %s matching constraints %q", region, cons)
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
	return s[i].Mem < s[j].Mem
}
