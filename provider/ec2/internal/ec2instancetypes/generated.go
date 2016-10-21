// Copyright 2016 Canonical Ltd.
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

// Version: 20161017231909
// Publication date: 2016-10-17 23:19:09 +0000 UTC
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
			Cost:     5563,
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
			Cost:     695,
		},

		// SKU: 4REMK3MMXCZ55ZX3
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8004,
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
			Cost:     133,
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
			Cost:     174,
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
			Cost:     20,
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
			Cost:     348,
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
			Cost:     1061,
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
			Cost:     80,
		},

		// SKU: ERVWZ4V3UBYH4NQH
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     26,
		},

		// SKU: EZCSGZJ8PMXA2QF2
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1000,
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
			Cost:     10,
		},

		// SKU: FBUWUPNC8FXRUS5W
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4002,
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
			Cost:     3477,
		},

		// SKU: KM8DYQWHEC32CGGX
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2001,
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
			Cost:     2122,
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
			Cost:     160,
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
			Cost:     531,
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
			Cost:     265,
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
			Cost:     1391,
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
			Cost:     40,
		},
	},

	"ap-northeast-2": {

		// SKU: 3MBNRY22Y6A2W6WY
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:     "m3.medium",
			Arches:   amd64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(350),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     91,
		},

		// SKU: 3UWMR4BVSMJ3PTQ5
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: 45D7HY2M47KUYJXR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:     "c3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(12543),
			Mem:      61440,
			VirtType: &hvm,
			Cost:     1839,
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
			Cost:     120,
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
			Cost:     331,
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

		// SKU: 6K25ZNG5NAXQC5AB
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
			Cost:     1321,
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

		// SKU: 7GTTQXNXREPMU7WY
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:     "m3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: 7MQ7AMJWV8BPWH88
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:     "c3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6271),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     919,
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
			Cost:     80,
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

		// SKU: 98ZFCFAZXXRGF7CG
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
			Cost:     5285,
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
			Cost:     165,
		},

		// SKU: BHS4CH7UVYY7QN2H
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4002,
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
			Cost:     955,
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
			Cost:     160,
		},

		// SKU: CFXCUT5A22XNZ43Y
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:     "c3.large",
			Arches:   both,
			CpuCores: 2,
			CpuPower: instances.CpuPower(783),
			Mem:      3840,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: DTEVN35HD43BM5ST
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:     "m3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      30720,
			VirtType: &hvm,
			Cost:     732,
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
			Cost:     239,
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
			Cost:     1910,
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

		// SKU: K79C7JVTDAKRA842
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:     "m3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     183,
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
			Cost:     10,
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
			Cost:     660,
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
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2001,
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
			Cost:     478,
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
			Cost:     40,
		},

		// SKU: R7GFV82WRF8QTZYP
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:     "c3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1567),
			Mem:      7680,
			VirtType: &hvm,
			Cost:     230,
		},

		// SKU: RM2KHQ9S45BW6B7M
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:     "c3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3135),
			Mem:      15360,
			VirtType: &hvm,
			Cost:     460,
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
			Cost:     3303,
		},

		// SKU: YG3C8Z588MN6BXGW
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8004,
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
			Cost:     20,
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
			Cost:     2195,
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
			Cost:     169,
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
			Cost:     9,
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
			Cost:     38,
		},

		// SKU: 7HYM8MHNNFW2NN6T
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3867,
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
			Cost:     549,
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
			Cost:     337,
		},

		// SKU: AFU2HU8WVY9T6QAK
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1933,
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
			Cost:     5400,
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
			Cost:     3375,
		},

		// SKU: G4283CPK5MQ5QQ2A
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     7733,
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
			Cost:     152,
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
			Cost:     19,
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
			Cost:     137,
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
			Cost:     76,
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
			Cost:     1097,
		},

		// SKU: TPTBS44NNEJN3HUG
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     967,
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
			Cost:     275,
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
			Cost:     1350,
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
			Cost:     675,
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
			Cost:     1155,
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
			Cost:     1421,
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
			Cost:     40,
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

		// SKU: 8E9KB9CNE94Z4AHE
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4070,
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
			Cost:     5685,
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
			Cost:     3553,
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
			Cost:     178,
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
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2035,
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
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1018,
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
			Cost:     80,
		},

		// SKU: J65Z38YCBYKP7Q49
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     20,
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
			Cost:     355,
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
			Cost:     144,
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
			Cost:     20,
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
			Cost:     10,
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
			Cost:     289,
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
			Cost:     711,
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
			Cost:     578,
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
			Cost:     160,
		},

		// SKU: VE5MWWHUXS2VR8DV
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8140,
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
			Cost:     2310,
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
			Cost:     673,
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
			Cost:     137,
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
			Cost:     2195,
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
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2035,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4070,
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
			Cost:     168,
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
			Cost:     20,
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
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     20,
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
			Cost:     1345,
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
			Cost:     40,
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
			Cost:     1097,
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
			Cost:     549,
		},

		// SKU: FJURXZQ9HT9HN2YJ
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1018,
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
			Cost:     160,
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
			Cost:     10,
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
			Cost:     5381,
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
			Cost:     336,
		},

		// SKU: RW8353QQ8DWZ4WQD
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8140,
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
			Cost:     3363,
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
			Cost:     275,
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
			Cost:     80,
		},
	},

	"eu-central-1": {

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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4051,
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
			Cost:     60,
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
			Cost:     7,
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
			Cost:     120,
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
			Cost:     143,
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
			Cost:     1069,
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
			Cost:     570,
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
			Cost:     30,
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
			Cost:     2138,
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
			Cost:     285,
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
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2025,
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
			Cost:     1140,
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
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1012,
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
			Cost:     534,
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
			Cost:     4560,
		},

		// SKU: SMTUMBHX6YKRBJQB
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8102,
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
			Cost:     15,
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
			Cost:     134,
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
			Cost:     2850,
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
			Cost:     267,
		},
	},

	"eu-west-1": {

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
			Name:     "cg1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      23040,
			VirtType: &hvm,
			Cost:     2360,
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
			Cost:     112,
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
			Cost:     264,
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
			Cost:     132,
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
			Cost:     528,
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
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     7502,
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
			Cost:     953,
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
			Cost:     1056,
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
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     20,
		},

		// SKU: DYTSK9JJGPSR6VQB
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1876,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3751,
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
			Cost:     56,
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

		// SKU: KKQD5EPCF8JFUDDA
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     938,
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
			Cost:     28,
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
			Cost:     2641,
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
			Cost:     4226,
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
			Cost:     7,
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
			Cost:     14,
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
			Cost:     477,
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
			Cost:     238,
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
			Cost:     119,
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
			Cost:     1906,
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
			Cost:     685,
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
			Cost:     216,
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
			Cost:     2600,
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
			Cost:     163,
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
			Cost:     171,
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
			Cost:     325,
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
			Cost:     13,
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
			Cost:     54,
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
			Cost:     343,
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
			Cost:     5480,
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
			Cost:     1370,
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
			Cost:     27,
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
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     27,
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
			Cost:     108,
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
			Cost:     3425,
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
			Cost:     1300,
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
			Cost:     650,
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
			Cost:     419,
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
			Cost:     958,
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
			Cost:     239,
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
			Cost:     105,
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
			Cost:     209,
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
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     853,
		},

		// SKU: 639ZEB9D49ASFB26
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     20,
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
			Cost:     120,
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
			Cost:     1675,
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
			Cost:     6,
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
			Cost:     838,
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
			Cost:     3830,
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
			Cost:     13,
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
			Cost:     2394,
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

		// SKU: MU4QGTJYWR6T73MZ
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1705,
		},

		// SKU: NARXYND9H74FTC7A
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6820,
		},

		// SKU: NF67K4WANEWZZV22
		// Instance family: GPU instance
		// Storage: 2 x 840
		{
			Name:     "cg1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      23040,
			VirtType: &hvm,
			Cost:     2100,
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
			Cost:     104,
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
			Cost:     52,
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
			Cost:     26,
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
			Cost:     479,
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

		// SKU: ZESHW7CZVERW2BN2
		// Instance family: Storage optimized
		// Storage: 4 x 800 SSD
		{
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3410,
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
			Cost:     52,
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
			Cost:     105,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3410,
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
			Cost:     104,
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
			Cost:     2394,
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
			Cost:     13,
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
			Cost:     958,
		},

		// SKU: GWP38ESW2CNEVPUS
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6820,
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
			Cost:     239,
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
			Cost:     6,
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
			Cost:     1675,
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
			Cost:     419,
		},

		// SKU: P3R3B44DZXKTKMYR
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1705,
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
			Cost:     120,
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
			Cost:     209,
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
			Cost:     838,
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
			Cost:     479,
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

		// SKU: Y9Z92WYATPYKXSM2
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     853,
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
			Cost:     26,
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
			Cost:     1008,
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
			Cost:     151,
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
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1022,
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
			Cost:     124,
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
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     2045,
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
			Cost:     504,
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
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     24,
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
			Cost:     4840,
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
			Cost:     605,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     4091,
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
			Cost:     1210,
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
			Cost:     15,
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
			Cost:     3025,
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
			Cost:     252,
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
			Cost:     2016,
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
			Cost:     7,
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

		// SKU: UF55C3729FAEBSYW
		// Instance family: Storage optimized
		// Storage: 8 x 800 SSD
		{
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     8183,
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
			Cost:     31,
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
			Cost:     303,
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
			Cost:     126,
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
			Cost:     62,
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
			Cost:     279,
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
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     7502,
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
			Cost:     140,
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
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     25,
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
			Cost:     559,
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
			Cost:     68,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3751,
		},

		// SKU: A78J2XTXBB29NGT4
		// Instance family: Memory optimized
		// Storage: 2 x 1,920
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     17340,
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
			Cost:     136,
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
			Cost:     1117,
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
			Cost:     1049,
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
			Cost:     524,
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
			Cost:     8,
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
			Cost:     2098,
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
			Cost:     17,
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
			Cost:     34,
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
			Cost:     262,
		},

		// SKU: N63KE7N2NCMCYWXC
		// Instance family: Memory optimized
		// Storage: 1 x 1,920
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8670,
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
			Cost:     131,
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
			Cost:     2793,
		},

		// SKU: TMYBBH8MNS5KCXDG
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1876,
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
			Cost:     4469,
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
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     938,
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
			Cost:     52,
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
			Cost:     6,
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
			Cost:     838,
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
			Cost:     209,
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
			Name:     "i2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3410,
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
			Name:     "i2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6820,
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
			Cost:     120,
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
			Cost:     958,
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
			Cost:     479,
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
			Cost:     105,
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
			Cost:     2394,
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
			Cost:     13,
		},

		// SKU: N5F93UFYUKWKB8KE
		// Instance family: Storage optimized
		// Storage: 1 x 800 SSD
		{
			Name:     "i2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     853,
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
			Cost:     3830,
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
			Cost:     1675,
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
			Cost:     104,
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
			Cost:     239,
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
			Cost:     26,
		},

		// SKU: XUTTHNZ5B5VJKKDE
		// Instance family: Micro instances
		// Storage: EBS only
		{
			Name:     "t1.micro",
			Arches:   both,
			CpuCores: 1,
			CpuPower: instances.CpuPower(20),
			Mem:      628,
			VirtType: &paravirtual,
			Cost:     20,
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
			Cost:     419,
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

		// SKU: ZKYE77DHMC32Y9BK
		// Instance family: Storage optimized
		// Storage: 2 x 800 SSD
		{
			Name:     "i2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1705,
		},
	},
}
