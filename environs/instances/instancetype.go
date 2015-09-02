// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"fmt"
	"sort"

	"github.com/juju/juju/constraints"
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

type InstanceTypePredicateFn func(InstanceType) bool

func (p InstanceTypePredicateFn) Not() InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return p(i) == false
	}
}

func (p InstanceTypePredicateFn) And(preds ...InstanceTypePredicateFn) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		for _, p := range append([]InstanceTypePredicateFn{p}, preds...) {
			if p == nil {
				continue
			} else if p(i) == false {
				return false
			}
		}
		return true
	}
}

func (p InstanceTypePredicateFn) Or(preds ...InstanceTypePredicateFn) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		for _, p := range append([]InstanceTypePredicateFn{p}, preds...) {
			if p == nil {
				continue
			} else if p(i) {
				return true
			}
		}
		return false
	}
}

func FilterInstanceTypes(possible []InstanceType, matches InstanceTypePredicateFn) []InstanceType {

	good := make([]InstanceType, 0, len(possible))
	for _, instanceType := range possible {
		if matches(instanceType) == false {
			continue
		}
		good = append(good, instanceType)
	}

	return good
}

func HasInstanceType(instanceType *string) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return instanceType == nil || *instanceType == "" || i.Name == *instanceType
	}
}

func HasAtLeastCPUCores(numRequired *uint64) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return numRequired == nil || i.CpuCores >= *numRequired
	}
}

func HasAtLeastCpuPower(cpuPower *uint64) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return cpuPower == nil || i.CpuPower == nil || *cpuPower <= *i.CpuPower
	}
}

func HasAtLeastMemOfSize(size *uint64) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return size == nil || i.Mem >= *size
	}
}

func HasArch(arch *string) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		if arch == nil {
			return true
		}
		for _, supportedArch := range i.Arches {
			if supportedArch == *arch {
				return true
			}
		}
		return false
	}
}

func HasAtLeastRootDiskOfSize(size *uint64) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return size == nil || i.RootDisk >= *size || i.RootDisk == 0 // TODO(katco): This seems wrong; if it's not set, is it 0?
	}
}

func HasTag(tag string) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		for _, t := range i.Tags {
			if tag == t {
				return true
			}
		}
		return false
	}
}

func TagsMatchConstraint(cons constraints.Value) InstanceTypePredicateFn {
	if cons.Tags == nil {
		return nil
	}
	var tagsMatch InstanceTypePredicateFn
	for _, tag := range *cons.Tags {
		if tag[0] == '^' {
			tagsMatch = InstanceTypePredicateFn.And(
				InstanceTypePredicateFn.Not(HasTag(tag[1:])),
			)
		} else {
			tagsMatch = InstanceTypePredicateFn.And(HasTag(tag))
		}
	}

	return tagsMatch
}

func MatchesConstraint(cons constraints.Value) InstanceTypePredicateFn {
	fmt.Fprintf(&DebugBuffer, "Constraint: %+v\n", cons)
	return InstanceTypePredicateFn.And(
		HasInstanceType(cons.InstanceType),
		HasArch(cons.Arch),
		HasAtLeastCPUCores(cons.CpuCores),
		HasAtLeastCpuPower(cons.CpuPower),
		HasAtLeastMemOfSize(cons.Mem),
		HasAtLeastRootDiskOfSize(cons.RootDisk),
		TagsMatchConstraint(cons),
	)
}

func HasGreatestMem(greatestMem *uint64) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		if i.Mem < *greatestMem {
			return false
		}

		*greatestMem = i.Mem
		return true
	}
}

func CountMatches(p InstanceTypePredicateFn, numMatches *int) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		if p(i) == false {
			return false
		}
		*numMatches++
		return true
	}
}

func LessThan(numMatches *int, desired int) InstanceTypePredicateFn {
	return func(i InstanceType) bool {
		return *numMatches < desired
	}
}

func MatchesConstraintsOrMinMem(constraints constraints.Value, minMem uint64) InstanceTypePredicateFn {
	matches := MatchesConstraint(constraints)

	fmt.Fprintf(&DebugBuffer, "hasInstanceType: %+v\n", constraints.HasInstanceType())
	if constraints.Mem == nil {
		fmt.Fprintf(&DebugBuffer, "mem: (nil)\n")
	} else {
		fmt.Fprintf(&DebugBuffer, "mem: %v\n", *constraints.Mem)
	}

	if constraints.HasInstanceType() == false && (constraints.Mem == nil || *constraints.Mem == 0) {

		mem := minMem
		minMemConstraint := constraints
		minMemConstraint.Mem = &mem
		fmt.Fprintf(&DebugBuffer, "minMemConstraint: %+v\n", minMemConstraint)

		greatestMem := uint64(0)
		numPerfectMatches := 0
		matches = InstanceTypePredicateFn.Or(
			CountMatches(MatchesConstraint(minMemConstraint), &numPerfectMatches),
			InstanceTypePredicateFn.And(
				LessThan(&numPerfectMatches, 1),
				HasGreatestMem(&greatestMem),
				matches,
			),
		)
	}

	return matches
}

// match returns true if itype can satisfy the supplied constraints. If so,
// it also returns a copy of itype with any arches that do not match the
// constraints filtered out.
func (itype InstanceType) match(cons constraints.Value) (InstanceType, bool) {
	nothing := InstanceType{}
	if cons.Arch != nil {
		itype.Arches = filterArches(itype.Arches, []string{*cons.Arch})
	}
	if cons.HasInstanceType() && itype.Name != *cons.InstanceType {
		return nothing, false
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
	if cons.RootDisk != nil && itype.RootDisk > 0 && itype.RootDisk < *cons.RootDisk {
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

// MatchingInstanceTypes returns all instance types matching constraints and available
// in region, sorted by increasing region-specific cost (if known).
func MatchingInstanceTypes(allInstanceTypes []InstanceType, region string, cons constraints.Value) ([]InstanceType, error) {
	var itypes []InstanceType

	// Rules used to select instance types:
	// - non memory constraints like cpu-cores etc are always honoured
	// - if no mem constraint specified and instance-type not specified,
	//   try opinionated default with enough mem to run a server.
	// - if no matches and no mem constraint specified, try again and
	//   return any matching instance with the largest memory
	origCons := cons
	if !cons.HasInstanceType() && cons.Mem == nil {
		minMem := uint64(minMemoryHeuristic)
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
