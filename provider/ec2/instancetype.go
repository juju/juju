// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"launchpad.net/goamz/aws"

	"launchpad.net/juju-core/environs/instances"
)

// Type of virtualisation used.
var (
	paravirtual = "pv"
	hvm         = "hvm"
)

// all instance types can run amd64 images, and some can also run i386 ones.
var (
	amd64 = []string{"amd64"}
	both  = []string{"amd64", "i386"}
)

// allRegions is defined here to allow tests to override the content.
var allRegions = aws.Regions

// allInstanceTypes holds the relevant attributes of every known
// instance type.
// Note that while the EC2 root disk default is 8G, constraints on disk
// for amazon will simply cause the root disk to grow to match the constraint
var allInstanceTypes = []instances.InstanceType{
	{ // First generation.
		Name:     "m1.small",
		Arches:   both,
		CpuCores: 1,
		CpuPower: instances.CpuPower(100),
		Mem:      1740,
		VType:    &paravirtual,
	}, {
		Name:     "m1.medium",
		Arches:   both,
		CpuCores: 1,
		CpuPower: instances.CpuPower(200),
		Mem:      3840,
		VType:    &paravirtual,
	}, {
		Name:     "m1.large",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(400),
		Mem:      7680,
		VType:    &paravirtual,
	}, {
		Name:     "m1.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(800),
		Mem:      15360,
		VType:    &paravirtual,
	},
	{ // Second generation.
		Name:     "m3.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1300),
		Mem:      15360,
		VType:    &paravirtual,
	}, {
		Name:     "m3.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      30720,
		VType:    &paravirtual,
	},
	{ // Micro.
		Name:     "t1.micro",
		Arches:   both,
		CpuCores: 1,
		CpuPower: instances.CpuPower(20),
		Mem:      613,
		VType:    &paravirtual,
	},
	{ // High-Memory.
		Name:     "m2.xlarge",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(650),
		Mem:      17408,
		VType:    &paravirtual,
	}, {
		Name:     "m2.2xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1300),
		Mem:      34816,
		VType:    &paravirtual,
	}, {
		Name:     "m2.4xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      69632,
		VType:    &paravirtual,
	},
	{ // High-CPU.
		Name:     "c1.medium",
		Arches:   both,
		CpuCores: 2,
		CpuPower: instances.CpuPower(500),
		Mem:      1740,
		VType:    &paravirtual,
	}, {
		Name:     "c1.xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2000),
		Mem:      7168,
		VType:    &paravirtual,
	},
	{ // Cluster compute.
		Name:     "cc1.4xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(3350),
		Mem:      23552,
		VType:    &hvm,
	}, {
		Name:     "cc2.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(8800),
		Mem:      61952,
		VType:    &hvm,
	},
	{ // High Memory cluster.
		Name:     "cr1.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(8800),
		Mem:      249856,
		VType:    &hvm,
	},
	{ // Cluster GPU.
		Name:     "cg1.4xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(3350),
		Mem:      22528,
		VType:    &hvm,
	},
	{ // High I/O.
		Name:     "hi1.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(3500),
		Mem:      61952,
		VType:    &paravirtual,
	},
	{ // High storage.
		Name:     "hs1.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(3500),
		Mem:      119808,
		VType:    &paravirtual,
	},
}

type instanceTypeCost map[string]uint64
type regionCosts map[string]instanceTypeCost

// allRegionCosts holds the cost in USDe-3/hour for each available instance
// type in each region.
var allRegionCosts = regionCosts{
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
		"t1.micro":   20,
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
		"t1.micro":   20,
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
		"t1.micro":    20,
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
		"t1.micro":   27,
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
		"t1.micro":    20,
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
