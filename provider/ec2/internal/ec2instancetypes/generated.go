// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes

import (
	"github.com/juju/utils/arch"

	"github.com/juju/juju/environs/instances"
)

var (
	paravirtual = "pv"
	hvm         = "hvm"
	amd64       = []string{arch.AMD64}
	both        = []string{arch.AMD64, arch.I386}
)

// Version: 20170901182201
// Publication date: 2017-09-01 18:22:01 +0000 UTC
//
// This pricing list is for informational purposes only. All prices are subject to the additional terms included in the pricing pages on http://aws.amazon.com. All Free Tier prices are also subject to the terms included at https://aws.amazon.com/free/

var allInstanceTypes = map[string][]instances.InstanceType{

	"ap-northeast-1": {

		// SKU: 2JSMK4YRVSAHV4RW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4128,
		},

		// SKU: 4BJPFU3PAZJ4AKMM
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       486,
			Deprecated: true,
		},

		// SKU: 4GHFAT5CNS2FEKB2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     516,
		},

		// SKU: 4REMK3MMXCZ55ZX3
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8004,
			Deprecated: true,
		},

		// SKU: 5YHRKH4DFNQ4XWHZ
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1464,
		},

		// SKU: 6JP9PA73B58NZHUN
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3376,
		},

		// SKU: 6M27QQ6HYCNA5KGA
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 6TMC6UD2UCCDAMNP
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       243,
			Deprecated: true,
		},

		// SKU: 77K4UJJUNGQ6UXXN
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     898,
		},

		// SKU: 7A24VVDQEZ54FYXU
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1688,
		},

		// SKU: 7KXQBZSKETPTG6QZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: 7MYWT7Y96UT3NJ2D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     129,
		},

		// SKU: 8H36QJ2PHPR3SJ48
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       5400,
			Deprecated: true,
		},

		// SKU: 9KMZWGZVTXKAQXNM
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     798,
		},

		// SKU: 9NSP3EV3G593P35X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     16,
		},

		// SKU: 9UBMZYZ6SXZ5JQGV
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: AKQ89V8E78T6H534
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       61,
			Deprecated: true,
		},

		// SKU: APGTJ6NBJA89PCKZ
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     183,
		},

		// SKU: AY6XZ64M22QQJCXE
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     193,
		},

		// SKU: BQQUCAM9PFTSUNQX
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1150,
			Deprecated: true,
		},

		// SKU: BURRP7APFUCZFSZK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     258,
		},

		// SKU: BYV8H4R4VJNAH42Q
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: C8MHVPEYQG6UHPS4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     640,
		},

		// SKU: CTK39QJHQN4CZ3PC
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     3592,
		},

		// SKU: DDX2JPPMM28BXD7D
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: E3J2T7B8EQDFXWDR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2043,
		},

		// SKU: E5MWNHYU3BAVZCRP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: E5ZC2EJP47JC4Y2A
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9671,
		},

		// SKU: E6F66FZ47YZNXAJ2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     64,
		},

		// SKU: ERVWZ4V3UBYH4NQH
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       26,
			Deprecated: true,
		},

		// SKU: EWRM596KUQ2YH8ER
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5120,
		},

		// SKU: EZCSGZJ8PMXA2QF2
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1000,
			Deprecated: true,
		},

		// SKU: F2RRJYX33EGMBSFR
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       122,
			Deprecated: true,
		},

		// SKU: F7XCNBBYFKX42QF3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     8,
		},

		// SKU: FBB5W5WTFXJSNGPN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     512,
		},

		// SKU: FBUWUPNC8FXRUS5W
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4002,
			Deprecated: true,
		},

		// SKU: G55JJ7CXZ5E2QE8H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     160,
		},

		// SKU: G6G6ZNFBYMW2V8BH
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       287,
			Deprecated: true,
		},

		// SKU: GJHUHQSUU37VCQ5A
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     399,
		},

		// SKU: GNEKD47PUMN4FP4J
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3160,
		},

		// SKU: GP8GQA2T96JQ4MBB
		// Instance family: Memory optimized
		// Storage: 2 x 120 SSD
		{
			Name:       "cr1.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(3200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       4105,
			Deprecated: true,
		},

		// SKU: HTNXMK8Z5YHMU737
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     255,
		},

		// SKU: J85A5X44TT267EH8
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     385,
		},

		// SKU: JTQKHD7ZTEEM4DC5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2580,
		},

		// SKU: KM8DYQWHEC32CGGX
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2001,
			Deprecated: true,
		},

		// SKU: M3G65XHCPFQHAQD5
		// Instance family: Storage optimized
		// Storage: 2 x 1024 SSD
		{
			Name:       "hi1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5376),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       3276,
			Deprecated: true,
		},

		// SKU: MJ7YVW9J2WD856AC
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19341,
		},

		// SKU: MKVJ4C4XUPQ3657J
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5856,
		},

		// SKU: MYX88QW5HYQW9KS4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2560,
		},

		// SKU: N6SGMGNN8CA3TG6Q
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1580,
		},

		// SKU: PCB5ARVZ6TNS7A96
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     770,
		},

		// SKU: PCNBVATW49APFGZQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: PSF2TQK8WMUGUPYK
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6752,
		},

		// SKU: PTSCWYT4DGMHMSYG
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       158,
			Deprecated: true,
		},

		// SKU: Q4QTSF7H37JFW9ER
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     128,
		},

		// SKU: Q5HVB8NUA7UMHV4Y
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     128,
		},

		// SKU: Q85F79PK8VHHZT6X
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: RCJ9VNKFJCUCGU3W
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     12336,
		},

		// SKU: RPSNHYM8M88X8DF5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1280,
		},

		// SKU: SA9UW2TC8EGBE7NW
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     732,
		},

		// SKU: T7CGRZ4XENPHVK6D
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1542,
		},

		// SKU: THKGMUKKFXV9CKUW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     24672,
		},

		// SKU: TPZKPCAQBPBS7CF8
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: U8JUARJS4SHG5W54
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     320,
		},

		// SKU: UDHFRPKESN82BQYQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6320,
		},

		// SKU: UJB452HW969DQZFB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: UMV7384WFS5N9T5F
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       575,
			Deprecated: true,
		},

		// SKU: URZU4GXQC7AT6RE9
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       632,
			Deprecated: true,
		},

		// SKU: VWWQ7S3GZ9J8TF77
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     844,
		},

		// SKU: XJ88E6MSR3AYHFXA
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1020,
		},

		// SKU: XU2NYYPCRTK4T7CN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1032,
		},

		// SKU: YCYU3NQCQRYQ2TU7
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: YJ2E4JTYGN2FMNQM
		// Instance family: Compute optimized
		// Storage: 4 x 840
		{
			Name:       "cc2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       2349,
			Deprecated: true,
		},

		// SKU: YR67H6NVBRN37HRZ
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     511,
		},

		// SKU: YUYNTU8AZ9MKK68V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     32,
		},

		// SKU: ZV2DS4C98AB8SS7J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     256,
		},
	},

	"ap-northeast-2": {

		// SKU: 25X5QV2TXMJAS9VK
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     11720,
		},

		// SKU: 3UWMR4BVSMJ3PTQ5
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1000,
			Deprecated: true,
		},

		// SKU: 4S6FN8VRCN82H4M4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     320,
		},

		// SKU: 5CB9VHZSJWQTZN3W
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     114,
		},

		// SKU: 5RC27Y2XYGFJVP7K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     246,
		},

		// SKU: 63EV7GRAYQT3HN8X
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1465,
		},

		// SKU: 65JJWWKAHFAMNF85
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19341,
		},

		// SKU: 6CMJDHFNPAKYJ783
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2560,
		},

		// SKU: 6NSPY3BTJRX47KWG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     984,
		},

		// SKU: 6TYE4QER4XX5TSC5
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: 6U3CMEPEAZEVZSV8
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: 7VFMGFAWZ9QPBHST
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     64,
		},

		// SKU: 852A82DVHUAQRBUS
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     844,
		},

		// SKU: 9DY7H84NVAJTABAD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3936,
		},

		// SKU: 9XQJDHCZD834J68K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     123,
		},

		// SKU: BHS4CH7UVYY7QN2H
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4002,
			Deprecated: true,
		},

		// SKU: BQDV4FCR9QJEQHQS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     907,
		},

		// SKU: BRTSXYEA84EMVTVE
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     798,
		},

		// SKU: BZTF9TN68JF5PASX
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5856,
		},

		// SKU: CEU6V4KXWNQA6DD3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     128,
		},

		// SKU: EGXGRBT8ERK49SBP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     256,
		},

		// SKU: EPF7FTX8HQURWQHY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1280,
		},

		// SKU: G6ATM6E28ZDDBNCE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     227,
		},

		// SKU: GENEHT29PB98QFPX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     512,
		},

		// SKU: GF99HNDMXXY2RBJG
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: HUNMAJP6W7UHJAAG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1815,
		},

		// SKU: JZYQ7EEFZRRYYC5S
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1688,
		},

		// SKU: K7VYRSCD5B26K6RF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     640,
		},

		// SKU: KF2B96YA25ZRC292
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9671,
		},

		// SKU: KGFSNH7UYJEDWTQQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     8,
		},

		// SKU: KUKJATN7HCNF2UFT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     492,
		},

		// SKU: KW2ZPRSC298WFJ94
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     399,
		},

		// SKU: MXYJRCMDAHMFNUAB
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: N6M9WS3F7XHZ2TXS
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6752,
		},

		// SKU: P75FYMSDEYRH34VG
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2001,
			Deprecated: true,
		},

		// SKU: PBRNAGPS98SBSDRS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     454,
		},

		// SKU: PSBR72NYUMRACH7E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     32,
		},

		// SKU: QWC4JPSHYEGW8MZR
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1464,
		},

		// SKU: RC4WEGPKU9M6BMP7
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: RTEC5MAEM2BXWZ39
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     160,
		},

		// SKU: SUA46K44385RPAM4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5120,
		},

		// SKU: VRF92SGE5N2VZZTQ
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     183,
		},

		// SKU: WFBUYA3WPRPDVNEH
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3376,
		},

		// SKU: X4D5VUURJC7BR95Z
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     23440,
		},

		// SKU: XCB734X2BM8PZ77F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2460,
		},

		// SKU: YG3C8Z588MN6BXGW
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8004,
			Deprecated: true,
		},

		// SKU: YMCDAKZ8EVGJJDRM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     16,
		},

		// SKU: Z9JAE8K6RJTMMZ7P
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     732,
		},
	},

	"ap-south-1": {

		// SKU: 2BHQP3WKDU9A2DSC
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     827,
		},

		// SKU: 2M95KZBY9QJCSVWB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: 2P3BAZEBUTS4SPUF
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     354,
		},

		// SKU: 2T64ZB6E54RM9GA2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1756,
		},

		// SKU: 37A5REAJC363YUHH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     476,
		},

		// SKU: 3P5UPPTRJJQ6TKSU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     123,
		},

		// SKU: 5N383TJKMC5FSCKD
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3032,
		},

		// SKU: 673MQ62EKV4VCTT8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     7,
		},

		// SKU: 6D7VQBNGRWYB2U7T
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: 6WAFB82CP99WZXD9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     30,
		},

		// SKU: 7DZBY6C9YNNVET76
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1216,
		},

		// SKU: 7HYM8MHNNFW2NN6T
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3867,
			Deprecated: true,
		},

		// SKU: 8BG4ECAGKKNWYDVU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     439,
		},

		// SKU: 8U4NEK2635VB7NHD
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     758,
		},

		// SKU: 9Q2KYTZY2YDQZCM8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     246,
		},

		// SKU: AFU2HU8WVY9T6QAK
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1933,
			Deprecated: true,
		},

		// SKU: BKC93YZQB5XG785E
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: BQ6P3ZSPRN3PC56P
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     13744,
		},

		// SKU: CA3Y8U6BUYR54NVM
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1516,
		},

		// SKU: D3KF5EAWRYNBCMNK
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9187,
		},

		// SKU: DDTXDG7MMJKV72FM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3936,
		},

		// SKU: FJX2SVQ2PFDKB8Z4
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1416,
		},

		// SKU: FQ7FVC9B3R8RBBXA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2460,
		},

		// SKU: G4283CPK5MQ5QQ2A
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       7733,
			Deprecated: true,
		},

		// SKU: G69QDQNHE5SR7846
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3306,
		},

		// SKU: GG7UHJRKQSGP364T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     119,
		},

		// SKU: GGTGBU32M4STN8YS
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1653,
		},

		// SKU: GNRA9A27Y4WHA4EE
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     18374,
		},

		// SKU: JXAMSKC2ZXKCA37S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     15,
		},

		// SKU: K2V83HS47FBDSX5J
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     608,
		},

		// SKU: KFTR5EQCGQ6AUYXP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     110,
		},

		// SKU: KKF4PXVB9Z3ANP6K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     238,
		},

		// SKU: MBD9GZ9JM5QGHC6U
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1718,
		},

		// SKU: MKNAAVQMBXXNTPQQ
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     190,
		},

		// SKU: NH9KFSA26V6F5742
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6612,
		},

		// SKU: Q3DWV2ZGENHRB735
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     27488,
		},

		// SKU: Q5HCF2WEXJ7TRHNF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     59,
		},

		// SKU: QV2HMETS44HPETDJ
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5664,
		},

		// SKU: RZAKYAJATKRN94UJ
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2832,
		},

		// SKU: TEV889FX73ZKZ8TU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     878,
		},

		// SKU: TPTBS44NNEJN3HUG
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       967,
			Deprecated: true,
		},

		// SKU: TPVVGJC63DQYU7EJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     220,
		},

		// SKU: WFUARWWHZWBKCUEF
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     177,
		},

		// SKU: X3V37E29WMEDZZ96
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: XMPC9229VJ8N9R4N
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     708,
		},

		// SKU: YKUFHRZDYCT9JG3A
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     984,
		},

		// SKU: ZEEU583UYCZMVJZV
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     379,
		},

		// SKU: ZVMAFPQR3NKB6VVP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     492,
		},
	},

	"ap-southeast-1": {

		// SKU: 2B4AFZB6SYHMPZGS
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       233,
			Deprecated: true,
		},

		// SKU: 2S29GABT3GMS28E4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: 39ZR86RYWKDSK82K
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       296,
			Deprecated: true,
		},

		// SKU: 3ZUGJVTA8NWE9NZT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     924,
		},

		// SKU: 4R9NGSGTSSXJGD8Z
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6680,
		},

		// SKU: 4T29U7SCA5JRZVDD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5120,
		},

		// SKU: 57M4AZ4NRYTPT6NM
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19341,
		},

		// SKU: 5BWHVPBAG2VXEG2N
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3340,
		},

		// SKU: 5ES8X7PS795W6ZD4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: 66F7RUND4W7FAD23
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1670,
		},

		// SKU: 6R4QVUNHTJVS9J2S
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       58,
			Deprecated: true,
		},

		// SKU: 7FQD2RCMJSS57GFA
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       117,
			Deprecated: true,
		},

		// SKU: 7QHAUE39SCU6N9N9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     30,
		},

		// SKU: 7TMGTEJPM5UPWQ8X
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       467,
			Deprecated: true,
		},

		// SKU: 876HZ3N29HJQQH9H
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5984,
		},

		// SKU: 8E9KB9CNE94Z4AHE
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4070,
			Deprecated: true,
		},

		// SKU: 8HKF2YYVVMBUQWDD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4000,
		},

		// SKU: 8V5MYBMPUD434579
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       655,
			Deprecated: true,
		},

		// SKU: 8VCD85YY26XCKZDE
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     392,
		},

		// SKU: 9RWVXPA45N3RYHNP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1280,
		},

		// SKU: ABMNUJ6SQ7A595A4
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     132,
		},

		// SKU: AEUJF75AZR2WPVWK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2500,
		},

		// SKU: B9DFHMNNN499Z259
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     125,
		},

		// SKU: DD25TB3PR4DVW2KT
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2992,
		},

		// SKU: DK6FJW8STXUGW6PA
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     4000,
		},

		// SKU: DKFKKEAW78H8X64T
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2035,
			Deprecated: true,
		},

		// SKU: DTZY5KW9NPT6V929
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     870,
		},

		// SKU: EERGZVYFKRBMSYKW
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       5570,
			Deprecated: true,
		},

		// SKU: EEUHF7PCXDQT2MYE
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6960,
		},

		// SKU: EV2HH2XUX6RZEAW3
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: G8R6BV5ZAJWSP2AN
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1496,
		},

		// SKU: G9Z5RTPAVX5KWH4Z
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     265,
		},

		// SKU: GCVKBN38AXXGHBQH
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1018,
			Deprecated: true,
		},

		// SKU: J23MFJ7UXYN9HFKS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     60,
		},

		// SKU: J65Z38YCBYKP7Q49
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       20,
			Deprecated: true,
		},

		// SKU: JAQKBJZV34JSH3K9
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     13744,
		},

		// SKU: JDH4WM7E92WUS9JS
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: K69TGQ7NEKQKXKHH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     240,
		},

		// SKU: KCZD349CGXR5DRQ3
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: KUSGF6VAK4SU2UDQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     640,
		},

		// SKU: M2D74WNYRB6V7EAJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     320,
		},

		// SKU: M5ZT2V2ZMSBCEB2Q
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3480,
		},

		// SKU: M6ESFJ5RPSHCG5CU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     160,
		},

		// SKU: MFGUBMTK5A9REQDM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2560,
		},

		// SKU: N55SZ6XU92JF33VX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     250,
		},

		// SKU: NYDC3MQ4HXYCHRE7
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     187,
		},

		// SKU: P6BPTANASQKJ8FJX
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     399,
		},

		// SKU: PJ8AKRU5VVMS9DFN
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     529,
		},

		// SKU: QAZ63UXRUYRSDT6R
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1718,
		},

		// SKU: QB3EG2XVBQ5BYA5F
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: QF57FDYSW9GTMVSM
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     374,
		},

		// SKU: R8K75VHRAADAJJ2W
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     784,
		},

		// SKU: RZV9MRNEARCGY297
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     15,
		},

		// SKU: SKTEJ2QN2YW8UFKF
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1740,
		},

		// SKU: SMHSRASDZ66J6CC3
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     798,
		},

		// SKU: T2PU2JF8K7NGF3AH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     7,
		},

		// SKU: TYGKARPH33A4B8DT
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       592,
			Deprecated: true,
		},

		// SKU: U2ZQYY8HDZEN4PT8
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     27488,
		},

		// SKU: U9CPUKN22CXMPGRV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     231,
		},

		// SKU: UKF69K7GTUQKKRPH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     500,
		},

		// SKU: UKGPAABCGR48DYC4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     462,
		},

		// SKU: UKY8RWKR7MVYC863
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1058,
		},

		// SKU: UYUWYNASFB3J75S6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: VE5MWWHUXS2VR8DV
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8140,
			Deprecated: true,
		},

		// SKU: VVKTWPMARM4HESXU
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: WWNCDS8M4BV5TZ4W
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     748,
		},

		// SKU: XUVJRQ9MSAQKDXE9
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2117,
		},

		// SKU: Y3RWQDFC7G8TZ3A8
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1183,
			Deprecated: true,
		},

		// SKU: YDP6BX3WNNZ488BZ
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       164,
			Deprecated: true,
		},

		// SKU: YSKUUH777M98DWE4
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     196,
		},

		// SKU: Z3DQKNTFUZ68H6TT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1848,
		},

		// SKU: ZAGTVD3ADUUPS6QV
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     98,
		},

		// SKU: ZB3PCMUQE9XQZAHW
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9671,
		},
	},

	"ap-southeast-2": {

		// SKU: 296YCXVCWAKPXKRE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     500,
		},

		// SKU: 2PKSXUFC38ZY888Q
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     130,
		},

		// SKU: 2YQUKAJYK5F5GQ85
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2992,
		},

		// SKU: 46ZVWU6WX68NZCE7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2085,
		},

		// SKU: 4PRF9CZZBT3AM9D4
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1183,
			Deprecated: true,
		},

		// SKU: 55WHWE5CGRCKDNSG
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     187,
		},

		// SKU: 5BKJJZ77BSJPMR4D
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       5570,
			Deprecated: true,
		},

		// SKU: 66QVG55FP52WHCFH
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19341,
		},

		// SKU: 69UM5U8QFXRAU255
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     372,
		},

		// SKU: 6CK52R5BRMQEVGRW
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2035,
			Deprecated: true,
		},

		// SKU: 6PB95M6GG8CNXMMR
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       592,
			Deprecated: true,
		},

		// SKU: 6UHS7YAMM8JY7X52
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4070,
			Deprecated: true,
		},

		// SKU: 6UMTMKVFBXENW3BF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     125,
		},

		// SKU: 6WEMUEK6JNZU6PTC
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     93,
		},

		// SKU: 78Z5UDBK335DDYN5
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9671,
		},

		// SKU: 7NYHPHSMD45SYSNN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     16,
		},

		// SKU: 8A5X9KQR4YKYYXCQ
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       467,
			Deprecated: true,
		},

		// SKU: 8XZUT4AHDH972AME
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     132,
		},

		// SKU: 9CYSN2TKZDN6GFWQ
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       20,
			Deprecated: true,
		},

		// SKU: AATPJ7WMWX748RZQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     24672,
		},

		// SKU: BE5KJ8JQRJNSND64
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     319,
		},

		// SKU: C4A5RM72TUGX8R5D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: CMDB58FT3PAJJNGN
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       655,
			Deprecated: true,
		},

		// SKU: CP3U32VDAT67RT9R
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1740,
		},

		// SKU: CW6MMQ5PUWH2ER9P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     256,
		},

		// SKU: D29U26UAEX6WK4TW
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     745,
		},

		// SKU: DBMRRDDSPZZKNV49
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: DF9ZTHT5UAFTDNCY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5107,
		},

		// SKU: DS7EYGXHAG6T6NTV
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     898,
		},

		// SKU: E6JQJZ8BQHCG328E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     32,
		},

		// SKU: EAVTTHNUHS2CF6PH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     159,
		},

		// SKU: F9BAR5QA2VU3ZTBF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1042,
		},

		// SKU: FCTGZX66N3RRDJ48
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1754,
		},

		// SKU: FFBDA7VFHVPEJXS6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     522,
		},

		// SKU: FJURXZQ9HT9HN2YJ
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1018,
			Deprecated: true,
		},

		// SKU: FYMCPD2A3YBTSUPQ
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2117,
		},

		// SKU: GKVR3QEC5B7WJXTD
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: HDSPKHDAUP2HXQTR
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     870,
		},

		// SKU: HHJGN8MDU3U6DFE5
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     186,
		},

		// SKU: HP48BUB3CX5F259P
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1276,
		},

		// SKU: JT2PVSWTGS2BMV4D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     128,
		},

		// SKU: KEVDJ9YEEGJZZGDS
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6960,
		},

		// SKU: KNJZFWCSBKY8N4NF
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     399,
		},

		// SKU: KWTW9RNYJG6GG3J2
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       233,
			Deprecated: true,
		},

		// SKU: KYSFQQQ4H28QEHFQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     8,
		},

		// SKU: MSGAHYMZTGGJN5WS
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3480,
		},

		// SKU: N32CG42C5KFN6GDH
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1058,
		},

		// SKU: N3D6SQF6HU9ENSPR
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: NV37A29BHV49EC6J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4000,
		},

		// SKU: P6V9GX45QEN62C49
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1542,
		},

		// SKU: P8EEBTBGBRJ8NMV2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2553,
		},

		// SKU: PZ5MY9JF8UD95F8E
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     12336,
		},

		// SKU: Q2JZP35TJBRDR3JQ
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     374,
		},

		// SKU: QZ3QJ95MESM7EP8U
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1496,
		},

		// SKU: R7QMC6E4E9FB48YN
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5984,
		},

		// SKU: R8KMJWXSQ8BJC35M
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     529,
		},

		// SKU: RAWDW374YPCAB65D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     250,
		},

		// SKU: RGHP93AGXPHPNNV7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     512,
		},

		// SKU: RW8353QQ8DWZ4WQD
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8140,
			Deprecated: true,
		},

		// SKU: RZV2TACP5YK7YCW9
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     638,
		},

		// SKU: SCGSTDEV6AX8BZK2
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     748,
		},

		// SKU: SPJKFAB8G379JD6R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2500,
		},

		// SKU: T72BQ8E4ETD9K62R
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       164,
			Deprecated: true,
		},

		// SKU: TD8NW4BSBCYU646U
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     3592,
		},

		// SKU: TPJVBXMBFDUBJM83
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       117,
			Deprecated: true,
		},

		// SKU: V5QBGHBWB72NCP2S
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3508,
		},

		// SKU: V5SUYWWSC9HUZFWJ
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       296,
			Deprecated: true,
		},

		// SKU: VM2PFN8ME9595UGP
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       58,
			Deprecated: true,
		},

		// SKU: WDP4VXVU2CSHWHQJ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7016,
		},

		// SKU: WNYWP7QUJ3MU8NVV
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     798,
		},

		// SKU: XD4VKMMZMCMZYFWJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     261,
		},

		// SKU: XPAKDV3PWHYTJU3X
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     265,
		},

		// SKU: ZNG78GP248PZPM6R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     64,
		},
	},

	"ca-central-1": {

		// SKU: 2KBX98CJX2HWB9Y6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     584,
		},

		// SKU: 44HDD89HH2F8UQJE
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: 5JBYKNTWNAS9ZFFH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     111,
		},

		// SKU: 7YD2CTAF78EVVBJS
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: 84RRJJ2JMNHAAFG7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     26,
		},

		// SKU: 9EH6ESGSKXAF3P35
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     292,
		},

		// SKU: 9NA2WFEXZ7RDCYF8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     103,
		},

		// SKU: 9VFZ3XPUYMQH2SMD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1168,
		},

		// SKU: 9ZFAT4J9YK6N24CW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3552,
		},

		// SKU: ASXQBSPHNBMAVDDX
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5504,
		},

		// SKU: B9CVS2G5SV8FNYPP
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: BW9U468HE4KTGB39
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     110,
		},

		// SKU: BYPN3A92GMZ82584
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     759,
		},

		// SKU: EFZ2NGQZ9PYWX896
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     7336,
		},

		// SKU: G4EGYKSK3QKJSHJP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     888,
		},

		// SKU: HQJBJTGFTSPC74GU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     146,
		},

		// SKU: J3CJJMBJ2AX72YT3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     444,
		},

		// SKU: J3RMSBG9SUXE4S3F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     206,
		},

		// SKU: KGBDPESEGF4BNTCQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     438,
		},

		// SKU: MBENAB2KVWCPAE53
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     14672,
		},

		// SKU: MUB9BC2VEUDHRH2X
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: NWZ9Q2HNNPQGRUEG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2336,
		},

		// SKU: P66QF9GGMTFWWDQX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     412,
		},

		// SKU: P8A5UFF56BKMYN69
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3036,
		},

		// SKU: P9XM8363KCGASTC8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1750,
		},

		// SKU: U34MCPN3J72T9YXY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     876,
		},

		// SKU: UJXSQ2MTGJKEV6TD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: WY8WPYMC5S9KSCER
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6072,
		},

		// SKU: X7FY37JNWXMQ8UMX
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1518,
		},

		// SKU: X92PHPFX577KYX8G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     52,
		},

		// SKU: XD8VQS9QGTEDMXNX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: XMFRN98P8QA898V3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2220,
		},

		// SKU: Y4MQ2FTHPYD6T6JC
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: YJ9F4B5UJ4AXRTD5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     218,
		},

		// SKU: Z5526AK4R9W2MEQZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     222,
		},

		// SKU: ZU3DGSCPMJCWQHHW
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4672,
		},
	},

	"eu-central-1": {

		// SKU: 2FXKMSNT79U4CF55
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     10608,
		},

		// SKU: 3AFDFDJ9FGMNBBUZ
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3201,
		},

		// SKU: 3KCGMZRVWDY4AC5R
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1600,
		},

		// SKU: 4TT65SC5HVYUSGR2
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4051,
			Deprecated: true,
		},

		// SKU: 5P7657GQ9EZ2Z4ZY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     54,
		},

		// SKU: 5RNA3KEVYJW8UJWT
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3176,
		},

		// SKU: 5ZZCF2WTD3M2NVHT
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     315,
		},

		// SKU: 64YPSFUXJJK7NNXR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1280,
		},

		// SKU: 686NEEYZAPY5GJ8N
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     129,
		},

		// SKU: 6PSHDB8D545JMBBD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: 6TVMF7Y6ZU47VR6J
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     372,
		},

		// SKU: 6VWRB5XDS5RZ5623
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2976,
		},

		// SKU: 6Y959B8MKQZ55MGT
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1588,
		},

		// SKU: 7EFZWDA5CSAB85BF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: 7EJH5CWEXABPY2ST
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     18674,
		},

		// SKU: 7SCPZAJBCACPU2WN
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2850,
		},

		// SKU: 7W6DNQ55YG9FCPXZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: 8KTQAHWA58GUHDGC
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     158,
		},

		// SKU: 8MASRMZD7KUHQBJC
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9337,
		},

		// SKU: 9VD98SS8PD636SQE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5121,
		},

		// SKU: ABFDCPB959KUGRH8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: AF4UC3SWD5PZXC25
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1488,
		},

		// SKU: ATHMXFEBFCM8TPWK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     909,
		},

		// SKU: AV7PMZU7MYQGTH9R
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     320,
		},

		// SKU: BG59UMPBQZ8AT89X
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: C2EDZ5DQN8NMN54X
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     772,
		},

		// SKU: CDQ3VSAVRNG39R6V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: CNP4PV4Y2J8YZVAR
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     632,
		},

		// SKU: CU49Z77S6UH36JXW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     27,
		},

		// SKU: D8BFUEFHTHMN4XUY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1817,
		},

		// SKU: EF7GKFKJ3Y5DM7E9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     240,
		},

		// SKU: ER456JE239VN5TQY
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     400,
		},

		// SKU: F5FY39C3HWRVW8M7
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6352,
		},

		// SKU: FECZ7UBC3GFUYSJC
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2025,
			Deprecated: true,
		},

		// SKU: FXJNETA7Z34Z9BAR
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     794,
		},

		// SKU: GDZZPNEEZXAN7X9J
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     79,
		},

		// SKU: HJDQXCZYW2H7N9CY
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     186,
		},

		// SKU: HW3SH7C5H3K3MV7E
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     258,
		},

		// SKU: JG83GAMRHT9DJ8TH
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: JGXEWM5NJCZJPHGG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     960,
		},

		// SKU: JTAH32JD3C26TDQZ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1425,
		},

		// SKU: KY65WK89RNASCT6R
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1326,
		},

		// SKU: KYFX85FCPCCT57BD
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2064,
		},

		// SKU: MB3JDB58W76ZHFT8
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     516,
		},

		// SKU: MTQWAHX8C4T4FYVW
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     800,
		},

		// SKU: N2333QQ45Q3K9RT9
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1012,
			Deprecated: true,
		},

		// SKU: N7WSYHVUT72KMK3V
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1032,
		},

		// SKU: NZS3Z83VUDZA9SPY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     454,
		},

		// SKU: Q5D9K2QEBW7SS9YP
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     3088,
		},

		// SKU: Q6FFSFPJYR84UFKC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: QDEU5YKUTCQUD2EA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5700,
		},

		// SKU: QJ82YTRR8GFNUS8T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3840,
		},

		// SKU: SMTUMBHX6YKRBJQB
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8102,
			Deprecated: true,
		},

		// SKU: T9GCN3NZ9U6N5BGN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     14,
		},

		// SKU: UBSAA4SE7N86SRHF
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     744,
		},

		// SKU: UJ8XNWZDKF9GB6T3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2560,
		},

		// SKU: URK82PJ9UJBYTTBJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     640,
		},

		// SKU: VEJEX5ZH8XF6E4JG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     160,
		},

		// SKU: VZ6J7FB3Q77KJBWQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     21216,
		},

		// SKU: WWTVB5GY85P5FGNW
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     114,
		},

		// SKU: XWVCP8TVZ3EZXHJT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2400,
		},

		// SKU: ZAC36C46HPYXADA7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     227,
		},
	},

	"eu-west-1": {

		// SKU: 249WVV2ASQDC3RY2
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: 2AUYT356PD9A2MBU
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2420,
		},

		// SKU: 2D5G3BCXGXH9GCH3
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16006,
		},

		// SKU: 2SX63SRBXZK94TSA
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1470,
		},

		// SKU: 38KKRTQP385PX9HY
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7776,
		},

		// SKU: 3H8WR8FBAE4DWNRB
		// Instance family: GPU instance
		// Storage: 2 x 840
		{
			Name:       "cg1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(1600),
			Mem:        23040,
			VirtType:   &hvm,
			Cost:       2360,
			Deprecated: true,
		},

		// SKU: 4G2Z3WVSPDEGMKFH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     101,
		},

		// SKU: 4JKQTWY5J6VJ6ESF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     592,
		},

		// SKU: 6FU9JEK79WWSARQ9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     222,
		},

		// SKU: 6HX9NKE3BQ5V3PMJ
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2940,
		},

		// SKU: 7X4K64YA59VZZAC3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     111,
		},

		// SKU: 8FFUWN2ESZYSB84N
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     444,
		},

		// SKU: 926EPQHVQ6AGDX5P
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     956,
		},

		// SKU: 9QYQQRQ9FD9YCPNB
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       379,
			Deprecated: true,
		},

		// SKU: 9VHN6EZGZGFZEHHK
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     735,
		},

		// SKU: ADT8TJSCKTFKTBMX
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       7502,
			Deprecated: true,
		},

		// SKU: BG8E99UBN6RZV6WV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     905,
		},

		// SKU: BKDVXZQADJ4PDJHJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     404,
		},

		// SKU: BNSCBCWPZHWPDKKS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     888,
		},

		// SKU: C3M6ZGSU66GC75NF
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     185,
		},

		// SKU: C65HY6EVYSPTGWDH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     296,
		},

		// SKU: CP6AQ5U62SXMQV9P
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     741,
		},

		// SKU: DFX4Y9GW9C3HE99V
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       20,
			Deprecated: true,
		},

		// SKU: DYTSK9JJGPSR6VQB
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1876,
			Deprecated: true,
		},

		// SKU: E3P4TVHCARM5N5RM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4742,
		},

		// SKU: E9FTXSZ49KS3R3HY
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2808,
		},

		// SKU: EB2QM2B74W2YCANP
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       47,
			Deprecated: true,
		},

		// SKU: EQP9JWYVRCW49MPW
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3751,
			Deprecated: true,
		},

		// SKU: F3VADBY3Z6MMHKTQ
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8003,
		},

		// SKU: FSS42UA3US5PWMV7
		// Instance family: Memory optimized
		// Storage: 2 x 120 SSD
		{
			Name:       "cr1.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(3200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3750,
			Deprecated: true,
		},

		// SKU: H4R7PWQCKEH9WRSS
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4840,
		},

		// SKU: H5BFJEKG7FWH5C2G
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5504,
		},

		// SKU: HB5V2X8TXQUTDZBV
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       148,
			Deprecated: true,
		},

		// SKU: HF7N6NNE7N8GDMBE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     50,
		},

		// SKU: HG3TP7M3FQZ54HKR
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1100,
			Deprecated: true,
		},

		// SKU: JGXNGK5X7WE7K3VF
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     146,
		},

		// SKU: JYFU75N5Q79WYWZE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: K24DCJ9F92C7KWQV
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2371,
		},

		// SKU: K7YHHNFGTNN2DP28
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: KKQD5EPCF8JFUDDA
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       938,
			Deprecated: true,
		},

		// SKU: KPQDNX9YMUA29HRQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     972,
		},

		// SKU: N6KDUVR23T758UUC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     25,
		},

		// SKU: NSCRWEDQZZESFDFG
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       190,
			Deprecated: true,
		},

		// SKU: NV44PJXFFQV9UNQZ
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5880,
		},

		// SKU: P3CTRQJY7SHQ6BJR
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     585,
		},

		// SKU: P75KF3MVS7BD8VRA
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     478,
		},

		// SKU: P7JTZV2EPW3T8GT2
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       550,
			Deprecated: true,
		},

		// SKU: PCKAVX9UQTRXBNNF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2220,
		},

		// SKU: PR4SS7VH54V5XAZZ
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1482,
		},

		// SKU: PY52HJB9NWEKKBZK
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2964,
		},

		// SKU: Q3BP5KJZEPCUMKM3
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       275,
			Deprecated: true,
		},

		// SKU: QA275J7BMXJR8X35
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: QRP5VBPEA34W72YQ
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     293,
		},

		// SKU: R3F9KB5UVZ9UKQ32
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1210,
		},

		// SKU: RASFAC97JWEGEPYS
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: RQP6UTWCTHK7X5XP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3552,
		},

		// SKU: SDFJSCXXJEFDV7P2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: STTHYT3WDDQU8UBR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: SZGY4A8U8CBJGHRV
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     15552,
		},

		// SKU: T3ZC3B9VPS8PA59H
		// Instance family: Compute optimized
		// Storage: 4 x 840
		{
			Name:       "cc2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       2250,
			Deprecated: true,
		},

		// SKU: TDVRYW6K68T4XJHJ
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       4900,
			Deprecated: true,
		},

		// SKU: TKHFWQ22TGGPCVCR
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: UNEZG8PVCP3RUSQG
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     73,
		},

		// SKU: V2SRX3YBPSJPD8E4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     453,
		},

		// SKU: V4Q928Z7YAM3TJ6X
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: VAUARUU95QRV96BX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     148,
		},

		// SKU: VM3SRW97DB2T2U8Z
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     702,
		},

		// SKU: VPAFYT3KA5TFAK4M
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     371,
		},

		// SKU: WDZRKB8HUJXEKH45
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     239,
		},

		// SKU: WR44KB22K2XD9343
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: WTE2TS5FTMMJQXHK
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       592,
			Deprecated: true,
		},

		// SKU: XWEGA3UJZ88J37T5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: YC9UG3ESW33SS2WK
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       95,
			Deprecated: true,
		},

		// SKU: YMCJTDYUBRJ9G3JJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1811,
		},

		// SKU: YT7Q7XWV392U2M45
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1912,
		},

		// SKU: YVBHSQT9PFQ3DB5S
		// Instance family: Storage optimized
		// Storage: 2 x 1024 SSD
		{
			Name:       "hi1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5376),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       3100,
			Deprecated: true,
		},

		// SKU: ZYJKKZNWFYWQDMCW
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1185,
		},
	},

	"eu-west-2": {

		// SKU: 2X6UN3SCD9TM673F
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     237,
		},

		// SKU: 3T2RCRCGD3A5FJVS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     950,
		},

		// SKU: 4CH6QCAN52UJ2AW2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     928,
		},

		// SKU: 6EUVB38T7YB7GF9S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     116,
		},

		// SKU: 6WQJ379XP5T8CCG9
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: 7ZJW8GF4W4CQTAMV
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1448,
		},

		// SKU: 92TGCNQTRRAJJAS7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     106,
		},

		// SKU: 9NVRASEGRRTWVG7A
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     119,
		},

		// SKU: 9VX7PNMQR7WWQG6U
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     181,
		},

		// SKU: A7UD5H5BJW5749J4
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5792,
		},

		// SKU: B5GZYSM5938VWDYM
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     772,
		},

		// SKU: CCC7SMBFCX33RDEG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: CERBK7JEFDU3SV3B
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     476,
		},

		// SKU: CFJ5RE2H7KHV9HK9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: CX2D324PPCTNMS57
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     362,
		},

		// SKU: DJ6ZCCPV7U8GMF93
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: E5XVGVP8X39F46VD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: EA9YCPFWYCCYZTTS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: GRFBXV37KF9VYJRF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     232,
		},

		// SKU: HDQ9VZCETFEMF7CB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     14,
		},

		// SKU: HEF8FVEMCGG7JDZ4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3712,
		},

		// SKU: JZWW5SBQZVBCV8AJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     424,
		},

		// SKU: KGHEQT4SACPTYQNB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     52,
		},

		// SKU: KZ3QWPFNPPDUDYW7
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6174,
		},

		// SKU: PAESNTWTS8RUGKY6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     212,
		},

		// SKU: QE4YJQ9HUCYB3CA9
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3087,
		},

		// SKU: QUSZR9Z7BVE8CD5Y
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1902,
		},

		// SKU: QX3BAPSZJEUCA9FJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     464,
		},

		// SKU: TK9JKUTPVAPNT9BR
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1544,
		},

		// SKU: UVNK5GZCP7NWANZR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2320,
		},

		// SKU: V277W3KQKBPHFGUN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: VN4E7YXFSEGMW9H7
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8403,
		},

		// SKU: WP9QSXA5MSKVTVNJ
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2896,
		},

		// SKU: XQ8FKG65FR9Z4ZPR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     26,
		},

		// SKU: Y6DK4ETN32KZE2ZQ
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16806,
		},

		// SKU: Y6EUBWYYGRBQAJKR
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     724,
		},
	},

	"sa-east-1": {

		// SKU: 2DQW6R4PKSZDG2T6
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     26010,
		},

		// SKU: 38R4NKAE2QECWRDD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     636,
		},

		// SKU: 3AW2EEGJZNBGCQTC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     162,
		},

		// SKU: 3KRHXWUDH2BDV4Y8
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2288,
		},

		// SKU: 4H6D39WMQKHE7G7X
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1120,
		},

		// SKU: 4KCYN288G4U4BEAG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2470,
		},

		// SKU: 5GTG8UXYNCRDW5C4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     155,
		},

		// SKU: 5V3T67JXMGR4TH34
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     159,
		},

		// SKU: 5YDAVRN5B6TSD9NF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     309,
		},

		// SKU: 6TN6BMN8S44CMRDW
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       117,
			Deprecated: true,
		},

		// SKU: 72TGAF9QN2XH5C5V
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       233,
			Deprecated: true,
		},

		// SKU: 7DYQRTNH9TX2QQCF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: 7JK3Y822S2U92G49
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1399,
		},

		// SKU: 84JB45JJDJXM67K4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     41,
		},

		// SKU: 8VWG8TTVN5G378AH
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       467,
			Deprecated: true,
		},

		// SKU: 9C4Q3RMVKSYS988K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     318,
		},

		// SKU: ABEXP4ERUWNM6W8J
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     700,
		},

		// SKU: ADMZJH7G4TK3XW72
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     95,
		},

		// SKU: AGV5N34XYJNRXKRG
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     325,
		},

		// SKU: B6W433SSVKEY68BH
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     761,
		},

		// SKU: BKHCG6HXJST7HSX4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     8960,
		},

		// SKU: CPGF97CV44XU5R37
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     190,
		},

		// SKU: CSJECZGEN7MJ4PNS
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1291,
			Deprecated: true,
		},

		// SKU: DE4F8GSMG9ZHARG8
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1300,
		},

		// SKU: DYHZ6YTHR4RRH3TS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     5088,
		},

		// SKU: EKJ89WCZYTF4ZNY8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1272,
		},

		// SKU: EY7JV9JX6H66P24B
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       323,
			Deprecated: true,
		},

		// SKU: F2TBQS4VX9RHU5VZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     560,
		},

		// SKU: F3B7MENDPCX44NH3
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     13005,
		},

		// SKU: FDUDDQXMYRBXXPU6
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     381,
		},

		// SKU: FM6RT86RTAN9G67D
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     4576,
		},

		// SKU: FSDH6G8FD9Z6EUM2
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       179,
			Deprecated: true,
		},

		// SKU: G3C2SRSGZ6EC7B2G
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     572,
		},

		// SKU: H8BY38DSCNH87FAD
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     163,
		},

		// SKU: HKWECXA9X8UKDCGK
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       718,
			Deprecated: true,
		},

		// SKU: HR6CM3GFVDT3BAMU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     20,
		},

		// SKU: JRRFY2C2V3KB63CJ
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     286,
		},

		// SKU: M6GCPQTQDNQK5XUW
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       58,
			Deprecated: true,
		},

		// SKU: MD2REDTVEQDNK4XJ
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       27,
			Deprecated: true,
		},

		// SKU: MR3NCU2GHXNVGXBE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2240,
		},

		// SKU: NXAU2M3KSE5AEQT5
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     350,
		},

		// SKU: PDY52X9T9DZY9CT5
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       645,
			Deprecated: true,
		},

		// SKU: PWMKDJAWK7S65AXS
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     9152,
		},

		// SKU: Q6HTSWNMCVFJPW2N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     4480,
		},

		// SKU: QBPF6VCNHZCVW9FP
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1144,
		},

		// SKU: SHPTTVUVD5P7R2FX
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2600,
		},

		// SKU: SUWWGGR72MSFMMCK
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     650,
		},

		// SKU: TGMMNAS3EPHPM7FD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     280,
		},

		// SKU: TRBTF7WUCDPWNYFM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     81,
		},

		// SKU: VZ7SHPDE4QVD6EJ6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     3180,
		},

		// SKU: W6ARQS59M94CBPW2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1235,
		},

		// SKU: W8DSYP8X87Q34DGY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     618,
		},

		// SKU: WCYXWR44SF5RDQSK
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2799,
		},

		// SKU: YW6RW65SRZ3Y2FP5
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5597,
		},

		// SKU: YXVD4WXRCSQ2W2JA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     648,
		},

		// SKU: ZGFYE9BANAUE5GK2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     324,
		},
	},

	"us-east-1": {

		// SKU: 2GCTBU78G22TGEXZ
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       44,
			Deprecated: true,
		},

		// SKU: 2S47E3PRB8XVH9QV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: 39748UVFEUKY3MVQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     398,
		},

		// SKU: 3DX9M63484ZSZFJV
		// Instance family: Compute optimized
		// Storage: 4 x 840
		{
			Name:       "cc2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       2000,
			Deprecated: true,
		},

		// SKU: 3H63S5QV423QAHHQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4560,
		},

		// SKU: 3RUU5T58T7XAFAAF
		// Instance family: Memory optimized
		// Storage: 2 x 120 SSD
		{
			Name:       "cr1.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(3200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3500,
			Deprecated: true,
		},

		// SKU: 3UP33R2RXCADSPSX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     800,
		},

		// SKU: 44BFC6CSKFS3KEJP
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: 47GP959QAF69YPG5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: 48VURD6MVAZ3M5JX
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     650,
		},

		// SKU: 4C7N4APU9GEUZ6H6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: 4J62B76AXGGMHG57
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     199,
		},

		// SKU: 4TCUDNKW7PMPSUT2
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2660,
		},

		// SKU: 58HUPRT96M5H8VUW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: 5KHB4S5E8M74C6ES
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       853,
			Deprecated: true,
		},

		// SKU: 639ZEB9D49ASFB26
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       20,
			Deprecated: true,
		},

		// SKU: 6TEX73KEE94WMEED
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       520,
			Deprecated: true,
		},

		// SKU: 8QZCKNB62EDMDT63
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13338,
		},

		// SKU: 8VCNEHQMSCQS4P39
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: 9G23QA9CK3NU3BRY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1591,
		},

		// SKU: A67CJDV9B3YBP6N6
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2600,
		},

		// SKU: A83EBS2T67UP72G2
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: AGHHWVT6KDRBWTWP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: ARPJFM962U4P5HAT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     796,
		},

		// SKU: ASDZTDFMC5425T7P
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     67,
		},

		// SKU: B4JUK3U7ZG63RGSF
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     840,
		},

		// SKU: CGJXHFUSGE546RV6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: CR9BJ8YMV2HGWRBH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: CRRB3H2DYHU6K9FV
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       4600,
			Deprecated: true,
		},

		// SKU: D5JBSPHEHDXDUWJR
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     665,
		},

		// SKU: DPCWVHKZ3AJBNM43
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: ECM8RSBXMC7F4WAS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3200,
		},

		// SKU: EN85M9PMPVGK77TA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "f1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     13200,
		},

		// SKU: EYGMRBWWFGSQBSBZ
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       350,
			Deprecated: true,
		},

		// SKU: GEDBVWHPGWMPYFMC
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     210,
		},

		// SKU: H48ZRU3X7FXGTGQM
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     105,
		},

		// SKU: H6T3SYB5G6QCVMZM
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1680,
		},

		// SKU: HZC9FAP4F9Y8JW67
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: J4T9ZF4AJ2DXE7SA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2000,
		},

		// SKU: J5XXRJGFYZHJVQZJ
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     333,
		},

		// SKU: J6U6GMEFVH686HBN
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       245,
			Deprecated: true,
		},

		// SKU: JVT9JXEVPBRUMA3N
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: KG9YWSKMK27V6W6V
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       175,
			Deprecated: true,
		},

		// SKU: KQTYTC2BEFA3UQD7
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: MGQXS8Z3TAKPMGUM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: MU4QGTJYWR6T73MZ
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1705,
			Deprecated: true,
		},

		// SKU: NARXYND9H74FTC7A
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       6820,
			Deprecated: true,
		},

		// SKU: NF67K4WANEWZZV22
		// Instance family: GPU instance
		// Storage: 2 x 840
		{
			Name:       "cg1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(1600),
			Mem:        23040,
			VirtType:   &hvm,
			Cost:       2100,
			Deprecated: true,
		},

		// SKU: P63NKZQXED5H7HUK
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1380,
		},

		// SKU: Q9SS9CE4RXPCX5KG
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: QCQ27AYFPSSTJG55
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       490,
			Deprecated: true,
		},

		// SKU: QG5G45WKDWDDHTFV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: QMW9CSCFTNV2H99M
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: QSNKQ8P78YXPTAH8
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: QY3YSEST3C6FQNQH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: RJZ63YZJGC58TPTS
		// Instance family: Storage optimized
		// Storage: 2 x 1024 SSD
		{
			Name:       "hi1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5376),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       3100,
			Deprecated: true,
		},

		// SKU: RKCQDTMY5DZS4JWT
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       980,
			Deprecated: true,
		},

		// SKU: RSMWKBGGTAAEV4RH
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2280,
		},

		// SKU: RSN2RZ8JSX98HFVM
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1330,
		},

		// SKU: SQ37ZQ2CZ2H95VDC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1140,
		},

		// SKU: TJCB42XUUBBP8KKF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4256,
		},

		// SKU: U3KDJRF6FGANNG5Z
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: U7343ZA6ABZUXFZ9
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     690,
		},

		// SKU: VA8Q43DVPX4YV6NG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: VHC3YWSZ6ZFZPJN4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     400,
		},

		// SKU: VZB4MZEV7XEAF6US
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "f1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1650,
		},

		// SKU: W8TS7DJMAYZW9FYR
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: WAWEH2Q4B3BTK68V
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7200,
		},

		// SKU: X4RWGEB2DKQGCWC2
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       130,
			Deprecated: true,
		},

		// SKU: XP5P8NMSB2W7KP3U
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5520,
		},

		// SKU: XVDMNM2WMYBYVW3T
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: YGU2QZY8VPP94FSR
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: YNFV4A5QUAMVDGKX
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6669,
		},

		// SKU: YUXKRQ5SQSHVKD58
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       87,
			Deprecated: true,
		},

		// SKU: Z73VPF4R8N955QMR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2128,
		},

		// SKU: ZA47RH8PF27SDZKP
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     420,
		},

		// SKU: ZDK96RPJGJ9MFZYU
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: ZESHW7CZVERW2BN2
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3410,
			Deprecated: true,
		},

		// SKU: ZJC9VZJF5NZNYSVK
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2760,
		},
	},

	"us-east-2": {

		// SKU: 24VNPYX565QU9U4D
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7200,
		},

		// SKU: 2N2QH6UEJZ5GUPT8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: 34ZEJZENQ3WGN6MA
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: 3VWERDY4UHEZUS9F
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2660,
		},

		// SKU: 5G4M46QWK9ZC9QDT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: 65YDGPXVA9GTBYCA
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     690,
		},

		// SKU: 82N8PRZ8U7GE8XRM
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: 93HAYG7AFZGMJKJY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3200,
		},

		// SKU: 9DMUVQNHGNHC7R92
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     333,
		},

		// SKU: 9F2Q74QEE8HZE6A8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: 9F7GGKJQGM6J387N
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: 9MXQF8NSPZESJJUW
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6669,
		},

		// SKU: AW2T9CUGJYGQKTY8
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3410,
			Deprecated: true,
		},

		// SKU: B7KJQVXZZNDAS23N
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: C2XHT7MUVASJ7UQ3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2000,
		},

		// SKU: DBBN8V6AE5WQCCGZ
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: DJPGFVCZAKBSEZ3N
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5520,
		},

		// SKU: DWQDRJ9AWUJ8BZKH
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13338,
		},

		// SKU: F9GPUA3E29X6GJVE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: GSMN37GEEUV2CC27
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     800,
		},

		// SKU: GWP38ESW2CNEVPUS
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       6820,
			Deprecated: true,
		},

		// SKU: GZSHSGVERG8544YU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: J3ASENPUVKBMMQG3
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: JNBD8ZZ5JSFMNNYR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: JVT8Z9JR8H2KTMEY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1591,
		},

		// SKU: K3Y5F2N8WVZ36GB2
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1140,
		},

		// SKU: M6S6AKBD4WYEHANV
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1330,
		},

		// SKU: MD5WDRNPRHM2XAGC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     398,
		},

		// SKU: MDSBS66AS4N5QZPV
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: MW8UQZATN9TSYX2K
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: P3R3B44DZXKTKMYR
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1705,
			Deprecated: true,
		},

		// SKU: PBMCNRQPJ8XA43PT
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1380,
		},

		// SKU: PSAR6KAESEV4S8JY
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2760,
		},

		// SKU: Q236HUQXMUFFR9AN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: QPP4W9YVZSKJSZ4A
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4560,
		},

		// SKU: R4CKBWU7M6GSC7QB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: R99FC88U7735H9RR
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     199,
		},

		// SKU: RGU9HRNUAS2TX83W
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     796,
		},

		// SKU: RHW8EAZNVJA4KSWC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     400,
		},

		// SKU: RWY5P9FB6JQYYM78
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2128,
		},

		// SKU: RXYEZ3QXT2GWNMRK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: UMK29BWNESP838CA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: URPTCUXU96S8XJUV
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     665,
		},

		// SKU: VU6WDT5USNCYBZEU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: W4ZRFQ4Q6W3P22C4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4256,
		},

		// SKU: WTADZ55ETP6ZEN73
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: Y9Z92WYATPYKXSM2
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       853,
			Deprecated: true,
		},

		// SKU: YQZKQQ43X7CBE4ZD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: ZDDYX9E844C9PPSA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2280,
		},

		// SKU: ZWW6ZZSFWQM7H534
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: ZZNXUPSQ9BSA2M88
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},
	},

	"us-gov-west-1": {

		// SKU: 28MYFN2XX2772KFF
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       293,
			Deprecated: true,
		},

		// SKU: 2DKYMP8CF8F43GMZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     159,
		},

		// SKU: 3GCCV4BFBUGPG9KP
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6016,
		},

		// SKU: 5FR2446ZVKVMX5JH
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     752,
		},

		// SKU: 6CVNTPV3HMNBWRCF
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     84,
		},

		// SKU: 6DWB5HXXA6HTFHVP
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       106,
			Deprecated: true,
		},

		// SKU: 6J7ZTVPXJKAX6EB3
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     672,
		},

		// SKU: 6PMSJ4V5N26J36BG
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     399,
		},

		// SKU: 6R6V4F6BTSJKCC7Q
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     958,
		},

		// SKU: 6VTUAGVX6J37U9GQ
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: 7AB3REVQH3BB3B5F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     448,
		},

		// SKU: 7BBW4T3J39FZV85H
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       53,
			Deprecated: true,
		},

		// SKU: 7QKNUBJTTFR95RQW
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     828,
		},

		// SKU: 89EV5BSMPDHAKNGR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: 8GK9ASUZPC2EWUZ4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: 93BGE6ZRJXNGXPKQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5280,
		},

		// SKU: 98882H5A8BVY29GC
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       423,
			Deprecated: true,
		},

		// SKU: 9PTNMYF3BTTMXQXW
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       1022,
			Deprecated: true,
		},

		// SKU: A5EN5X5DCNGRXPV6
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: BP3D9JS4K9UCBBZ3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     112,
		},

		// SKU: BXAR9D46EJJXYWD9
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: CMWE8B43GS86ZUFX
		// Instance family: Compute optimized
		// Storage: 4 x 840
		{
			Name:       "cc2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       2250,
			Deprecated: true,
		},

		// SKU: CNA2FG87Z6YSZ865
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1320,
		},

		// SKU: CQDRUMDTUB5DGT63
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       586,
			Deprecated: true,
		},

		// SKU: CXM9YR66XHMFFJDD
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     168,
		},

		// SKU: D8J594N2J5BEEN7U
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     319,
		},

		// SKU: DGRA5487A6WJRREE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5107,
		},

		// SKU: EFP49PY9RHXAQAHZ
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6624,
		},

		// SKU: EFPYDPDNRQTJBP3V
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16006,
		},

		// SKU: EG7K36A6WPQ4YM89
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       2045,
			Deprecated: true,
		},

		// SKU: EXPVWKGGASGW4CEJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     479,
		},

		// SKU: FBKCPG7Y9BT98V6X
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2553,
		},

		// SKU: FUTAFX3ARUD9RJJQ
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       211,
			Deprecated: true,
		},

		// SKU: FZTFNX6E6VTCH3BH
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1171,
			Deprecated: true,
		},

		// SKU: GWPUCHQBWTA9GWE7
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       24,
			Deprecated: true,
		},

		// SKU: H2JDFG4KAR24BKSQ
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3008,
		},

		// SKU: HVTS332KRPZUNRU4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: J9DSB6BXY5KQK7F9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: K5CWXN5HSW7SME2R
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     798,
		},

		// SKU: KNCYTRUPV9RQUJ4J
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       4091,
			Deprecated: true,
		},

		// SKU: KT5XYNZBAZ6PAHHJ
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: MCV6F96BD976FX6F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: MS9288AGUM9ZHPZ3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     224,
		},

		// SKU: N5HCW4AX6C3QWS7P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     14,
		},

		// SKU: P76AN6DXWYCD69GD
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: PB9ZD3B8VHMC8YD9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2520,
		},

		// SKU: PKQU74FNEQGYH8AN
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8003,
		},

		// SKU: Q864N6CVS6UKQ3WZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     239,
		},

		// SKU: QFVQE44FY9YSYTTR
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1915,
		},

		// SKU: QG4YUM59ZPHHDF39
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1504,
		},

		// SKU: R75SXSFUFBHZPRSS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: R8QUU7JSJKZP3WYX
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1656,
		},

		// SKU: RG9GWJUK8NQBF57M
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       157,
			Deprecated: true,
		},

		// SKU: RQ2E87MS29TV3CDY
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: RY97T2T2385G4KXW
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: SPN86ZCGXTJQR3TZ
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       5520,
			Deprecated: true,
		},

		// SKU: SZ4U6UEUN6MP8R7D
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1276,
		},

		// SKU: T49MVZWJMT2P8BS5
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3312,
		},

		// SKU: U4DXFARHKHZQGSHP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     638,
		},

		// SKU: U7DC62BMZPV7YRYS
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2640,
		},

		// SKU: UF55C3729FAEBSYW
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       8183,
			Deprecated: true,
		},

		// SKU: VR6H7WYKMFWVPAUK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     28,
		},

		// SKU: VXKKRPEQERAMFSFJ
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     336,
		},

		// SKU: W6ABH88PE5BKSJZQ
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: WVKVYKA9HGPSEYHF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: XBS8PBKJMH9G6SDB
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       628,
			Deprecated: true,
		},

		// SKU: XYAF9UA2YWC7DNZT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: YYGVB3MJTV4Q348E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     56,
		},

		// SKU: ZZTR42B59D85VUCY
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     126,
		},
	},

	"us-west-1": {

		// SKU: 2EBX6PMG5FBY92KC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     234,
		},

		// SKU: 2M44JQQN3ZP9874A
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2808,
		},

		// SKU: 3CUFXRHG38QDZNHT
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       7502,
			Deprecated: true,
		},

		// SKU: 3MU3SGMJKBFABKE9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     117,
		},

		// SKU: 4GAV6VD5FWD8W8B4
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1912,
		},

		// SKU: 5JEH726H8KDYDWPP
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       25,
			Deprecated: true,
		},

		// SKU: 5JQZHK4R7B7U6R3D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     468,
		},

		// SKU: 6H5EA7PH345UPBVC
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       1100,
			Deprecated: true,
		},

		// SKU: 76F5CX9DPY2B39T5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     244,
		},

		// SKU: 7AJJ9ANNCNNX5WY6
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       95,
			Deprecated: true,
		},

		// SKU: 7QXMEKWFBRKXCR5T
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     478,
		},

		// SKU: 7T9BSUQBURKAGP2T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     61,
		},

		// SKU: 87ZU79BG86PYWTSG
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     702,
		},

		// SKU: 8RXDZ4H62GHK7Y7N
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     154,
		},

		// SKU: 8XWJMTS7HY3XFPBD
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3751,
			Deprecated: true,
		},

		// SKU: CTG879VYY65QE94C
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     616,
		},

		// SKU: CTW2PCJK622MCHPV
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2964,
		},

		// SKU: D7EXY7CNAHW9BTHD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     122,
		},

		// SKU: DCM8ZJ894B27CQ8G
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3068,
		},

		// SKU: ECGYTDY66NMATM39
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6136,
		},

		// SKU: EMKWZYXVQQ6HNNKC
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: EVYB78ZE853DF3CC
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     308,
		},

		// SKU: F4RA9QG9BAGEHG29
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6250,
		},

		// SKU: F5KDHTDT282JC5R3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     936,
		},

		// SKU: FA7B379WCHNBVMNU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     997,
		},

		// SKU: G3EQX8J7UNTWEVC7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     498,
		},

		// SKU: GRGVZYA9QN53ASFB
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       190,
			Deprecated: true,
		},

		// SKU: GRKGK4BN2EGBK686
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     7,
		},

		// SKU: GSN36ZXJH466ES5F
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1993,
		},

		// SKU: H8QFMXWT89NGP6VU
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: HQXQHXXJPTDD2HW7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2371,
		},

		// SKU: HSP446QHF6SX27BF
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: J5UNF2XTPQCS5N59
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1482,
		},

		// SKU: J6ATSGQU69WY57Z8
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: J6T772QKJ5B49GMC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     15,
		},

		// SKU: JHV4BKWFVMXQ2T6R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     31,
		},

		// SKU: JJRB8PAXGN6JTB3D
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       379,
			Deprecated: true,
		},

		// SKU: JUT2GARXC5CE93CM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     249,
		},

		// SKU: M9QTFFFRQC7KYS3Q
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: MMX6UFM34E65AQDY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4742,
		},

		// SKU: MSDZGAVV7DCFG86R
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     592,
		},

		// SKU: MSR496BA8MMSSHES
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     296,
		},

		// SKU: MUR692RSQPYGJKT6
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: N8JKAXF693B9PWC2
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5504,
		},

		// SKU: P5VFWENV9YDAQVFH
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       47,
			Deprecated: true,
		},

		// SKU: PSNEQGH9XVX3FSE8
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     956,
		},

		// SKU: PYE3YUCCZCUBD7Z6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     124,
		},

		// SKU: QFP73PEKRJ8N2C3P
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1534,
		},

		// SKU: R33YWMGU2UZNZRG5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     148,
		},

		// SKU: RMARGSE4CA952UDV
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     371,
		},

		// SKU: RT9NWVFZWQDDRNES
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     77,
		},

		// SKU: S6XCDNADM5DDPNUP
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     239,
		},

		// SKU: S7T2R43H93585V7D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2340,
		},

		// SKU: TMYBBH8MNS5KCXDG
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1876,
			Deprecated: true,
		},

		// SKU: V3KZ88KMZGE7XS8G
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     781,
		},

		// SKU: VQ4GF8ZANSG56N6Z
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3744,
		},

		// SKU: VZ7V29X35F98VENC
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     741,
		},

		// SKU: VZA22HBW4H82PHGF
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       550,
			Deprecated: true,
		},

		// SKU: WNGPF3ZVZEAVC7FH
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3125,
		},

		// SKU: X7EQJS6PVD6RDBPD
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       938,
			Deprecated: true,
		},

		// SKU: XFDUQKGKAPWHG6P5
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       148,
			Deprecated: true,
		},

		// SKU: XKAXY7525KWTBXQJ
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       275,
			Deprecated: true,
		},

		// SKU: XN66GCWSFVKQP8UD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1185,
		},

		// SKU: YPK2GJ5Z9TMRRSK7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: YY26V92H8QNEPQER
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     185,
		},

		// SKU: ZXHBSSRM8QPYG3GC
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1563,
		},

		// SKU: ZXWJ6NZFPEP89DVZ
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       592,
			Deprecated: true,
		},
	},

	"us-west-2": {

		// SKU: 2ES9C4RF3WGQZAQN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.medium",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(40),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: 2J3G8CUM4UVYVFJH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.nano",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(5),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 2JUMD5V8V9V6D9JC
		// Instance family: General purpose
		// Storage: 1 x 410
		{
			Name:       "m1.medium",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        3840,
			VirtType:   &paravirtual,
			Cost:       87,
			Deprecated: true,
		},

		// SKU: 2K68UGQ7NNMG757D
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: 34G9YZGUJNTY6HG9
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13338,
		},

		// SKU: 3FTJBHZMWT7D76MD
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6495),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     796,
		},

		// SKU: 3U96RB67X35NC337
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: 4D5UJSPKUSHPMT7B
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4560,
		},

		// SKU: 4SCSPCTHFCXYY6GT
		// Instance family: General purpose
		// Storage: 4 x 420
		{
			Name:       "m1.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        15360,
			VirtType:   &paravirtual,
			Cost:       350,
			Deprecated: true,
		},

		// SKU: 584JD8RT9GR57BFS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1623),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     199,
		},

		// SKU: 59UCQMPPJJA5QRY4
		// Instance family: Storage optimized
		// Storage: 2 x 1.9 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: 5EPUM8UK2RTQKW5E
		// Instance family: Compute optimized
		// Storage: 4 x 420
		{
			Name:       "c1.xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        7168,
			VirtType:   &paravirtual,
			Cost:       520,
			Deprecated: true,
		},

		// SKU: 5WF4ZHD94FEY4YDX
		// Instance family: Memory optimized
		// Storage: 1 x 850
		{
			Name:       "m2.2xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(400),
			Mem:        35021,
			VirtType:   &paravirtual,
			Cost:       490,
			Deprecated: true,
		},

		// SKU: 672XUMHHEC7SYFT7
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     105,
		},

		// SKU: 6P434MFC33XNF65Z
		// Instance family: Compute optimized
		// Storage: 4 x 840
		{
			Name:       "cc2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       2000,
			Deprecated: true,
		},

		// SKU: 7528XD9PPHXN6NN2
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:     "g2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11647),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     2600,
		},

		// SKU: 7RMRB492WTPDQ5Z4
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:     "r3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: 7TXU2CNQ5EQ3DWYS
		// Instance family: Storage optimized
		// Storage: 1 x 1.9 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: 83KC74WNYCKW5CYN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: 8ZTRUHSHU2GT4CDF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2128,
		},

		// SKU: 9GHZN7VCNV2MGV4N
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     840,
		},

		// SKU: 9HMZJQ3SKEW4P7ST
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:       "i2.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       3410,
			Deprecated: true,
		},

		// SKU: 9NSBRG2FE96XPHXK
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     210,
		},

		// SKU: 9RYWCN75CJX2C238
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1380,
		},

		// SKU: AKWXS6FJQE43VAZ9
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:       "i2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       6820,
			Deprecated: true,
		},

		// SKU: B2M25Y2U9824Q5TG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(672),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: BMEYUTP658QKQRTP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     800,
		},

		// SKU: BNBBBYA6WNXQ3TZV
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2760,
		},

		// SKU: CP2TNWZCKSRY486E
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: CVFJSWADA39YVNW2
		// Instance family: General purpose
		// Storage: 2 x 420
		{
			Name:       "m1.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        7680,
			VirtType:   &paravirtual,
			Cost:       175,
			Deprecated: true,
		},

		// SKU: D8RPR5AJPDXSC9DF
		// Instance family: General purpose
		// Storage: 1 x 160
		{
			Name:       "m1.small",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(100),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       44,
			Deprecated: true,
		},

		// SKU: DTDPGWV4T5RAVP44
		// Instance family: Storage optimized
		// Storage: 24 x 2000
		{
			Name:       "hs1.8xlarge",
			Arches:     amd64,
			CpuCores:   17,
			CpuPower:   instances.CpuPower(4760),
			Mem:        119808,
			VirtType:   &hvm,
			Cost:       4600,
			Deprecated: true,
		},

		// SKU: DW2RY9FQP8VE6V74
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5520,
		},

		// SKU: E7T5V224CMC9A43F
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: F649UJZP8WARX37N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4256,
		},

		// SKU: F6544RN8RCJHYC5Z
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     420,
		},

		// SKU: FQBEE3KF5TRFG5ZF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: GMTWE5CTY4FEUYDN
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:     "r3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     665,
		},

		// SKU: J9H28ZVG9UDW7CX4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     400,
		},

		// SKU: JH68FQ55JWMC4CG9
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     67,
		},

		// SKU: JNZ6ESS4AS6RFUAF
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:     "r3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1330,
		},

		// SKU: K24TXC5VMFQZ53MC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(811),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: K5MSZ8JUCECB23H9
		// Instance family: Storage optimized
		// Storage: 2 x 1024 SSD
		{
			Name:       "hi1.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5376),
			Mem:        61952,
			VirtType:   &hvm,
			Cost:       3100,
			Deprecated: true,
		},

		// SKU: KCTVWQQPE9VFXHGP
		// Instance family: Memory optimized
		// Storage: 1 x 420
		{
			Name:       "m2.xlarge",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        17511,
			VirtType:   &paravirtual,
			Cost:       245,
			Deprecated: true,
		},

		// SKU: MBANS55WTSZ5HYS8
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:     "r3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     333,
		},

		// SKU: MNG2Y3YRJK7GPKQR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1680,
		},

		// SKU: MWG952JV6DF8YBYE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.10xlarge",
			Arches:   amd64,
			CpuCores: 40,
			CpuPower: instances.CpuPower(13440),
			Mem:      163840,
			VirtType: &hvm,
			Cost:     2000,
		},

		// SKU: N4D3MGNKSH7Q9KT3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(10),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: N5F93UFYUKWKB8KE
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:       "i2.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       853,
			Deprecated: true,
		},

		// SKU: NA6BZ2FSPKCZGWTA
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "r3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2660,
		},

		// SKU: P9ZPWZF7CCR7MS77
		// Instance family: Compute optimized
		// Storage: 1 x 350
		{
			Name:       "c1.medium",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(200),
			Mem:        1741,
			VirtType:   &paravirtual,
			Cost:       130,
			Deprecated: true,
		},

		// SKU: PWCUVRQBX67NDRMJ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7200,
		},

		// SKU: PWGQ6MKD7A6EHVXN
		// Instance family: Memory optimized
		// Storage: 2 x 120 SSD
		{
			Name:       "cr1.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(3200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3500,
			Deprecated: true,
		},

		// SKU: QBRGX479S7RZ4QEA
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:     "g2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2911),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     650,
		},

		// SKU: QBYJCF2RGQTF5H5D
		// Instance family: Memory optimized
		// Storage: 2 x 840
		{
			Name:       "m2.4xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(800),
			Mem:        70042,
			VirtType:   &paravirtual,
			Cost:       980,
			Deprecated: true,
		},

		// SKU: RPV38M7F4AHYVV74
		// Instance family: Storage optimized
		// Storage: 4 x 1.9 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: T3BJKYM5NU9B6XGY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r4.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: TKYAD5H42TYVUTMG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3200,
		},

		// SKU: UNB4R4KS4XXHQFD2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(14615),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1591,
		},

		// SKU: VBKTV5SAFT4WNV9X
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: W4WYWEGHXCH4WAQ7
		// Instance family: Storage optimized
		// Storage: 1 x 0.475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: WE87HQHP89BK3AXK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: X5NPE8XF7KHV7AAD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m4.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     200,
		},

		// SKU: XK9YF9AJ9EBH7W4U
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.small",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: XUTTHNZ5B5VJKKDE
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:       "t1.micro",
			Arches:     both,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(20),
			Mem:        628,
			VirtType:   &paravirtual,
			Cost:       20,
			Deprecated: true,
		},

		// SKU: YBN8Q7AQJD9ZT57S
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c4.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3247),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     398,
		},

		// SKU: YKQ7DN6CCEDVB8Q2
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: YMWQW8W92QHE628D
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     690,
		},

		// SKU: YT7P3Q75RMN2RX4J
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: YVDVYGHJPYJH6X4W
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1140,
		},

		// SKU: YYCVC33TV9QRD5SF
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2280,
		},

		// SKU: YZN6FRZW8JHKE3HV
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6669,
		},

		// SKU: Z2A6DZ9V9QZ363BC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: Z5V5GD3UGX5Q3YME
		// Instance family: Storage optimized
		// Storage: 8 x 1.9 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: ZJZ9FZDG4RFMC5DU
		// Instance family: Storage optimized
		// Storage: 1 x 0.95 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: ZKYE77DHMC32Y9BK
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:       "i2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1705,
			Deprecated: true,
		},
	},
}
