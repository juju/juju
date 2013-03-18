package ec2

import (
	"fmt"
	"launchpad.net/juju-core/state"
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
	hvm      bool
}

// match returns true if itype can satisfy the supplied constraints. If so,
// it also returns a copy of itype with any arches that do not match the
// constraints filtered out.
func (itype instanceType) match(cons state.Constraints) (instanceType, bool) {
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
outer:
	for _, arch := range src {
		for _, match := range filter {
			if arch == match {
				dst = append(dst, arch)
				continue outer
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
func getInstanceTypes(region string, cons state.Constraints) ([]instanceType, error) {
	if cons.CpuPower == nil {
		cons.CpuPower = &defaultCpuPower
	}
	allCosts := allRegionCosts[region]
	if len(allCosts) == 0 {
		return nil, fmt.Errorf("no instance types found in %s", region)
	}
	var costs []float64
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
// sorting a matching slice of costs in USD/hour.
type byCost struct {
	itypes []instanceType
	costs  []float64
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

// allRegionCosts holds the cost in USD/hour for each available instance
// type in each region.
var allRegionCosts = map[string]map[string]float64{
	"ap-northeast-1": { // Tokyo.
		"m1.small":   0.088,
		"m1.medium":  0.175,
		"m1.large":   0.350,
		"m1.xlarge":  0.700,
		"m3.xlarge":  0.760,
		"m3.2xlarge": 1.520,
		"t1.micro":   0.027,
		"m2.xlarge":  0.505,
		"m2.2xlarge": 1.010,
		"m2.4xlarge": 2.020,
		"c1.medium":  0.185,
		"c1.xlarge":  0.740,
	},
	"ap-southeast-1": { // Singapore.
		"m1.small":   0.080,
		"m1.medium":  0.160,
		"m1.large":   0.320,
		"m1.xlarge":  0.640,
		"m3.xlarge":  0.700,
		"m3.2xlarge": 1.400,
		"t1.micro":   0.020,
		"m2.xlarge":  0.495,
		"m2.2xlarge": 0.990,
		"m2.4xlarge": 1.980,
		"c1.medium":  0.183,
		"c1.xlarge":  0.730,
	},
	"ap-southeast-2": { // Sydney.
		"m1.small":   0.080,
		"m1.medium":  0.160,
		"m1.large":   0.320,
		"m1.xlarge":  0.640,
		"m3.xlarge":  0.700,
		"m3.2xlarge": 1.400,
		"t1.micro":   0.020,
		"m2.xlarge":  0.495,
		"m2.2xlarge": 0.990,
		"m2.4xlarge": 1.980,
		"c1.medium":  0.183,
		"c1.xlarge":  0.730,
	},
	"eu-west-1": { // Ireland.
		"m1.small":    0.065,
		"m1.medium":   0.130,
		"m1.large":    0.260,
		"m1.xlarge":   0.520,
		"m3.xlarge":   0.550,
		"m3.2xlarge":  1.100,
		"t1.micro":    0.020,
		"m2.xlarge":   0.460,
		"m2.2xlarge":  0.920,
		"m2.4xlarge":  1.840,
		"c1.medium":   0.165,
		"c1.xlarge":   0.660,
		"cc2.8xlarge": 2.700,
		"cg1.4xlarge": 2.360,
		"hi1.4xlarge": 3.410,
	},
	"sa-east-1": { // Sao Paulo.
		"m1.small":   0.080,
		"m1.medium":  0.160,
		"m1.large":   0.320,
		"m1.xlarge":  0.640,
		"t1.micro":   0.027,
		"m2.xlarge":  0.540,
		"m2.2xlarge": 1.080,
		"m2.4xlarge": 2.160,
		"c1.medium":  0.200,
		"c1.xlarge":  0.800,
	},
	"us-east-1": { // Northern Virginia.
		"m1.small":    0.060,
		"m1.medium":   0.120,
		"m1.large":    0.240,
		"m1.xlarge":   0.480,
		"m3.xlarge":   0.500,
		"m3.2xlarge":  1.000,
		"t1.micro":    0.020,
		"m2.xlarge":   0.410,
		"m2.2xlarge":  0.820,
		"m2.4xlarge":  1.640,
		"c1.medium":   0.145,
		"c1.xlarge":   0.580,
		"cc1.4xlarge": 1.300,
		"cc2.8xlarge": 2.400,
		"cr1.8xlarge": 3.500,
		"cg1.4xlarge": 2.100,
		"hi1.4xlarge": 3.100,
		"hs1.8xlarge": 4.600,
	},
	"us-west-1": { // Northern California.
		"m1.small":   0.065,
		"m1.medium":  0.130,
		"m1.large":   0.260,
		"m1.xlarge":  0.520,
		"m3.xlarge":  0.550,
		"m3.2xlarge": 1.100,
		"t1.micro":   0.025,
		"m2.xlarge":  0.460,
		"m2.2xlarge": 0.920,
		"m2.4xlarge": 1.840,
		"c1.medium":  0.165,
		"c1.xlarge":  0.660,
	},
	"us-west-2": { // Oregon.
		"m1.small":    0.060,
		"m1.medium":   0.120,
		"m1.large":    0.240,
		"m1.xlarge":   0.480,
		"m3.xlarge":   0.500,
		"m3.2xlarge":  1.000,
		"t1.micro":    0.020,
		"m2.xlarge":   0.410,
		"m2.2xlarge":  0.820,
		"m2.4xlarge":  1.640,
		"c1.medium":   0.145,
		"c1.xlarge":   0.580,
		"cc2.8xlarge": 2.400,
		"cr1.8xlarge": 3.500,
		"hi1.4xlarge": 3.100,
	},
}
