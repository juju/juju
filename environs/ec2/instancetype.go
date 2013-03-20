package ec2

import (
	"fmt"
	"launchpad.net/juju-core/constraints"
	"sort"
)

// instanceType holds all relevant attributes of the various ec2 instance
// types.
type instanceType struct {
	name     string
	arches   []string
	cpuCores uint64
	cpuPower uint64
	mem      uint64
	// hvm instance types must be launched with hvm images.
	hvm bool
}

// match returns true if itype can satisfy the supplied constraints. If so,
// it also returns a copy of itype with any arches that do not match the
// constraints filtered out.
func (itype instanceType) match(cons constraints.Value) (instanceType, bool) {
	nothing := instanceType{}
	if cons.Arch != nil {
		itype.arches = filterArches(itype.arches, []string{*cons.Arch})
	}
	if len(itype.arches) == 0 {
		return nothing, false
	}
	if cons.CpuCores != nil && itype.cpuCores < *cons.CpuCores {
		return nothing, false
	}
	if cons.CpuPower != nil && itype.cpuPower < *cons.CpuPower {
		return nothing, false
	}
	if cons.Mem != nil && itype.mem < *cons.Mem {
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

// defaultCpuPower is larger that t1.micro's cpuPower, and no larger than
// any other instance type's cpuPower. It is used when no explicit CpuPower
// constraint exists, preventing t1.micro from being chosen unless the user
// has clearly indicated that they are willing to accept poor performance.
var defaultCpuPower uint64 = 100

// getInstanceTypes returns all instance types matching cons and available
// in region, sorted by increasing region-specific cost.
func getInstanceTypes(region string, cons constraints.Value) ([]instanceType, error) {
	if cons.CpuPower == nil {
		v := defaultCpuPower
		cons.CpuPower = &v
	}
	allCosts := allRegionCosts[region]
	if len(allCosts) == 0 {
		return nil, fmt.Errorf("no instance types found in %s", region)
	}
	var costs []uint64
	var itypes []instanceType
	for _, itype := range allInstanceTypes {
		cost, ok := allCosts[itype.name]
		if !ok {
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
	itypes []instanceType
	costs  []uint64
}

func (bc byCost) Len() int           { return len(bc.costs) }
func (bc byCost) Less(i, j int) bool { return bc.costs[i] < bc.costs[j] }
func (bc byCost) Swap(i, j int) {
	bc.costs[i], bc.costs[j] = bc.costs[j], bc.costs[i]
	bc.itypes[i], bc.itypes[j] = bc.itypes[j], bc.itypes[i]
}

// all instance types can run amd64 images, and some can also run i386 ones.
var (
	amd64 = []string{"amd64"}
	both  = []string{"amd64", "i386"}
)

// allInstanceTypes holds the relevant attributes of every known
// instance type.
var allInstanceTypes = []instanceType{
	{ // First generation.
		name:     "m1.small",
		arches:   both,
		cpuCores: 1,
		cpuPower: 100,
		mem:      1740,
	}, {
		name:     "m1.medium",
		arches:   both,
		cpuCores: 1,
		cpuPower: 200,
		mem:      3840,
	}, {
		name:     "m1.large",
		arches:   amd64,
		cpuCores: 2,
		cpuPower: 400,
		mem:      7680,
	}, {
		name:     "m1.xlarge",
		arches:   amd64,
		cpuCores: 4,
		cpuPower: 800,
		mem:      15360,
	},
	{ // Second generation.
		name:     "m3.xlarge",
		arches:   amd64,
		cpuCores: 4,
		cpuPower: 1300,
		mem:      15360,
	}, {
		name:     "m3.2xlarge",
		arches:   amd64,
		cpuCores: 8,
		cpuPower: 2600,
		mem:      30720,
	},
	{ // Micro.
		name:     "t1.micro",
		arches:   both,
		cpuCores: 1,
		cpuPower: 20,
		mem:      613,
	},
	{ // High-memory.
		name:     "m2.xlarge",
		arches:   amd64,
		cpuCores: 2,
		cpuPower: 650,
		mem:      17408,
	}, {
		name:     "m2.2xlarge",
		arches:   amd64,
		cpuCores: 4,
		cpuPower: 1300,
		mem:      34816,
	}, {
		name:     "m2.4xlarge",
		arches:   amd64,
		cpuCores: 8,
		cpuPower: 2600,
		mem:      69632,
	},
	{ // High-CPU.
		name:     "c1.medium",
		arches:   both,
		cpuCores: 2,
		cpuPower: 500,
		mem:      1740,
	}, {
		name:     "c1.xlarge",
		arches:   amd64,
		cpuCores: 8,
		cpuPower: 2000,
		mem:      7168,
	},
	{ // Cluster compute.
		name:     "cc1.4xlarge",
		arches:   amd64,
		cpuCores: 8,
		cpuPower: 3350,
		mem:      23552,
		hvm:      true,
	}, {
		name:     "cc2.8xlarge",
		arches:   amd64,
		cpuCores: 16,
		cpuPower: 8800,
		mem:      61952,
		hvm:      true,
	},
	{ // High memory cluster.
		name:     "cr1.8xlarge",
		arches:   amd64,
		cpuCores: 16,
		cpuPower: 8800,
		mem:      249856,
		hvm:      true,
	},
	{ // Cluster GPU.
		name:     "cg1.4xlarge",
		arches:   amd64,
		cpuCores: 8,
		cpuPower: 3350,
		mem:      22528,
		hvm:      true,
	},
	{ // High I/O.
		name:     "hi1.4xlarge",
		arches:   amd64,
		cpuCores: 16,
		cpuPower: 3500,
		mem:      61952,
	},
	{ // High storage.
		name:     "hs1.8xlarge",
		arches:   amd64,
		cpuCores: 16,
		cpuPower: 3500,
		mem:      119808,
	},
}

// allRegionCosts holds the cost in USDe-3/hour for each available instance
// type in each region.
var allRegionCosts = map[string]map[string]uint64{
	"ap-northeast-1": { // Tokyo.
		"m1.small":   88,
		"m1.medium":  175,
		"m1.large":   350,
		"m1.xlarge":  700,
		"m3.xlarge":  760,
		"m3.2xlarge": 1520,
		"t1.micro":   27,
		"m2.xlarge":  505,
		"m2.2xlarge": 1010,
		"m2.4xlarge": 2020,
		"c1.medium":  185,
		"c1.xlarge":  740,
	},
	"ap-southeast-1": { // Singapore.
		"m1.small":   80,
		"m1.medium":  160,
		"m1.large":   320,
		"m1.xlarge":  640,
		"m3.xlarge":  700,
		"m3.2xlarge": 1400,
		"t1.micro":   020,
		"m2.xlarge":  495,
		"m2.2xlarge": 990,
		"m2.4xlarge": 1980,
		"c1.medium":  183,
		"c1.xlarge":  730,
	},
	"ap-southeast-2": { // Sydney.
		"m1.small":   80,
		"m1.medium":  160,
		"m1.large":   320,
		"m1.xlarge":  640,
		"m3.xlarge":  700,
		"m3.2xlarge": 1400,
		"t1.micro":   020,
		"m2.xlarge":  495,
		"m2.2xlarge": 990,
		"m2.4xlarge": 1980,
		"c1.medium":  183,
		"c1.xlarge":  730,
	},
	"eu-west-1": { // Ireland.
		"m1.small":    65,
		"m1.medium":   130,
		"m1.large":    260,
		"m1.xlarge":   520,
		"m3.xlarge":   550,
		"m3.2xlarge":  1100,
		"t1.micro":    020,
		"m2.xlarge":   460,
		"m2.2xlarge":  920,
		"m2.4xlarge":  1840,
		"c1.medium":   165,
		"c1.xlarge":   660,
		"cc2.8xlarge": 2700,
		"cg1.4xlarge": 2360,
		"hi1.4xlarge": 3410,
	},
	"sa-east-1": { // Sao Paulo.
		"m1.small":   80,
		"m1.medium":  160,
		"m1.large":   320,
		"m1.xlarge":  640,
		"t1.micro":   027,
		"m2.xlarge":  540,
		"m2.2xlarge": 1080,
		"m2.4xlarge": 2160,
		"c1.medium":  200,
		"c1.xlarge":  800,
	},
	"us-east-1": { // Northern Virginia.
		"m1.small":    60,
		"m1.medium":   120,
		"m1.large":    240,
		"m1.xlarge":   480,
		"m3.xlarge":   500,
		"m3.2xlarge":  1000,
		"t1.micro":    20,
		"m2.xlarge":   410,
		"m2.2xlarge":  820,
		"m2.4xlarge":  1640,
		"c1.medium":   145,
		"c1.xlarge":   580,
		"cc1.4xlarge": 1300,
		"cc2.8xlarge": 2400,
		"cr1.8xlarge": 3500,
		"cg1.4xlarge": 2100,
		"hi1.4xlarge": 3100,
		"hs1.8xlarge": 4600,
	},
	"us-west-1": { // Northern California.
		"m1.small":   65,
		"m1.medium":  130,
		"m1.large":   260,
		"m1.xlarge":  520,
		"m3.xlarge":  550,
		"m3.2xlarge": 1100,
		"t1.micro":   25,
		"m2.xlarge":  460,
		"m2.2xlarge": 920,
		"m2.4xlarge": 1840,
		"c1.medium":  165,
		"c1.xlarge":  660,
	},
	"us-west-2": { // Oregon.
		"m1.small":    60,
		"m1.medium":   120,
		"m1.large":    240,
		"m1.xlarge":   480,
		"m3.xlarge":   500,
		"m3.2xlarge":  1000,
		"t1.micro":    020,
		"m2.xlarge":   410,
		"m2.2xlarge":  820,
		"m2.4xlarge":  1640,
		"c1.medium":   145,
		"c1.xlarge":   580,
		"cc2.8xlarge": 2400,
		"cr1.8xlarge": 3500,
		"hi1.4xlarge": 3100,
	},
}
