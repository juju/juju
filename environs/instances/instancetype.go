// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/juju/core/constraints"
)

// InstanceType holds all relevant attributes of the various instance types.
type InstanceType struct {
	Id         string
	Name       string
	Arch       string
	CpuCores   uint64
	Mem        uint64
	Networking InstanceTypeNetworking
	Cost       uint64
	RootDisk   uint64
	// These attributes are not supported by all clouds.
	VirtType *string // The type of virtualisation used by the hypervisor, must match the image.
	CpuPower *uint64
	Tags     []string
	// These two values are needed to know the maximum value of cpu and
	// memory on flexible/custom instances. Currently only supported on
	// OCI.
	MaxCpuCores *uint64
	MaxMem      *uint64
	// True value indicates it supports Secure Encrypted Virtualization.
	// False on the contrary.
	IsSev bool
}

// InstanceTypeNetworking hold relevant information about an instances
// networking capabilities.
type InstanceTypeNetworking struct {
	// SupportsIPv6 indicates if the instance supports ipv6 networking.
	SupportsIPv6 bool
}

// InstanceTypesWithCostMetadata holds an array of InstanceType and metadata
// about their cost.
type InstanceTypesWithCostMetadata struct {
	// InstanceTypes holds the array of InstanceTypes affected by this cost scheme.
	InstanceTypes []InstanceType
	// CostUnit holds the unit in which the InstanceType.Cost is expressed.
	CostUnit string
	// CostCurrency holds the currency in which InstanceType.Cost is expressed.
	CostCurrency string
	// CostDivisor indicates a number that must be applied to InstanceType.Cost to obtain
	// a number that is in CostUnit.
	// If 0 it means that InstanceType.Cost is already expressed in CostUnit.
	CostDivisor uint64
}

func CpuPower(power uint64) *uint64 {
	return &power
}

// match returns true if itype can satisfy the supplied constraints. If so,
// it also returns a copy of itype with any arches that do not match the
// constraints filtered out.
func (itype InstanceType) match(cons constraints.Value) (InstanceType, bool) {
	nothing := InstanceType{}
	if cons.HasArch() && *cons.Arch != itype.Arch {
		return nothing, false
	}
	if cons.HasInstanceType() && itype.Name != *cons.InstanceType {
		return nothing, false
	}
	if cons.CpuCores != nil && itype.CpuCores < *cons.CpuCores {
		if itype.MaxCpuCores == nil || *itype.MaxCpuCores < *cons.CpuCores {
			return nothing, false
		}
	}
	if cons.CpuPower != nil && itype.CpuPower != nil && *itype.CpuPower < *cons.CpuPower {
		return nothing, false
	}
	if cons.Mem != nil && itype.Mem < *cons.Mem {
		if itype.MaxMem == nil || *itype.MaxMem < *cons.Mem {
			return nothing, false
		}
	}
	if cons.RootDisk != nil && itype.RootDisk > 0 && itype.RootDisk < *cons.RootDisk {
		return nothing, false
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 && !tagsMatch(*cons.Tags, itype.Tags) {
		return nothing, false
	}
	if cons.HasVirtType() && (itype.VirtType == nil || *itype.VirtType != *cons.VirtType) {
		return nothing, false
	}
	return itype, true
}

const (
	// MinCpuCores is the assumed minimum CPU cores we prefer in order to run a server.
	MinCpuCores uint64 = 1
	// minMemoryHeuristic is the assumed minimum amount of memory (in MB) we prefer in order to run a server (2GB)
	minMemoryHeuristic uint64 = 2048
)

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

// MatchingInstanceTypes returns all instance types matching constraints and available
// in region, sorted by increasing region-specific cost (if known).
func MatchingInstanceTypes(allInstanceTypes []InstanceType, region string, cons constraints.Value) ([]InstanceType, error) {
	var itypes []InstanceType

	// Rules used to select instance types:
	// - non memory constraints like cores etc are always honoured
	// - if no cpu-cores constraint specified, try opinionated default
	//   with enough cpu cores to run a server.
	// - if no mem constraint specified and instance-type not specified,
	//   try opinionated default with enough mem to run a server.
	// - if no matches and no mem constraint specified, try again and
	//   return any matching instance with the largest memory
	origCons := cons
	if !cons.HasInstanceType() && !cons.HasCpuCores() {
		minCpuCores := MinCpuCores
		cons.CpuCores = &minCpuCores
	}
	if !cons.HasInstanceType() && !cons.HasMem() {
		minMem := minMemoryHeuristic
		cons.Mem = &minMem
	}
	itypes = matchingTypesForConstraint(allInstanceTypes, cons)

	// No matches using opinionated default, so if no mem constraint specified,
	// look for matching instance with largest memory.
	if len(itypes) == 0 && cons.Mem != origCons.Mem {
		itypes = matchingTypesForConstraint(allInstanceTypes, origCons)
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
	return nil, fmt.Errorf("no instance types in %s matching constraints %q", region, origCons)
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

// byMemory is used to sort a slice of instance types by the amount of RAM they have.
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

// ByName is used to sort a slice by name by best effort. As we have different separators for different providers
// A possible sort could be:
// We sort by using a lexical sort for the type, which is before the delimiter,
// and if they are the same, we sort by using the cost

type ByName []InstanceType

func (bt ByName) Len() int      { return len(bt) }
func (bt ByName) Swap(i, j int) { bt[i], bt[j] = bt[j], bt[i] }
func (bt ByName) Less(i, j int) bool {
	inst0, inst1 := &bt[i], &bt[j]
	baseInst0 := strings.FieldsFunc(inst0.Name, splitDelimiters)
	baseInst1 := strings.FieldsFunc(inst1.Name, splitDelimiters)
	if baseInst0[0] != baseInst1[0] {
		return baseInst0[0] < baseInst1[0]
	}
	// Name is equal, so use cost as a tie breaker.
	// Result is in ascending order of cost so instance with lowest cost is first.
	return inst0.Cost < inst1.Cost

}

func splitDelimiters(r rune) bool {
	return r == ',' || r == '-' || r == '.'
}
