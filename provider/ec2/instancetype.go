// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"gopkg.in/amz.v3/aws"

	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/juju/arch"
)

// Type of virtualisation used.
var (
	paravirtual = "pv"
	hvm         = "hvm"
)

// all instance types can run amd64 images, and some can also run i386 ones.
var (
	amd64 = []string{arch.AMD64}
	both  = []string{arch.AMD64, arch.I386}
)

// allRegions is defined here to allow tests to override the content.
var allRegions = aws.Regions

// allInstanceTypes holds the relevant attributes of every known
// instance type.
// Note that while the EC2 root disk default is 8G, constraints on disk
// for amazon will simply cause the root disk to grow to match the constraint
var allInstanceTypes = []instances.InstanceType{
	{ // General purpose, 1st generation.
		Name:     "m1.small",
		Arches:   both,
		CpuCores: 1,
		CpuPower: instances.CpuPower(100),
		Mem:      1740,
		VirtType: &paravirtual,
	}, {
		Name:     "m1.medium",
		Arches:   both,
		CpuCores: 1,
		CpuPower: instances.CpuPower(200),
		Mem:      3840,
		VirtType: &paravirtual,
	}, {
		Name:     "m1.large",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(400),
		Mem:      7680,
		VirtType: &paravirtual,
	}, {
		Name:     "m1.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(800),
		Mem:      15360,
		VirtType: &paravirtual,
	},

	{ // General purpose, 2nd generation.
		Name:     "m3.medium",
		Arches:   amd64,
		CpuCores: 1,
		CpuPower: instances.CpuPower(300),
		Mem:      3840,
		VirtType: &paravirtual,
	}, {
		Name:     "m3.large",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(650),
		Mem:      7680,
		VirtType: &paravirtual,
	}, {
		Name:     "m3.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1300),
		Mem:      15360,
		VirtType: &paravirtual,
	}, {
		Name:     "m3.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      30720,
		VirtType: &paravirtual,
	},

	{ // Compute-optimized, 1st generation.
		Name:     "c1.medium",
		Arches:   both,
		CpuCores: 2,
		CpuPower: instances.CpuPower(500),
		Mem:      1740,
		VirtType: &paravirtual,
	}, {
		Name:     "c1.xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2000),
		Mem:      7168,
		VirtType: &paravirtual,
	}, {
		Name:     "cc2.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(8800),
		Mem:      61952,
		VirtType: &hvm,
	},

	{ // Compute-optimized, 2nd generation.
		Name:     "c3.large",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(700),
		Mem:      3840,
		VirtType: &paravirtual,
	}, {
		Name:     "c3.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1400),
		Mem:      7680,
		VirtType: &paravirtual,
	}, {
		Name:     "c3.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2800),
		Mem:      15360,
		VirtType: &paravirtual,
	}, {
		Name:     "c3.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(5500),
		Mem:      30720,
		VirtType: &paravirtual,
	}, {
		Name:     "c3.8xlarge",
		Arches:   amd64,
		CpuCores: 32,
		CpuPower: instances.CpuPower(10800),
		Mem:      61440,
		VirtType: &paravirtual,
	},

	{ // GPU instances, 1st generation.
		Name:     "cg1.4xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(3350),
		Mem:      22528,
		VirtType: &hvm,
	},

	{ // GPU instances, 2nd generation.
		Name:     "g2.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      15360,
		VirtType: &hvm,
	},

	{ // Memory-optimized, 1st generation.
		Name:     "m2.xlarge",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(650),
		Mem:      17408,
		VirtType: &paravirtual,
	}, {
		Name:     "m2.2xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1300),
		Mem:      34816,
		VirtType: &paravirtual,
	}, {
		Name:     "m2.4xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      69632,
		VirtType: &paravirtual,
	}, {
		Name:     "cr1.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(8800),
		Mem:      249856,
		VirtType: &hvm,
	},

	{ // Memory-optimized, 2nd generation.
		Name:     "r3.large",
		Arches:   amd64,
		CpuCores: 2,
		CpuPower: instances.CpuPower(650),
		Mem:      15616,
		VirtType: &hvm,
	}, {
		Name:     "r3.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1300),
		Mem:      31232,
		VirtType: &hvm,
	}, {
		Name:     "r3.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2600),
		Mem:      62464,
		VirtType: &hvm,
	}, {
		Name:     "r3.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(5200),
		Mem:      124928,
		VirtType: &hvm,
	}, {
		Name:     "r3.8xlarge",
		Arches:   amd64,
		CpuCores: 32,
		CpuPower: instances.CpuPower(10400),
		Mem:      249856,
		VirtType: &hvm,
	},

	{ // Storage-optimized, 1st generation.
		Name:     "hi1.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(3500),
		Mem:      61952,
		VirtType: &paravirtual,
	},

	{ // Storage-optimized, 2nd generation.
		Name:     "i2.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		CpuPower: instances.CpuPower(1400),
		Mem:      31232,
		VirtType: &hvm,
	}, {
		Name:     "i2.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		CpuPower: instances.CpuPower(2700),
		Mem:      62464,
		VirtType: &hvm,
	}, {
		Name:     "i2.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(5300),
		Mem:      124928,
		VirtType: &hvm,
	}, {
		Name:     "i2.8xlarge",
		Arches:   amd64,
		CpuCores: 32,
		CpuPower: instances.CpuPower(10400),
		Mem:      249856,
		VirtType: &hvm,
	}, {
		Name:     "hs1.8xlarge",
		Arches:   amd64,
		CpuCores: 16,
		CpuPower: instances.CpuPower(3500),
		Mem:      119808,
		VirtType: &paravirtual,
	},

	{ // Tiny-weeny.
		Name:     "t1.micro",
		Arches:   both,
		CpuCores: 1,
		// Burstable baseline is 20%
		CpuPower: instances.CpuPower(20),
		Mem:      613,
		VirtType: &paravirtual,
	},

	{ // General Purpose, 3rd generation.
		Name:     "t2.micro",
		Arches:   amd64,
		CpuCores: 1,
		Mem:      1024,
		// Burstable baseline is 10% (from http://aws.amazon.com/ec2/faqs/#burst)
		CpuPower: instances.CpuPower(10),
		VirtType: &hvm,
	},
	{ // General Purpose, 3rd generation.
		Name:     "t2.small",
		Arches:   amd64,
		CpuCores: 1,
		Mem:      2048,
		// Burstable baseline is 20% (from http://aws.amazon.com/ec2/faqs/#burst)
		CpuPower: instances.CpuPower(20),
		VirtType: &hvm,
	},
	{ // General Purpose, 3rd generation.
		Name:     "t2.medium",
		Arches:   amd64,
		CpuCores: 2,
		Mem:      4096,
		// Burstable baseline is 40% (from http://aws.amazon.com/ec2/faqs/#burst)
		CpuPower: instances.CpuPower(40),
		VirtType: &hvm,
	},

	{ // Compute-optimized, 3rd generation.
		Name:     "c4.large",
		Arches:   amd64,
		CpuCores: 2,
		Mem:      3840,
		CpuPower: instances.CpuPower(800),
		VirtType: &hvm,
	}, {
		Name:     "c4.xlarge",
		Arches:   amd64,
		CpuCores: 4,
		Mem:      7680,
		CpuPower: instances.CpuPower(1600),
		VirtType: &hvm,
	}, {
		Name:     "c4.2xlarge",
		Arches:   amd64,
		CpuCores: 8,
		Mem:      15360,
		CpuPower: instances.CpuPower(3100),
		VirtType: &hvm,
	}, {
		Name:     "c4.4xlarge",
		Arches:   amd64,
		CpuCores: 16,
		Mem:      30720,
		CpuPower: instances.CpuPower(6200),
		VirtType: &hvm,
	}, {
		Name:   "c4.8xlarge",
		Arches: amd64,
		// The source of this information at http://aws.amazon.com/ec2/instance-types/
		CpuCores: 36,
		Mem:      61440,
		CpuPower: instances.CpuPower(13200),
		VirtType: &hvm,
	},
}

type instanceTypeCost map[string]uint64
type regionCosts map[string]instanceTypeCost

// allRegionCosts holds the cost in USDe-3/hour for each available instance
// type in each region.
var allRegionCosts = regionCosts{
	"ap-northeast-1": { // Tokyo.
		"m1.small":  61,
		"m1.medium": 122,
		"m1.large":  243,
		"m1.xlarge": 486,

		"m3.medium":  101,
		"m3.large":   203,
		"m3.xlarge":  405,
		"m3.2xlarge": 810,

		"c1.medium": 158,
		"c1.xlarge": 632,

		"cc2.8xlarge": 2349,

		"c3.large":   128,
		"c3.xlarge":  255,
		"c3.2xlarge": 511,
		"c3.4xlarge": 1021,
		"c3.8xlarge": 2043,

		"g2.2xlarge": 898,

		"m2.xlarge":  287,
		"m2.2xlarge": 575,
		"m2.4xlarge": 1150,

		"cr1.8xlarge": 4105,

		"r3.large":   210,
		"r3.xlarge":  420,
		"r3.2xlarge": 840,
		"r3.4xlarge": 1680,
		"r3.8xlarge": 3360,

		"hi1.4xlarge": 3276,

		"i2.xlarge":  1001,
		"i2.2xlarge": 2001,
		"i2.4xlarge": 4002,
		"i2.8xlarge": 8004,

		"hs1.8xlarge": 5400,

		"t1.micro": 26,

		"t2.micro":  20,
		"t2.small":  40,
		"t2.medium": 80,

		"c4.large":   147,
		"c4.xlarge":  294,
		"c4.2xlarge": 588,
		"c4.4xlarge": 1176,
		"c4.8xlarge": 2352,
	},
	"ap-southeast-1": { // Singapore.
		"m1.small":  58,
		"m1.medium": 117,
		"m1.large":  233,
		"m1.xlarge": 467,

		"m3.medium":  98,
		"m3.large":   196,
		"m3.xlarge":  392,
		"m3.2xlarge": 784,

		"c1.medium": 164,
		"c1.xlarge": 655,

		"c3.large":   132,
		"c3.xlarge":  265,
		"c3.2xlarge": 529,
		"c3.4xlarge": 1058,
		"c3.8xlarge": 2117,

		"m2.xlarge":  296,
		"m2.2xlarge": 592,
		"m2.4xlarge": 1183,

		"r3.large":   210,
		"r3.xlarge":  420,
		"r3.2xlarge": 840,
		"r3.4xlarge": 1680,
		"r3.8xlarge": 3360,

		"i2.xlarge":  1018,
		"i2.2xlarge": 2035,
		"i2.4xlarge": 4070,
		"i2.8xlarge": 8140,

		"hs1.8xlarge": 5570,

		"t1.micro": 20,

		"t2.micro":  20,
		"t2.small":  40,
		"t2.medium": 80,

		"c4.large":   152,
		"c4.xlarge":  304,
		"c4.2xlarge": 608,
		"c4.4xlarge": 1216,
		"c4.8xlarge": 2432,

		"g2.2xlarge": 1000,
	},
	"ap-southeast-2": { // Sydney.
		"m1.small":  58,
		"m1.medium": 117,
		"m1.large":  233,
		"m1.xlarge": 467,

		"m3.medium":  98,
		"m3.large":   196,
		"m3.xlarge":  392,
		"m3.2xlarge": 784,

		"c1.medium": 164,
		"c1.xlarge": 655,

		"c3.large":   132,
		"c3.xlarge":  265,
		"c3.2xlarge": 529,
		"c3.4xlarge": 1058,
		"c3.8xlarge": 2117,

		"m2.xlarge":  296,
		"m2.2xlarge": 592,
		"m2.4xlarge": 1183,

		"r3.large":   210,
		"r3.xlarge":  420,
		"r3.2xlarge": 840,
		"r3.4xlarge": 1680,
		"r3.8xlarge": 3360,

		"i2.xlarge":  1018,
		"i2.2xlarge": 2035,
		"i2.4xlarge": 4070,
		"i2.8xlarge": 8140,

		"hs1.8xlarge": 5570,

		"t1.micro": 20,

		"t2.micro":  20,
		"t2.small":  40,
		"t2.medium": 80,

		"c4.large":   152,
		"c4.xlarge":  304,
		"c4.2xlarge": 608,
		"c4.4xlarge": 1216,
		"c4.8xlarge": 2432,

		"g2.2xlarge": 898,
	},
	"eu-west-1": { // Ireland.
		"m1.small":  47,
		"m1.medium": 95,
		"m1.large":  190,
		"m1.xlarge": 379,

		"m3.medium":  77,
		"m3.large":   154,
		"m3.xlarge":  308,
		"m3.2xlarge": 616,

		"c1.medium": 148,
		"c1.xlarge": 592,

		"cc2.8xlarge": 2250,

		"c3.large":   120,
		"c3.xlarge":  239,
		"c3.2xlarge": 478,
		"c3.4xlarge": 956,
		"c3.8xlarge": 1912,

		"cg1.4xlarge": 2360,

		"g2.2xlarge": 702,

		"m2.xlarge":  275,
		"m2.2xlarge": 550,
		"m2.4xlarge": 1100,

		"cr1.8xlarge": 3750,

		"r3.large":   195,
		"r3.xlarge":  390,
		"r3.2xlarge": 780,
		"r3.4xlarge": 1560,
		"r3.8xlarge": 3120,

		"hi1.4xlarge": 3100,

		"i2.xlarge":  938,
		"i2.2xlarge": 1876,
		"i2.4xlarge": 3751,
		"i2.8xlarge": 7502,

		"hs1.8xlarge": 4900,

		"t1.micro": 20,

		"t2.micro":  14,
		"t2.small":  28,
		"t2.medium": 56,

		"c4.large":   132,
		"c4.xlarge":  264,
		"c4.2xlarge": 528,
		"c4.4xlarge": 1056,
		"c4.8xlarge": 2112,
	},
	"sa-east-1": { // Sao Paulo.
		"m1.small":  58,
		"m1.medium": 117,
		"m1.large":  233,
		"m1.xlarge": 467,

		"m3.medium":  95,
		"m3.large":   190,
		"m3.xlarge":  381,
		"m3.2xlarge": 761,

		"c1.medium": 179,
		"c1.xlarge": 718,

		"m2.xlarge":  323,
		"m2.2xlarge": 645,
		"m2.4xlarge": 1291,

		"t1.micro": 27,

		"t2.micro":  27,
		"t2.small":  54,
		"t2.medium": 108,

		"c3.large":   163,
		"c3.xlarge":  325,
		"c3.2xlarge": 650,
		"c3.4xlarge": 1300,
		"c3.8xlarge": 2600,
	},
	"us-east-1": { // Northern Virginia.
		"m1.small":  44,
		"m1.medium": 87,
		"m1.large":  175,
		"m1.xlarge": 350,

		"m3.medium":  70,
		"m3.large":   140,
		"m3.xlarge":  280,
		"m3.2xlarge": 560,

		"c1.medium": 130,
		"c1.xlarge": 520,

		"cc2.8xlarge": 2000,

		"c3.large":   105,
		"c3.xlarge":  210,
		"c3.2xlarge": 420,
		"c3.4xlarge": 840,
		"c3.8xlarge": 1680,

		"cg1.4xlarge": 2100,

		"g2.2xlarge": 650,

		"m2.xlarge":  245,
		"m2.2xlarge": 490,
		"m2.4xlarge": 980,

		"cr1.8xlarge": 3500,

		"r3.large":   175,
		"r3.xlarge":  350,
		"r3.2xlarge": 700,
		"r3.4xlarge": 1400,
		"r3.8xlarge": 2800,

		"hi1.4xlarge": 3100,

		"i2.xlarge":  853,
		"i2.2xlarge": 1705,
		"i2.4xlarge": 3410,
		"i2.8xlarge": 6820,

		"hs1.8xlarge": 4600,

		"t1.micro": 20,

		"t2.micro":  13,
		"t2.small":  26,
		"t2.medium": 52,

		"c4.large":   116,
		"c4.xlarge":  232,
		"c4.2xlarge": 464,
		"c4.4xlarge": 928,
		"c4.8xlarge": 1856,
	},
	"us-west-1": { // Northern California.
		"m1.small":  47,
		"m1.medium": 95,
		"m1.large":  190,
		"m1.xlarge": 379,

		"m3.medium":  77,
		"m3.large":   154,
		"m3.xlarge":  308,
		"m3.2xlarge": 616,

		"c1.medium": 148,
		"c1.xlarge": 592,

		"c3.large":   120,
		"c3.xlarge":  239,
		"c3.2xlarge": 478,
		"c3.4xlarge": 956,
		"c3.8xlarge": 1912,

		"g2.2xlarge": 702,

		"m2.xlarge":  275,
		"m2.2xlarge": 550,
		"m2.4xlarge": 1100,

		"r3.large":   195,
		"r3.xlarge":  390,
		"r3.2xlarge": 780,
		"r3.4xlarge": 1560,
		"r3.8xlarge": 3120,

		"i2.xlarge":  938,
		"i2.2xlarge": 1876,
		"i2.4xlarge": 3751,
		"i2.8xlarge": 7502,

		"t1.micro": 25,

		"t2.micro":  17,
		"t2.small":  34,
		"t2.medium": 68,

		"c4.large":   138,
		"c4.xlarge":  276,
		"c4.2xlarge": 552,
		"c4.4xlarge": 1104,
		"c4.8xlarge": 2208,
	},
	"us-west-2": { // Oregon.
		"m1.small":  44,
		"m1.medium": 87,
		"m1.large":  175,
		"m1.xlarge": 350,

		"m3.medium":  70,
		"m3.large":   140,
		"m3.xlarge":  280,
		"m3.2xlarge": 560,

		"c1.medium": 130,
		"c1.xlarge": 520,

		"cc2.8xlarge": 2000,

		"c3.large":   105,
		"c3.xlarge":  210,
		"c3.2xlarge": 420,
		"c3.4xlarge": 840,
		"c3.8xlarge": 1680,

		"g2.2xlarge": 650,

		"m2.xlarge":   245,
		"m2.2xlarge":  490,
		"m2.4xlarge":  980,
		"cr1.8xlarge": 3500,

		"r3.large":   175,
		"r3.xlarge":  350,
		"r3.2xlarge": 700,
		"r3.4xlarge": 1400,
		"r3.8xlarge": 2800,

		"hi1.4xlarge": 3100,

		"i2.xlarge":  853,
		"i2.2xlarge": 1705,
		"i2.4xlarge": 3410,
		"i2.8xlarge": 6820,

		"hs1.8xlarge": 4600,

		"t1.micro": 20,

		"t2.micro":  13,
		"t2.small":  26,
		"t2.medium": 52,

		"c4.large":   116,
		"c4.xlarge":  232,
		"c4.2xlarge": 464,
		"c4.4xlarge": 928,
		"c4.8xlarge": 1856,
	},
	"eu-central-1": { // Frankfurt.
		"t2.micro":  15,
		"t2.small":  30,
		"t2.medium": 60,

		"m3.medium":  83,
		"m3.large":   166,
		"m3.xlarge":  332,
		"m3.2xlarge": 665,

		"c3.large":   129,
		"c3.xlarge":  258,
		"c3.2xlarge": 516,
		"c3.4xlarge": 1032,
		"c3.8xlarge": 2064,

		"r3.large":   210,
		"r3.xlarge":  421,
		"r3.2xlarge": 842,
		"r3.4xlarge": 1684,
		"r3.8xlarge": 3369,

		"i2.xlarge":  1013,
		"i2.2xlarge": 2026,
		"i2.4xlarge": 4051,
		"i2.8xlarge": 8102,
	},
	"us-gov-west-1": { // Isolated region - US GovCloud.
		"t2.micro":  15,
		"t2.small":  31,
		"t2.medium": 62,

		"m3.medium":  84,
		"m3.large":   168,
		"m3.xlarge":  336,
		"m3.2xlarge": 672,

		"c3.large":   126,
		"c3.xlarge":  252,
		"c3.2xlarge": 504,
		"c3.4xlarge": 1008,
		"c3.8xlarge": 2016,

		"r3.large":   210,
		"r3.xlarge":  420,
		"r3.2xlarge": 840,
		"r3.4xlarge": 1680,
		"r3.8xlarge": 3360,

		"i2.xlarge":  1023,
		"i2.2xlarge": 2046,
		"i2.4xlarge": 4092,
		"i2.8xlarge": 8184,

		"hs1.8xlarge": 5520,
	},
	"cn-north-1": { // Isolated region - China, Beijing.
		// Instance type information is from
		// http://www.amazonaws.cn/en/ec2/details/
		// TODO (anastasiamac 2015-03-05):
		// To compensate for lack of pricing, we
		// are putting in arbitrary values that reflect the relative costs.
		// This is justified by the fact that, in Juju, we use instance-by-region cost
		// to identify least expensive machine with desired constraints.
		"t1.micro": 1,

		"m1.small": 2,

		"m3.medium":  3,
		"m3.large":   5,
		"m3.xlarge":  7,
		"m3.2xlarge": 9,

		"c3.large":   4,
		"c3.xlarge":  6,
		"c3.2xlarge": 8,
		"c3.4xlarge": 10,
		"c3.8xlarge": 11,
	},
}
