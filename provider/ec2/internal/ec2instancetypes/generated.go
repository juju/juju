// Copyright 2019 Canonical Ltd.
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
	arm64       = []string{arch.ARM64} // AWS Graviton Processor
	both        = []string{arch.AMD64, arch.I386}
)

// Version: 20190807200923
// Publication date: 2019-08-07 20:09:23 +0000 UTC
//
// This pricing list is for informational purposes only. All prices are subject to the additional terms included in the pricing pages on http://aws.amazon.com. All Free Tier prices are also subject to the terms included at https://aws.amazon.com/free/

var allInstanceTypes = map[string][]instances.InstanceType{

	"ap-east-1": {

		// SKU: 26G6RQCBMMDDGG7K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     334,
		},

		// SKU: 2REFZ4PXWWEJYQ54
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3888,
		},

		// SKU: 2Y2V5UBKTCGHE4AP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     7,
		},

		// SKU: 36MVCKK9X7DXXNK9
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     7656,
		},

		// SKU: 3UBWTTRFPMJVSUD9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: 4SEHGPX4SEPTDT43
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4428,
		},

		// SKU: 5D4KPPK9ZJDTYZJA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     467,
		},

		// SKU: 72MVS9NXUFKFHVWN
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1914,
		},

		// SKU: 7QK4ZFUAH9MZDAUR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2112,
		},

		// SKU: 7URKC9AGEEM95R6S
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1240,
		},

		// SKU: 8QPAHM9MKEP5PEQF
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3720,
		},

		// SKU: 8RR98TE2SM6X42M2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: 9R3F47NPR5BBVP7K
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     310,
		},

		// SKU: AMHRMNM3HNYJY7UH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5344,
		},

		// SKU: ANAS4E5WK63VJZXG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8016,
		},

		// SKU: B24VZQ245HHTC7D5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     58,
		},

		// SKU: BP8GDVRU5EDQN6DR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6336,
		},

		// SKU: C9BYT7MV38F5UAAQ
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6592,
		},

		// SKU: C9GDJDUQYTPN6FT5
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4200,
		},

		// SKU: CCXRU6TATSJ2ZJR8
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7440,
		},

		// SKU: CZCST55XES66B6JT
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     167,
		},

		// SKU: D8JMFZU99X4G69RD
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     620,
		},

		// SKU: DF845UK3SRGJES3Y
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     155,
		},

		// SKU: EEFZUR65GG8SJ556
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     412,
		},

		// SKU: EEGBU8ZU8TMZMM5X
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     123,
		},

		// SKU: F3KJ258ZAWKCZWD5
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8400,
		},

		// SKU: F9JW9SVB2E9T3VTU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4008,
		},

		// SKU: FKJ94FPCX937Z47R
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1400,
		},

		// SKU: FUEYMQFU5HZNBTG4
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     10638,
		},

		// SKU: GPQYRPZNMTRUJUHK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1336,
		},

		// SKU: GZS33MRX5JT3UZPA
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     246,
		},

		// SKU: HD3ZTK9MG87DVPPX
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8400,
		},

		// SKU: J2UNFUKJM3TE7NUF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3168,
		},

		// SKU: JBWJNX3K6PV6VVTQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     233,
		},

		// SKU: JFURWYGE9Z5YZXE6
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     492,
		},

		// SKU: JSDKSKSM9NPNTFDB
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2800,
		},

		// SKU: K8MVRFQ498NUCVA3
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     700,
		},

		// SKU: KNTJ6CQ985WC7W3W
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     350,
		},

		// SKU: MK986TZK9VJ6BJDN
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1648,
		},

		// SKU: MWDJJHACZACH3QC5
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     21276,
		},

		// SKU: NMUXPGJKW5UD9A64
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: PC9GZ6A87QU9SCHX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1056,
		},

		// SKU: PFRW82WEUFXKKAUY
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     206,
		},

		// SKU: QME4VXFVGS854HNG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     14,
		},

		// SKU: RKDDATTSSWFJPXQS
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     824,
		},

		// SKU: SACK3RZMYDJ7P36C
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     528,
		},

		// SKU: SQJWYW9PWRRU6VNS
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     175,
		},

		// SKU: SX79ZMRC8Z3YAHEE
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     957,
		},

		// SKU: TMGZ999XPRUY4QFP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     132,
		},

		// SKU: TQRVZ94DBG874RKA
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1944,
		},

		// SKU: TWNDR8SJXKMK7B2Q
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4960,
		},

		// SKU: U5RPSEN4YPA96NJ3
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2480,
		},

		// SKU: UKJ6GQTAJZ8UVFYS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     116,
		},

		// SKU: UMNPWBP59RYES2AX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: UPQ5YGP3MNW8MVE7
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5600,
		},

		// SKU: UX3AKDNS3ZFJV7VE
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     984,
		},

		// SKU: V87XMK3R2ARUP54E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     29,
		},

		// SKU: VA82325BXSQ2VQVR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4224,
		},

		// SKU: VMMBZ5RPYGZJZYWQ
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3296,
		},

		// SKU: VNPUVQJT49TF74P2
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3828,
		},

		// SKU: W5Y3922HXEAKTRBH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8016,
		},

		// SKU: XC9FSDGK7ZS8H2NX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     264,
		},

		// SKU: XCUPED9MH5JZRQ3X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6336,
		},

		// SKU: Y3DNPF557GG5GT2A
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2672,
		},

		// SKU: YJU8ZZQCQJ7BPDKR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     668,
		},

		// SKU: YP2QZCVJMGX2URNX
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2214,
		},

		// SKU: ZCSJ9AQWVUHSFHM8
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7440,
		},
	},

	"ap-northeast-1": {

		// SKU: 226JGRK7TWUDF9N7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     274,
		},

		// SKU: 2DCU7R2BA29KKVRK
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2196,
		},

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

		// SKU: 2NK92W5SRKRY46GS
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     33552,
		},

		// SKU: 2P26J68VPWZXCSFK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5376,
		},

		// SKU: 2XF9NDPWBAEXYY6S
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     38688,
		},

		// SKU: 3CJSUV6SJ9TG2J2F
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     976,
		},

		// SKU: 3EKSG6FSZT5B4H8B
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     348,
		},

		// SKU: 3KYNGSJGJJ4XEFVU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     248,
		},

		// SKU: 47H2WAYY6QSTD7YK
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2336,
		},

		// SKU: 47UYAYQZ76QHKKFW
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5856,
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

		// SKU: 4KGZZ449TM545WB4
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     696,
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

		// SKU: 4TBNX74EV3WAWKY3
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5136,
		},

		// SKU: 4YZ8VXNMD5EAE434
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     272,
		},

		// SKU: 5BM3HMGCJV2Q57EE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     137,
		},

		// SKU: 5RFFBCKXAV4J29B7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: 5YHRKH4DFNQ4XWHZ
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1464,
		},

		// SKU: 63Z72DKBXFXRW6XE
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: 6986XC33S6FFMJGG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2976,
		},

		// SKU: 6A63VD4RDBFRY4JK
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19344,
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

		// SKU: 6KTQUBH8Y7R2NVBW
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3504,
		},

		// SKU: 6M27QQ6HYCNA5KGA
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       96,
			Deprecated: true,
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

		// SKU: 7622ZVSJPAVXJEF9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     49,
		},

		// SKU: 77K4UJJUNGQ6UXXN
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       898,
			Deprecated: true,
		},

		// SKU: 77PTRYZ5MAUP8HU6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     54,
		},

		// SKU: 78GXN9487KDND7SY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     896,
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

		// SKU: 7JRQ6MWJRT9N43CJ
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1596,
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

		// SKU: 7MJR4RD25PP93ENY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     27,
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

		// SKU: 7NNR38NZG5YND5C9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: 7QD4AKXMXBV3VMJE
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: 7XD3Z36Z2W84NUBV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     24,
		},

		// SKU: 898WEUWPW6W5Y8FC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     448,
		},

		// SKU: 8D4AAVFV2HFGTB38
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
		},

		// SKU: 8GZM9DWX38GD3B5B
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7008,
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

		// SKU: 8SQ27U5EUJTFR36M
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2448,
		},

		// SKU: 8VN3HX7E6Z8JVZ78
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     217,
		},

		// SKU: 8YT9TXPXJ6KCKS3Z
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     584,
		},

		// SKU: 9DU2PHQVSTD7CDFV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     195,
		},

		// SKU: 9KMZWGZVTXKAQXNM
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       798,
			Deprecated: true,
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
			Cost:     15,
		},

		// SKU: 9UBMZYZ6SXZ5JQGV
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: 9UKJDNWJQK8DP8RU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: A25H95TXV3CQAUH3
		// Instance family: GPU instance
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "p3dn.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     42783,
		},

		// SKU: AG6SXPB68M2AKYAV
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     122,
		},

		// SKU: AK9XGJTUW9TWT8FN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
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
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     183,
		},

		// SKU: AUVGPGSZCPAEKV6D
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
		},

		// SKU: AY6XZ64M22QQJCXE
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       193,
			Deprecated: true,
		},

		// SKU: B3DV2MA5V9RSKQJ6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4384,
		},

		// SKU: B5TE4RHXJ6427WXR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: BBFY9H74SU6YSW3K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: BC64WZG5DDVZDB2S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     224,
		},

		// SKU: BCXQ4G6R7EF5PMVF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1096,
		},

		// SKU: BDT5TR37G2FCDKV2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     108,
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
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1596,
			Deprecated: true,
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

		// SKU: CC8QYVPQE8UT5HKP
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5448,
		},

		// SKU: CQ2ZH47AFH9JFZAG
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4672,
		},

		// SKU: CR3WZ76QUCUUVDB3
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4392,
		},

		// SKU: CTK39QJHQN4CZ3PC
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       3592,
			Deprecated: true,
		},

		// SKU: CX79CXQ739SJJJ6P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     435,
		},

		// SKU: DAFNCUF5NU3EK9D3
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     454,
		},

		// SKU: DAPC5MD4ZQN9K67N
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: DC7PGTZ5B7Q3B7RB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: DDX2JPPMM28BXD7D
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3192,
			Deprecated: true,
		},

		// SKU: DZA8S5644YPDVE4W
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: E3J2T7B8EQDFXWDR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2043,
			Deprecated: true,
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
		// Storage: 1 x 1920 SSD
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
			Cost:     60,
		},

		// SKU: EP8EJMA4GKSUCMU6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     214,
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

		// SKU: ESYFZEM6DQKDVAH7
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     908,
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

		// SKU: F68Z2JGPZDKQNUMB
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4176,
		},

		// SKU: F7HVMYD3FDRYMJMR
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
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
			Cost:     7,
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
			Cost:     486,
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

		// SKU: FCC4C43QD9KUHD2X
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     428,
		},

		// SKU: FE9CPXTKGGW59Q7V
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1209,
		},

		// SKU: FPXP8QM9DMXHP6QP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1926,
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

		// SKU: G7RXGXQ8RGVJT46D
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     1088,
		},

		// SKU: GEXKG5YDUZY2TCW5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3288,
		},

		// SKU: GJHUHQSUU37VCQ5A
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       399,
			Deprecated: true,
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

		// SKU: H25ASVQCWDVAJNEE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     391,
		},

		// SKU: HTNXMK8Z5YHMU737
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       255,
			Deprecated: true,
		},

		// SKU: J85A5X44TT267EH8
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       385,
			Deprecated: true,
		},

		// SKU: JDHFUYQU2RVEVNEX
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1362,
		},

		// SKU: JEHEAPY2C39JZAUV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3584,
		},

		// SKU: JSZVDCWYVY2DD7Z4
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5568,
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

		// SKU: K3XHSPE6PG7KJ2K4
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     12768,
		},

		// SKU: KA565JRTVNZB5VF2
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4836,
		},

		// SKU: KB5V9BJ77S8AV7TK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     107,
		},

		// SKU: KFG4ACJGYMJG4V7D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1792,
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

		// SKU: KNVQZWZRBTHCFMS5
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     16776,
		},

		// SKU: KWUMHT4YYUHYMCEV
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7008,
		},

		// SKU: KZKSU555GJ4DVX5D
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     174,
		},

		// SKU: MJ7YVW9J2WD856AC
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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
		// Storage: 8 x 1900 NVMe SSD
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

		// SKU: N44KGZVAD599XUCH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3968,
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

		// SKU: NHAYVGAZ85Z9E6KC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4896,
		},

		// SKU: NRPXFBPFDSHUN7HQ
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1168,
		},

		// SKU: NSHNGDUGQU539WAK
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6384,
		},

		// SKU: P6PU35RPD48NS8MP
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1040,
		},

		// SKU: PCB5ARVZ6TNS7A96
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       770,
			Deprecated: true,
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

		// SKU: PDTW7GWSW6M62NWP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     548,
		},

		// SKU: PH6VW23DSQQJ6VRZ
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
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
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       128,
			Deprecated: true,
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
			Cost:     121,
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

		// SKU: QURQWTBF5KRTP7X4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6576,
		},

		// SKU: QWMUD5R6CJC3QGWB
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2784,
		},

		// SKU: R49K2Y7KZ6527C35
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2418,
		},

		// SKU: R6XFHJMC2AM65YGJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: RCJ9VNKFJCUCGU3W
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     12336,
		},

		// SKU: REQJXNHA97H376TZ
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     292,
		},

		// SKU: RJ57F96V2H3GEG5E
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5136,
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

		// SKU: S4EWKNDHYM7FSPG6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     856,
		},

		// SKU: S4XVQ4EACMWBEFY7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2568,
		},

		// SKU: SA9UW2TC8EGBE7NW
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     732,
		},

		// SKU: SD97GUGBCUND24YK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     992,
		},

		// SKU: SFUYMZQAV538QWXK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3852,
		},

		// SKU: SHF9TA3MCU6W2BRA
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: STGJMS57G86TDVWH
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: T78ECX7JX4BC9B7Z
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: T7CGRZ4XENPHVK6D
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
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
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     24672,
		},

		// SKU: TPZKPCAQBPBS7CF8
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: TQS4MEBGXTVVVC8S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     97,
		},

		// SKU: TZG97WFA265PFBMW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: U8JHTS3NQT8NSMZP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1984,
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

		// SKU: U9EUE7H4E7G5TZN2
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5448,
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

		// SKU: UPMYPSR8RCJA4QDE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: UQXX4TKDWFXJZ2FR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
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

		// SKU: USE46NRHP4S7UT6J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     124,
		},

		// SKU: V8HAYDMNCFGNSSP3
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: VKMKTRZRKPWPW5Z2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2192,
		},

		// SKU: VTKKFBR2Z5YZ5U2E
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     244,
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

		// SKU: WEMS88BYNHHUKWC8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     496,
		},

		// SKU: WHR37BGS9EYEPVKT
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     4194,
		},

		// SKU: XBTA6RG7KN7YMKKP
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9672,
		},

		// SKU: XJ88E6MSR3AYHFXA
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1020,
			Deprecated: true,
		},

		// SKU: XJREGD2AQD38AEY7
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1392,
		},

		// SKU: XM3G7K4ZKZDB2A55
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     112,
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

		// SKU: Y6EKUV29QTSRMM7Y
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     146,
		},

		// SKU: Y74PTFH3JBHER34P
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2724,
		},

		// SKU: Y75R84BY6B35VEBN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: YCYU3NQCQRYQ2TU7
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
		},

		// SKU: YEYCEKV9KYPX2ZNE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2688,
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
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       511,
			Deprecated: true,
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
			Cost:     30,
		},

		// SKU: ZAEMVYU798AFMXPQ
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     227,
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
			Cost:     243,
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
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     11720,
		},

		// SKU: 27A25X656PT4MTQ9
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     880,
		},

		// SKU: 28JJ72ZSYPYE3N9W
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
		},

		// SKU: 2FN5PXDGXPZDCV43
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     236,
		},

		// SKU: 36VUSS585AQARPSY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: 38WTDXMUU954VY9X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     46,
		},

		// SKU: 3PJ9XNY2REM5NDKX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     384,
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

		// SKU: 3YJ6GD93UKMUV4BD
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     122,
		},

		// SKU: 4EBZEVA3NHM678H3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
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

		// SKU: 54QMTYDC28MUXSDK
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: 56QS73F6Q39QZR5P
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     139,
		},

		// SKU: 57X3YE7SYW2VD2R5
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1384,
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

		// SKU: 5QQNGUP7WWQBCJFQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
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

		// SKU: 5U4PTNB2JS3SSWPQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     93,
		},

		// SKU: 63EV7GRAYQT3HN8X
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1465,
		},

		// SKU: 63XZGHB96R37WYRD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
		},

		// SKU: 65JJWWKAHFAMNF85
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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

		// SKU: 6Q3NTPW2WSVHKCT3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     416,
		},

		// SKU: 6TYE4QER4XX5TSC5
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3192,
			Deprecated: true,
		},

		// SKU: 6U3CMEPEAZEVZSV8
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
		},

		// SKU: 6YKYHPH3Z2YWUVDG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     187,
		},

		// SKU: 77ZAVWG86353YQN7
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     110,
		},

		// SKU: 79VMAH8PEB66P7JP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2196,
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
			Cost:     57,
		},

		// SKU: 822X28N5YHCZG5KZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     104,
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

		// SKU: 8UXUVRFUWYET2CA4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1728,
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

		// SKU: 9XQVTYSNZQSY76YZ
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     692,
		},

		// SKU: 9YYHGGF4CP9CTGXD
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1112,
		},

		// SKU: ANX6D5S3HNEAH8DR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2832,
		},

		// SKU: B63NDMZ5NURV4FVB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     208,
		},

		// SKU: B7967H75UA84RVZM
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     220,
		},

		// SKU: B937WNFP7YNUQP4Z
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1064,
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
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       798,
			Deprecated: true,
		},

		// SKU: BZTF9TN68JF5PASX
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5856,
		},

		// SKU: C2X6J7RSNDMJVBF6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: C692EUTFHN2R6EY9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     374,
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
			Cost:     115,
		},

		// SKU: CHKZYN2BAENBVK5B
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4152,
		},

		// SKU: DC8GUQW3AXDJG9DY
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     4234,
		},

		// SKU: DEHCSWHPKRSAJY9U
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     472,
		},

		// SKU: DGS83RFX673VMN2U
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3336,
		},

		// SKU: DTDC37NKQ3MP5TFY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5664,
		},

		// SKU: DVMT2ZCCA8FM2MBA
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     346,
		},

		// SKU: DWPQBNYZN83SFBME
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9672,
		},

		// SKU: EEDJDVNAQ6TW8UBY
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5536,
		},

		// SKU: EFB98K3UPBPJGD6V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     118,
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
			Cost:     230,
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

		// SKU: ET2S2M8W3WU3AZ35
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     38688,
		},

		// SKU: F6QTZ5UY6TKCG9RV
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2224,
		},

		// SKU: FHDTQ8QU54YT2QPJ
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4836,
		},

		// SKU: FHTM46GSD28Q62A8
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2840,
		},

		// SKU: FZ9RCRW2E6VC7KZZ
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1596,
		},

		// SKU: G5CAZXC4M5ENHEZN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     52,
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

		// SKU: G9E9SCVERZ56Y85X
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
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
			Cost:     460,
		},

		// SKU: GF99HNDMXXY2RBJG
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: GRP3Z94C2H8CABVP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: H27H9ZVPDQRDYVZZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     244,
		},

		// SKU: H29SS3FM6BHC9SX4
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     556,
		},

		// SKU: H7ZUWUQM6CR4EC83
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
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

		// SKU: JGHY7JDPX64CRY3F
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
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

		// SKU: KDAX3D9XVMGQ7UP4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: KF2B96YA25ZRC292
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
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
			Cost:     7,
		},

		// SKU: KMSM72XT6WM9JWMG
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     440,
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
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       399,
			Deprecated: true,
		},

		// SKU: M6YZNSPV6P3F2GE9
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1420,
		},

		// SKU: M7RR6MZS9CNW5VRM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4392,
		},

		// SKU: MXWFGTRWX4ZZUQTS
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6672,
		},

		// SKU: MXYJRCMDAHMFNUAB
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1596,
			Deprecated: true,
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

		// SKU: NBPZFPF467GMKYF4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
		},

		// SKU: NFDYVGXQ4AG6V6A4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1888,
		},

		// SKU: NYVHMS44MY8N2F3M
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: P5Y9XBZ954K8WR5N
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19344,
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

		// SKU: PBPW6S2SPRV65NN5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
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
			Cost:     28,
		},

		// SKU: PTA78XFTYGERGH5B
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     12768,
		},

		// SKU: PZHVQ3KFPA3RHA5V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     26,
		},

		// SKU: QKHYTHTQKCKA6S8C
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3960,
		},

		// SKU: QU2T3JN63B6TWTXC
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: QUDR94VPERQVEQ9N
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     173,
		},

		// SKU: QWC4JPSHYEGW8MZR
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1464,
		},

		// SKU: R4HV2GA2S7MK25HK
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4448,
		},

		// SKU: RC4WEGPKU9M6BMP7
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: RETZSEHZ3GRF22AR
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6384,
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

		// SKU: RZG8ZGBGC38SDDXZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: S56DV5SGXVR7Q5KR
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1980,
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

		// SKU: SWC362HA642CV2EF
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     278,
		},

		// SKU: T3AP5AHC37U9QW47
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3776,
		},

		// SKU: T5ZECHGMNCH9CDGT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: TCAZS653W7TG29VW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5680,
		},

		// SKU: TEKAB27S96DBYQ84
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     944,
		},

		// SKU: TRZY7P9CKPQ9DJMF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: TT2CTV84H9J9HFUG
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6672,
		},

		// SKU: U2U66ATN65MVW7WU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5664,
		},

		// SKU: UT6J98VNVRC29V28
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     976,
		},

		// SKU: V9DDTRJVN878EW6R
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: VRF92SGE5N2VZZTQ
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     183,
		},

		// SKU: WC53THR2WCPKTNYN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
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

		// SKU: WQSTMZQC9ZCWCUHC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: WX3XY9X8XPY6PGU7
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2418,
		},

		// SKU: WYC7KCRDW9Q989XA
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2768,
		},

		// SKU: WZ4ZY3T8YN7DQQHW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     934,
		},

		// SKU: X4D5VUURJC7BR95Z
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     23440,
		},

		// SKU: X9H6ZAWKUTYUQU6U
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1209,
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

		// SKU: XYCPAC93YTMRJWNC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     16936,
		},

		// SKU: YBHKZXGK9NJSBYYR
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: YBWGSBF6QZ6AJWXB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     192,
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
			Cost:     14,
		},

		// SKU: Z9JAE8K6RJTMMZ7P
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     732,
		},

		// SKU: ZFWKPCV46VEUXBN6
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     33872,
		},
	},

	"ap-northeast-3": {

		// SKU: 2MT4KMQGNAQ2ECT5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     428,
		},

		// SKU: 37BJVQ2GYX5GZRAA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: 3F6RMJJR5Y8542Y6
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7008,
		},

		// SKU: 3JH79JETZW3SZ646
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

		// SKU: 3MDGR6AUEBJF625D
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

		// SKU: 3N5DNH2RJCWJ6G52
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     243,
		},

		// SKU: 3QHHWCP98NFGM9QS
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     976,
		},

		// SKU: 43HMUV8XJBY9GDTW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     435,
		},

		// SKU: 4J4GQWFRT54XKMT3
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

		// SKU: 4XF55CTWGTYUW2JZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     496,
		},

		// SKU: 56Y3PZPN5SCVD87V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     486,
		},

		// SKU: 5V8T4ATYDNEP73Q6
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2196,
		},

		// SKU: 5X7N3CPPMWGY4ZKE
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

		// SKU: 6THTSXVXHJMVUVVU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     217,
		},

		// SKU: 6XS8R6XPM9UM9VEX
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     732,
		},

		// SKU: 77JTNMPN7SMR66HA
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: 86BV4R6XHH6WVA9H
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

		// SKU: 8DSBANTPFJCEX88E
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: 8GDDN3YFSGGUSVVH
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
		},

		// SKU: 8RR8GA4CBBM44627
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3852,
		},

		// SKU: 8ZGUM62MDPWU4UCS
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4392,
		},

		// SKU: 97DPZHB55D838CV7
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     183,
		},

		// SKU: 9NQDMT6QSXM6RUFQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     27,
		},

		// SKU: 9S93E9MJQH99EHSX
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

		// SKU: 9WWG68SRA86PZRWC
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     696,
		},

		// SKU: 9ZPAJZ5TEB6UQKW4
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

		// SKU: AJZETM8B93K725ZP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
		},

		// SKU: AZ4YSJAWCT4G74FC
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     122,
		},

		// SKU: BDX2FU63VZRJNJGZ
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

		// SKU: BHH47PJVPTKS72H7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     124,
		},

		// SKU: BWQCYKSGZ4WY4K55
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

		// SKU: BYBPERHJMCUKDW7P
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     366,
		},

		// SKU: CMZ2FKQ8UT38NCD7
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

		// SKU: CQQZCKSY6YEMQA6P
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2784,
		},

		// SKU: CV4EFQUCSKER9ZC3
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     292,
		},

		// SKU: D2PJG4M9UD6XZ8Q2
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

		// SKU: DRFNMGDTUUF9Y9B2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: EFDPA78Q26MQNSKN
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     174,
		},

		// SKU: F63Q22UW42PX64MY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1926,
		},

		// SKU: FCA353JMRAWV2D2U
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7008,
		},

		// SKU: GAUJR648FMC2767V
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

		// SKU: GTK2SHCX585XAUCP
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     244,
		},

		// SKU: GYSU5TZH84UBGJV4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: H35BJZ8JMU5369GD
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1392,
		},

		// SKU: H3CH2UTTCS6THX9U
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2336,
		},

		// SKU: J9BGRGA32GTUMHKX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
		},

		// SKU: JHPXF7XPYWDT9ZD6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: JQR784R9HETS4J3V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: JVFEG4BU2Q4224FC
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: KH6EKWXWMXHXFHWK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: KVVE2YP3H4HGWNWZ
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

		// SKU: N3MTF5YPWS4YQHH6
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

		// SKU: N6NQ63YATHEBJWXA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     54,
		},

		// SKU: NGXUMZD4QPGS8TF7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: NJFNC2A4V7WY4K2T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3968,
		},

		// SKU: NUVUKYC4HA8XUVCS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
		},

		// SKU: P8YNS4YBWBVENDKQ
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

		// SKU: P9Z688S2DKQ5ENA6
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: PDT63EEN4XS464DW
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
		},

		// SKU: PW35PQKFRPDNCUX3
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4176,
		},

		// SKU: QC3AC8CNHR8WKG7Z
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

		// SKU: QU4HNGDSGMEKVAKQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     856,
		},

		// SKU: R6VVFMMD4MV3Q2Y3
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

		// SKU: RN5K3FNSQ8TH6TN6
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3504,
		},

		// SKU: RQKNWM83VTNJ9SMG
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

		// SKU: S3Q9RVEV46VUMN3B
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4672,
		},

		// SKU: SAQ5WDBQRM4ME9Q7
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

		// SKU: SU3V844ZHVHSUFQU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     121,
		},

		// SKU: SY9U7UMW78NWEWMP
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5856,
		},

		// SKU: T445G2UX2QG87RST
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

		// SKU: T7D6M7G9V7AAJMK5
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

		// SKU: TAKXC2NZ5TW5GRBN
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

		// SKU: TJBT7XAUCCPW7ZYP
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

		// SKU: TUAGAHU88Q3FHNAK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: U2VYJDXAY72DRR4S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: U8EE86RS32AH7XSF
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

		// SKU: USYPGCHK2D2QSUZ9
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     348,
		},

		// SKU: V4PJKK6UX2DYAFDJ
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1464,
		},

		// SKU: VC3VNU9R8ARRKP75
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     107,
		},

		// SKU: VUR7M8GNX774K7AY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1984,
		},

		// SKU: WNAWTSQ9RH9FNQ3K
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     584,
		},

		// SKU: X5399VFYB36JG2DW
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

		// SKU: XFXZJMN9NM9GPPQA
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     214,
		},

		// SKU: XQP3NNH9U6FC6ZWV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     992,
		},

		// SKU: XTY8CSV6FY5X89FY
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     146,
		},

		// SKU: Y4KR2VA35UYB87C5
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1168,
		},

		// SKU: YDS383P9FK99EMJH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2976,
		},

		// SKU: Z6ZDTX743JR7SUTE
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5568,
		},

		// SKU: ZF7SNU5YCFP8858R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     248,
		},
	},

	"ap-south-1": {

		// SKU: 2BEU7PZFSVSE9N75
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     520,
		},

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

		// SKU: 2HGJV2DUZ5QVQ2ER
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     358,
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
			Cost:     137,
		},

		// SKU: 2P3BAZEBUTS4SPUF
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
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
			Cost:     1600,
		},

		// SKU: 2UE6Z9BZHY8AH6EJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     170,
		},

		// SKU: 34UG3C3MVQ5UQ7RG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2040,
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
			Cost:     396,
		},

		// SKU: 37JEC4YNC3Z2W58G
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1208,
		},

		// SKU: 3KJ7KW3B9XAPPZ28
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2080,
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
			Cost:     105,
		},

		// SKU: 3Q99MKY6XA8EAWG6
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: 3YK5MBDCSCBMTCRG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: 4NK2QE2HS7XC4TZC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     44,
		},

		// SKU: 4PFJ8B5MY48MH8WY
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2416,
		},

		// SKU: 54PVA9BWBBRBTH9J
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4832,
		},

		// SKU: 5N383TJKMC5FSCKD
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3032,
			Deprecated: true,
		},

		// SKU: 5SQQAD8ERYYJQ6XY
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     122,
		},

		// SKU: 5UCWNC579P38S46Q
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     604,
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
			Cost:     6,
		},

		// SKU: 67PRXCGVN9RS2KHP
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1952,
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
			Cost:     274,
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
			Cost:     24,
		},

		// SKU: 7DVR2WJR9AGPMGNC
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5856,
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
			Cost:     1096,
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

		// SKU: 7J2EWBWKVVEKU3KS
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1782,
		},

		// SKU: 7XWYFMXA4D3WAWAY
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     396,
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
			Cost:     400,
		},

		// SKU: 8U4NEK2635VB7NHD
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       758,
			Deprecated: true,
		},

		// SKU: 8UHPPU397H2XXBRW
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     302,
		},

		// SKU: 9NX7BP9ZGC9GB8AX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
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
			Cost:     210,
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

		// SKU: B7MCVXR9PZEKVA6F
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     976,
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
			Cost:     4384,
		},

		// SKU: BQ6P3ZSPRN3PC56P
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     13744,
		},

		// SKU: CA3Y8U6BUYR54NVM
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1516,
			Deprecated: true,
		},

		// SKU: CB3SYAC56AHD82RF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     808,
		},

		// SKU: CFVB4D37G6887A44
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1944,
		},

		// SKU: D3KF5EAWRYNBCMNK
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6881,
		},

		// SKU: D6QHUQFHHE2F9AQV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: DBNB3VVCG7U8C2EC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: DBTDQ322TDQF5FCK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4160,
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
			Cost:     3360,
		},

		// SKU: DJWUQKJU7UVNQ69E
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3120,
		},

		// SKU: DQV98WGJFVN4UE2B
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
		},

		// SKU: DT9PBDRR8P65PY37
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     792,
		},

		// SKU: FJX2SVQ2PFDKB8Z4
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
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
			Cost:     2100,
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

		// SKU: G4EM2YEU83PCCBNV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     864,
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

		// SKU: GBNKVV5KCQECH7K6
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3904,
		},

		// SKU: GFK5RJVCZ5KAB3QU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3888,
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
			Cost:     99,
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
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13762,
		},

		// SKU: H3WAK899KRTW8GMG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2424,
		},

		// SKU: HVWA3YJFM6AXUUWR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     404,
		},

		// SKU: J3PMHX739UXQX76F
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     198,
		},

		// SKU: J6VHFD3WEGZX8JB4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3232,
		},

		// SKU: JQ58GPWWMQHQ9T4G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: JSM24DP8S4SKF55M
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1530,
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
			Cost:     12,
		},

		// SKU: JZFVJKVWN9CGZSZ3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     260,
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
			Cost:     548,
		},

		// SKU: K4R9HJYY337SERZQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6240,
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
			Cost:     100,
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
			Cost:     198,
		},

		// SKU: KQFUY4Q3G88TJJBJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     340,
		},

		// SKU: KSSSSJNVRE2X7JTX
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: M39J5J75JWH39NR5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: MBD9GZ9JM5QGHC6U
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1718,
		},

		// SKU: MKNAAVQMBXXNTPQQ
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       190,
			Deprecated: true,
		},

		// SKU: MPZ5UQVCW27S9RRB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
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

		// SKU: P6TB45QDGY6M3GGP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: P7ZHPA67XB3YVUQ3
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3624,
		},

		// SKU: PXU5JKS73GKS2JFU
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: Q3DWV2ZGENHRB735
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
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
			Cost:     49,
		},

		// SKU: QV2HMETS44HPETDJ
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5664,
		},

		// SKU: QXBASR9TVZ5QTQQN
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     151,
		},

		// SKU: QYQXSB5RHTECU8TP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     22,
		},

		// SKU: R2TWDBE6U46GMMXV
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2928,
		},

		// SKU: RGBZATFGTXCZCT9F
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6240,
		},

		// SKU: RNHEM8HXMV3HT4RN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: RUWAU5N3UGGM4SSN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     89,
		},

		// SKU: RZAKYAJATKRN94UJ
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2832,
		},

		// SKU: S59XGYRXG7QQEACF
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     244,
		},

		// SKU: SWCNQVNDKSEMKH3G
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     680,
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
			Cost:     800,
		},

		// SKU: TGVXPJ6EU52VAAN9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1616,
		},

		// SKU: TP75XD4KFXUPPSU5
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3564,
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
			Cost:     200,
		},

		// SKU: V6Z7PAWDZYSHXEQF
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     99,
		},

		// SKU: VBD2QYRW5UYMAGSN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: VFMWKB8Q4FGPRREH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1040,
		},

		// SKU: WFUARWWHZWBKCUEF
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     177,
		},

		// SKU: WSEUB4NX5HQYMN3K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     179,
		},

		// SKU: WZT4DQPYBWSDYU5Y
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     130,
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
			Cost:     2192,
		},

		// SKU: XMPC9229VJ8N9R4N
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
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
			Cost:     840,
		},

		// SKU: YYHWKNKS82USRK67
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5856,
		},

		// SKU: ZABBU8B9S3TCCTV3
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: ZEEU583UYCZMVJZV
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       379,
			Deprecated: true,
		},

		// SKU: ZHUPGV53EVQF5VC7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     101,
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
			Cost:     420,
		},
	},

	"ap-southeast-1": {

		// SKU: 28CRHH72DZ5RGNR8
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7632,
		},

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

		// SKU: 2JDPKD8ESY4SCMDK
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
		},

		// SKU: 2KGKRTTNGH4U3GRZ
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     348,
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
			Cost:     467,
		},

		// SKU: 2UAE2HHDU4T3S5QW
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     141,
		},

		// SKU: 2VC9VZF99YQ757M2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
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

		// SKU: 3E7YWB4B2EEBEDN7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3528,
		},

		// SKU: 3VAXBWQHJMX8APVD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     272,
		},

		// SKU: 3ZNQXWH5NAXEF2JK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     211,
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

		// SKU: 42YFQ7MTPJE3JM7D
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     224,
		},

		// SKU: 44442Y2FWKN4DQSR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: 4KYFCEJHF3SRDZWJ
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     564,
		},

		// SKU: 4MQMMCS4PZDGBW9E
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2232,
		},

		// SKU: 4NT7SHGE5SCVPC26
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     16936,
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

		// SKU: 525DEHPU5R4UDSA5
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1209,
		},

		// SKU: 53MCK9279HGTEYVB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     496,
		},

		// SKU: 57M4AZ4NRYTPT6NM
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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

		// SKU: 5RUD2RE9D7MCM7AY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1728,
		},

		// SKU: 5S8CWFNMNH323ENK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 5Y9778VBKBUHDKCC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     4234,
		},

		// SKU: 65F7UR3ZG47KP2ED
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2016,
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

		// SKU: 6KSFVBWBRASE8WW7
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3816,
		},

		// SKU: 6NGRRR9CKP457WZB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
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

		// SKU: 6RUYYZGJBW84VYS7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: 6YZ8PP5Q4B442EER
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4704,
		},

		// SKU: 6ZUNPKRUPSCUHNBS
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     318,
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
			Cost:     29,
		},

		// SKU: 7R54VKGMJMT98R8P
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     271,
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

		// SKU: 7TXVW7HBDFYHJCM4
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6768,
		},

		// SKU: 7XD9NKMZWDGNKUXF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2880,
		},

		// SKU: 8465CTC77J3XUXNF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6528,
		},

		// SKU: 876HZ3N29HJQQH9H
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5984,
		},

		// SKU: 876WBBVHD565Y6SK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: 89YU6ZRTZAK6T895
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4704,
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

		// SKU: 8KFKUQJC696Y34GG
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: 8P2QTQHN4GNEZMKH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: 8SP8PNNGN4WE5TVA
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     696,
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
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       392,
			Deprecated: true,
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
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       132,
			Deprecated: true,
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

		// SKU: AHYQ7YVN7JGWVMMG
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3096,
		},

		// SKU: ANYZEEC33ZKVVTYB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     196,
		},

		// SKU: AY2FD2QT29S4U2HG
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     38688,
		},

		// SKU: B27DZDZQZHQJM2C9
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1032,
		},

		// SKU: B452CUHBSXFCCEDD
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     258,
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

		// SKU: BH8KAMC9DSEBGJYW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5184,
		},

		// SKU: BJWSEGCHDKR6J3A8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: BTVTF85NP8GUVYUN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: C5V63GGB5M5KJBP8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: CFWCMJ4Z92W6JTBS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     98,
		},

		// SKU: CQURNK8H5QX7SZF2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2352,
		},

		// SKU: CS29CWW793C96ASS
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2418,
		},

		// SKU: D4BGCVW8R2WQ8RMH
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     542,
		},

		// SKU: DD25TB3PR4DVW2KT
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
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
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       4000,
			Deprecated: true,
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

		// SKU: DPBPEBA2XABEHZYP
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1356,
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

		// SKU: DY4B9YDDBE3ZPMZE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1764,
		},

		// SKU: EAPEX34FD6ZRXPUQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3456,
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

		// SKU: EK63GNM42Q7FG3PM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1088,
		},

		// SKU: EV2HH2XUX6RZEAW3
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
		},

		// SKU: EY62CYQXS87M3BAE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: F3Y3ARSDT4BZCJ6M
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2784,
		},

		// SKU: FGWTH6S7T24BW447
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1392,
		},

		// SKU: FNWABAQZP6SKB2RX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: G35Q6WG5NVHJXBFW
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1626,
		},

		// SKU: G6QSCET9HEJXJXTQ
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
		},

		// SKU: G8R6BV5ZAJWSP2AN
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
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
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       265,
			Deprecated: true,
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

		// SKU: GD5QXY3FPWXKZXPG
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: GFAB6AXC8V6VZW73
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     124,
		},

		// SKU: GGBXFZA8CHDNUNQE
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: HAJ26YATNUGFM7KQ
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1272,
		},

		// SKU: HRS3QGPAPTSW736Z
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     108,
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
			Cost:     58,
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
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     13744,
		},

		// SKU: JDH4WM7E92WUS9JS
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3192,
			Deprecated: true,
		},

		// SKU: JHZ3DBAFAFZ85KBQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     422,
		},

		// SKU: JJDG4MJRSMGCREPE
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4512,
		},

		// SKU: JYPQYEUGN9YKTEDU
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19344,
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
			Cost:     233,
		},

		// SKU: KCZD349CGXR5DRQ3
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1596,
			Deprecated: true,
		},

		// SKU: KNAQDT88PD3HZU46
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: KUS5SQY2F9PHU9VP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
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

		// SKU: KW3WTFU96R5HCCA4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     432,
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

		// SKU: M5J5HN4R2CEAFPNQ
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     129,
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

		// SKU: M6D89MRWKYMRN2ZT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
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

		// SKU: M92PSZFXV89R5T3Q
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4176,
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

		// SKU: MPAENH8EXQYU7NF4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     377,
		},

		// SKU: MTHGDCWPSPX2839U
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: N4WBTSKJQ5JH9UAA
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     13008,
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

		// SKU: N9GA6DUUMJ35Q3X4
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3252,
		},

		// SKU: NKCRNUWEGHZ2UA64
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
		},

		// SKU: NYDC3MQ4HXYCHRE7
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     187,
		},

		// SKU: P3QSXW8JJY9U8QGF
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4836,
		},

		// SKU: P6BPTANASQKJ8FJX
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       399,
			Deprecated: true,
		},

		// SKU: PEUWNPTX7G8GK8B4
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6768,
		},

		// SKU: PFR4QPKAXZGSYVUJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
		},

		// SKU: PJ8AKRU5VVMS9DFN
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       529,
			Deprecated: true,
		},

		// SKU: PRBDPBQFXP8HG252
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     992,
		},

		// SKU: Q63G9V5TSAJWNMXD
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9672,
		},

		// SKU: Q89KQGYP9P6NEQYJ
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     112,
		},

		// SKU: QAU2XSRX79DBAR9W
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     636,
		},

		// SKU: QAZ63UXRUYRSDT6R
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
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
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     374,
		},

		// SKU: QQ2XVEUCUCJTQB4Z
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: QSDQ6KJH7YR3M6U3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: R3G7NYKYUB88W5F9
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5568,
		},

		// SKU: R66CTTZ9CJZA5FD7
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     282,
		},

		// SKU: R8K75VHRAADAJJ2W
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       784,
			Deprecated: true,
		},

		// SKU: RF2KD9FHT6329R4R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1920,
		},

		// SKU: RMNNWWGVXH29KSQP
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1084,
		},

		// SKU: RS2EX27AXRDTNVUJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     248,
		},

		// SKU: RU5RWKGWG6KS4QPY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: RYJP6BRQYNPZW8HF
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     174,
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
			Cost:     14,
		},

		// SKU: S7DD4BKZ7MH8AXQP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2176,
		},

		// SKU: SD9XDMQX6ZX5F92E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     960,
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
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       798,
			Deprecated: true,
		},

		// SKU: SQXHUU6SDCXUG56W
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6192,
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

		// SKU: T5K2U8XRZQRKQW72
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: TCNC2G9SYYFMHZBA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: TP2WJCU2NQ47P3S2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3840,
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
			CpuPower: instances.CpuPower(20608),
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

		// SKU: UDG465B8QFCG56QG
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2256,
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
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1058,
			Deprecated: true,
		},

		// SKU: USWP83ASNCY4BSQF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     52,
		},

		// SKU: UW49SPD2EJQFXU5E
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3384,
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
			Cost:     116,
		},

		// SKU: V46HANJZZ89MCB47
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: VCCKNYURW3SX8D6J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     480,
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

		// SKU: VHEFGEXDM5TBQZQR
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     516,
		},

		// SKU: VHKZ7H27WR745AZ4
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     159,
		},

		// SKU: VKXHWF4ZU9QPYD4Q
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     105,
		},

		// SKU: VVKTWPMARM4HESXU
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       1000,
			Deprecated: true,
		},

		// SKU: VZZ9GJS4VK57MMFZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: W4ZBEK9WGDVY8G9T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2592,
		},

		// SKU: W9PERBS3HNCYG24V
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
		},

		// SKU: WQ4MYXEZEW7RZBPZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4352,
		},

		// SKU: WWNCDS8M4BV5TZ4W
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     748,
		},

		// SKU: X3EAKWTRZDZCJXFB
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     896,
		},

		// SKU: X83BCZCJGFKW8FSP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3264,
		},

		// SKU: XJCZNGBHQ4URRV67
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: XK3UK323TNH56729
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     392,
		},

		// SKU: XQGCM75UPCBZ54XK
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     33872,
		},

		// SKU: XQNKA8ZJYU83DHY9
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: XRPWTR8AKRN468DK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     26,
		},

		// SKU: XSXMGMXPBTAVPRF4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: XUVJRQ9MSAQKDXE9
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2117,
			Deprecated: true,
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

		// SKU: Y7WGCXGA4G2JJPB2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     240,
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

		// SKU: YGF7PPM3FN32WAG5
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6504,
		},

		// SKU: YSKUUH777M98DWE4
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       196,
			Deprecated: true,
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

		// SKU: Z9DRMYS5T8482JSY
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1128,
		},

		// SKU: ZAGTVD3ADUUPS6QV
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       98,
			Deprecated: true,
		},

		// SKU: ZB3PCMUQE9XQZAHW
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9671,
		},

		// SKU: ZTT5TF9DC7ZE6TE8
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     448,
		},

		// SKU: ZXNBDQWJFA2WNX68
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     784,
		},
	},

	"ap-southeast-2": {

		// SKU: 25WHS436YREMJA46
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     302,
		},

		// SKU: 28BD34XSRKXRZSVM
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1209,
		},

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

		// SKU: 2DNQCWK8FFDWGGH9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2592,
		},

		// SKU: 2DYRKD83PP6HQ2GB
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2784,
		},

		// SKU: 2NQXKXA35MAMBEQG
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
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

		// SKU: 2TSD7N786Q8DF647
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     260,
		},

		// SKU: 2Y6SXNQ9NCEBURDE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     151,
		},

		// SKU: 2YQUKAJYK5F5GQ85
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2992,
		},

		// SKU: 2Z7FWHYVTXPYSZ4E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: 2ZSKW5N6X86FEKAW
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     222,
		},

		// SKU: 3N5UHMTZJDKQH393
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1136,
		},

		// SKU: 3QZGK647FGPYPPUV
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2268,
		},

		// SKU: 3T77YF8NAR7YXSRN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     1128,
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

		// SKU: 47M5629FN4A52YX2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     108,
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

		// SKU: 52D6FTUVTRVBB6HJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     422,
		},

		// SKU: 55WHWE5CGRCKDNSG
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
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

		// SKU: 5MEGP2X8VTY5ZH56
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     142,
		},

		// SKU: 5P3P6VYJBP8GSSKT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2664,
		},

		// SKU: 5ZTHF34UHKZZBAYK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     52,
		},

		// SKU: 66QVG55FP52WHCFH
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       372,
			Deprecated: true,
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

		// SKU: 6GDB9RFA3CDJNK8R
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     568,
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

		// SKU: 6SMHQV4N7A8GXAJ7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     95,
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
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       93,
			Deprecated: true,
		},

		// SKU: 6XR6GZ28BWW6G6SP
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1392,
		},

		// SKU: 75WF9ZQV7BNR5AJU
		// Instance family: Compute optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "c5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: 78Z5UDBK335DDYN5
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
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
			Cost:     14,
		},

		// SKU: 7PKKMJZCTR3TDAD3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
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

		// SKU: 8BZR6NS5ZFKBCZJF
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1356,
		},

		// SKU: 8R28NHFBSUJGP83N
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: 8SQTV987382BW7Z6
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: 8XZUT4AHDH972AME
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       132,
			Deprecated: true,
		},

		// SKU: 94HVTDPPQ66D595G
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2418,
		},

		// SKU: 95ZAVWQ887ZARP8J
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3624,
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

		// SKU: 9Q9T3T3WUBMWRJ92
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3840,
		},

		// SKU: AAKEAYX76A4MBETP
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     159,
		},

		// SKU: AATPJ7WMWX748RZQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     24672,
		},

		// SKU: AE24MAMUSTJQ2URX
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6240,
		},

		// SKU: AFJQX6QPDP36VF6M
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     444,
		},

		// SKU: AJU54N3RVM6BVQF8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     211,
		},

		// SKU: ANJPSG7F85N5YKVV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1728,
		},

		// SKU: AY2KMM6DUX7JU6MZ
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     636,
		},

		// SKU: AZNDB478X32F9E4B
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     696,
		},

		// SKU: B2FBFCHDEKT4SYR4
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: B5WGD7UWSCEUU7AW
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6816,
		},

		// SKU: B79B744JMHV7GBB3
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     282,
		},

		// SKU: B7WDU84CV8BAGNYW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     105,
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

		// SKU: BF58Q74F9EA6BKUW
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6816,
		},

		// SKU: BNAJNTU2SES6X2QN
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1154,
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

		// SKU: CVRJGAUET9Y34BUC
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
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
			Cost:     233,
		},

		// SKU: CXJGKV8H28A9Q2NF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     960,
		},

		// SKU: CZQ8MWDZD87KDWJR
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     174,
		},

		// SKU: CZQZN7XNWXXXNUU8
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: D29U26UAEX6WK4TW
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       745,
			Deprecated: true,
		},

		// SKU: D3QWTSM7Y7Z4PGTE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2880,
		},

		// SKU: DBMRRDDSPZZKNV49
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1596,
			Deprecated: true,
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

		// SKU: DKYEEH5NM2FYNG6T
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6528,
		},

		// SKU: DMYTTVRMHPPWYUDT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: DS7EYGXHAG6T6NTV
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       898,
			Deprecated: true,
		},

		// SKU: DVM64ENMEF8TJ2VS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     190,
		},

		// SKU: E3J2WNEP4FB5RBG4
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     1008,
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
			Cost:     29,
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

		// SKU: EFM32KF4GBTW6B7G
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: EKK7DV37DAFKWW4S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5184,
		},

		// SKU: ESFPMDVJSKF58U7N
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     16936,
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

		// SKU: FC9Q2AUNCDSENPFV
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     130,
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
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2117,
			Deprecated: true,
		},

		// SKU: GKVR3QEC5B7WJXTD
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3192,
			Deprecated: true,
		},

		// SKU: GUXTVKBD6AX5DSH3
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3120,
		},

		// SKU: H293A95RN6FSQQHF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: HDQRRSX4C27JV825
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1088,
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

		// SKU: HF9BRY6XU2QEDNDH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4832,
		},

		// SKU: HHJGN8MDU3U6DFE5
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       186,
			Deprecated: true,
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

		// SKU: HPWR3U55RMT4GVUR
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3816,
		},

		// SKU: HVYC2XFAV9M6MDPG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2176,
		},

		// SKU: HXWVK8ZBMBMGMVKN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     13,
		},

		// SKU: JBKJW4PSHEMYA7C3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: JEPUKXN8RHX39MCF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: JFGMBKVFBU4Y8YBN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     111,
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
			Cost:     116,
		},

		// SKU: JVSCVVPTPVNY339J
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     141,
		},

		// SKU: JX7BX97ESDC2GZS7
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     284,
		},

		// SKU: JZ4PJDAGSSZ4ZT7Y
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1272,
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
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       399,
			Deprecated: true,
		},

		// SKU: KQPGF5ZKTX8DCS5J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: KR3T5CU24HAW4UXN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5328,
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
			Cost:     7,
		},

		// SKU: MDZFGAZC635JNRAU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1920,
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
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1058,
			Deprecated: true,
		},

		// SKU: N3D6SQF6HU9ENSPR
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
		},

		// SKU: NBEUETP9AB5ZGD4H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4352,
		},

		// SKU: NQ6AYDZDEMTV9N62
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2272,
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

		// SKU: NXGGFHBZBMWKZB9H
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     888,
		},

		// SKU: P6V9GX45QEN62C49
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1542,
		},

		// SKU: P83EA2X65KAYJEVW
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1208,
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

		// SKU: PF8VTK9HY6HY2KTA
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: PNAHW7UFYWRFWU3P
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3996,
		},

		// SKU: PXJ54NJV375NKRN7
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7632,
		},

		// SKU: PXX8KQE5UVBAN2D2
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5568,
		},

		// SKU: PZ5MY9JF8UD95F8E
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     12336,
		},

		// SKU: Q2JZP35TJBRDR3JQ
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     374,
		},

		// SKU: Q8TA88PMTA549MU4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: QQMKZKZ56JCDJW88
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8352,
		},

		// SKU: QZ3QJ95MESM7EP8U
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1496,
		},

		// SKU: R38YK2SZTEYTTPWC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: R7QMC6E4E9FB48YN
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
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
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       529,
			Deprecated: true,
		},

		// SKU: R9Y82NHWSGC28GT5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5076,
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
			Cost:     467,
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

		// SKU: RZP36AQYWS2URSTU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     272,
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
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     748,
		},

		// SKU: SFTKTW2Z6CSFAUFC
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4544,
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

		// SKU: SPY78VMVH6S2FZ2N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: SRQ5DPTUBCZSDNMU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: SYWG6GXUBYK28WV2
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     520,
		},

		// SKU: SZ9M85SK7YBNNHXZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: SZWBDHQGK3ZS8NPP
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     4234,
		},

		// SKU: T2K3UDU2WT8UPBEW
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1998,
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

		// SKU: TAYCGKYB9FWNKXKY
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4836,
		},

		// SKU: TD8NW4BSBCYU646U
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       3592,
			Deprecated: true,
		},

		// SKU: TN5W9YFXZ57M7U7C
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9672,
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

		// SKU: TUR8ZDXX5EMYBJY5
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: TVGEGY3EG8GJGPJR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
		},

		// SKU: TWX5CSJBECGZJDB5
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: U4CRVE4RVE3SWDJ9
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     604,
		},

		// SKU: U8XY9MMPZWZ97MMU
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: UAM5NM7HEYS3TZJG
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     33872,
		},

		// SKU: UHQ6GNPKHCQPGXF6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5328,
		},

		// SKU: URA544WHNFXSJCVQ
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: V4J96TRCSRB8BYEG
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
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

		// SKU: W7KEP5KVMZP745H6
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     19344,
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

		// SKU: WN3KQ6EZ89QHREY5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3264,
		},

		// SKU: WNYWP7QUJ3MU8NVV
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       798,
			Deprecated: true,
		},

		// SKU: WXXGUA9Z8QQ9YV8U
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     318,
		},

		// SKU: XA2MCGDMCTRCKV88
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2416,
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

		// SKU: XKABB3HJQ5ACE35K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: XPAKDV3PWHYTJU3X
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       265,
			Deprecated: true,
		},

		// SKU: Y4CU2M7AJN8S5SN2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     564,
		},

		// SKU: Y6XVZPTCKV2AR48W
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4176,
		},

		// SKU: YNZBC6A26TXAHW57
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
		},

		// SKU: YQE7DJQRQE23CYMV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2538,
		},

		// SKU: YSAJN7PPFTUEXSEN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     380,
		},

		// SKU: YSY45DV78JKWDSNK
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3408,
		},

		// SKU: YTSUFB5KUMQURKW5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     240,
		},

		// SKU: ZE58X652GSZ94NSV
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     38688,
		},

		// SKU: ZJF5Q3KWKS8VD3X2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     26,
		},

		// SKU: ZNBTWVCZENJHA3AQ
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     348,
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
			Cost:     58,
		},

		// SKU: ZQ494U58MQ5TKJPG
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1040,
		},

		// SKU: ZXDRY2N3ZT6RHHV5
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4536,
		},
	},

	"ca-central-1": {

		// SKU: 25M2GQEC2CNYRHXS
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: 28WUAUX5F6Q393QV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     371,
		},

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

		// SKU: 2MVKFHYBJ42BGZXH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     20,
		},

		// SKU: 2N2VAKSPNXXYKRC3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: 2YJU5BSGCEAHJCZ2
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5664,
		},

		// SKU: 2YVMV6UYW3CGPD73
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     214,
		},

		// SKU: 39MUFZY7JF5GPZR4
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2528,
		},

		// SKU: 3BU8MS96U7N734KU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2568,
		},

		// SKU: 3JF2BTY89RVU289W
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     158,
		},

		// SKU: 3Y9CCZC5968YYQP8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5136,
		},

		// SKU: 44D42B7CJ8THK4CE
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3792,
		},

		// SKU: 44HDD89HH2F8UQJE
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: 48PNHGCUGSKHKXWB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 4EGW7JZGU48G2WXG
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     460,
		},

		// SKU: 4U2R7DFUY8MXDRXA
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1152,
		},

		// SKU: 5HX9D8RU7VK893RY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6624,
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

		// SKU: 63SJAJMZ59X4NN5R
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: 6JRY4PWQBH89Q5YG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: 6JU9CURJPYX4YX7B
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: 6RXASBV82M8GPCBX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     107,
		},

		// SKU: 6X6PA8UQJ2Y6WZM5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     185,
		},

		// SKU: 7JPFX6752USFVZ6U
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2208,
		},

		// SKU: 7KHAHXUUYVDA7TQQ
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: 7WNDUCM8KSTC4SFZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: 7YD2CTAF78EVVBJS
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: 834UAB5UH8W4SF24
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     288,
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
			Cost:     25,
		},

		// SKU: 85AER3VSQF8QRMWU
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     576,
		},

		// SKU: 8PM7EH2WEN8X2976
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4416,
		},

		// SKU: 8PRZXJU3R6EPKUZU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: 9DQPGXK34RW7PX78
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1674,
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

		// SKU: 9HFHXV75EMCQJ84H
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4032,
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
			Cost:     102,
		},

		// SKU: 9RTXNCHNG9RP4R7Z
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     552,
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

		// SKU: ANNX3Q5FME26QR5G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1536,
		},

		// SKU: ASXQBSPHNBMAVDDX
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
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
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: BHC68DMHGZCNAN5Q
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1264,
		},

		// SKU: BP4BF2ZUCZS3ZA4B
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     26928,
		},

		// SKU: BS9CY7RD3PEZMQNN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     93,
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

		// SKU: C8DR5PUGDMGZFZBW
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: CJBZAX43TFCY7MP4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     124,
		},

		// SKU: CY3PQ6KJVSHSHNCG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     138,
		},

		// SKU: E6MFFHZCFP25RCBQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     428,
		},

		// SKU: EFPGXFAYRF6KFFDU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: EFZ2NGQZ9PYWX896
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     7336,
		},

		// SKU: EH25VTUWKDSWQSPA
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7584,
		},

		// SKU: EMSCJ83J3PNQ3CU6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3348,
		},

		// SKU: ERT7PBVETRX65YQZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     41,
		},

		// SKU: EZRVPN55TYQAHMF7
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     13464,
		},

		// SKU: FSGQVDV47SQ3YTFM
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: FXKKWT3SEQQBM8C9
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     2976,
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

		// SKU: G9DSFWQ6WFAMSUCY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: GD526B6CQMZ2C9DB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     92,
		},

		// SKU: GEP3A83C8EE66M85
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: GJKARJ27US2HSDGT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     856,
		},

		// SKU: GXAPN94K3KV7P7UY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     46,
		},

		// SKU: H4WZD73NF9ATN4Y3
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: H576RJHNX4PSDBZM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     248,
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

		// SKU: J2Q4QDVB97B5S589
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5056,
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
			Cost:     204,
		},

		// SKU: KEBGQP5CPGMSJQCH
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     186,
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

		// SKU: M2YD4VWZJP8TV78M
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     992,
		},

		// SKU: MBENAB2KVWCPAE53
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     14672,
		},

		// SKU: MFA9YNFG9223YWB4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     276,
		},

		// SKU: MMVE56V3NSJGEMTA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3366,
		},

		// SKU: MTQ9EQQNZC4JWG6K
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     316,
		},

		// SKU: MUB9BC2VEUDHRH2X
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: MWX2S56GW7EY8ZT7
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     424,
		},

		// SKU: MXX7BC2ET5HDY4TW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     167,
		},

		// SKU: MYTV4XP76ZXUNMCV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     83,
		},

		// SKU: N5AAYADMUWK67JAG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     372,
		},

		// SKU: NCEJ52QZ9SGHF3RV
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     496,
		},

		// SKU: NEY4NQE9TU76TZJV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
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

		// SKU: NYTEZKW7JNF97BD4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5136,
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
			Cost:     409,
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

		// SKU: PE4HA3F7XV5BC77E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: PFTCGCCAY5UQTHVJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: PK2KTRY9P5937ZV9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: Q6JJPF9W7PZCSFB7
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7584,
		},

		// SKU: R2H6SG57JHH3STUB
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: R4WRNPA3Q9MYXZMW
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3312,
		},

		// SKU: S8MGKM3D9PZZAFVZ
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: S96TQZX57H2HWA7W
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1416,
		},

		// SKU: SRUF8WV5JMVC3CXQ
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1908,
		},

		// SKU: SVKK54AVET2F2MRH
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     848,
		},

		// SKU: TBBQMY5CD4M6QCEU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     3968,
		},

		// SKU: TKGBWHF2XYHYUAMW
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3816,
		},

		// SKU: TVAY2YAM4NR4N7AM
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     230,
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

		// SKU: UG2NZPQQ4CD46NKZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1104,
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
			Cost:     12,
		},

		// SKU: UK2D2TFXP2K8CSRM
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     632,
		},

		// SKU: UU6TX3BD4NENX4WK
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     144,
		},

		// SKU: VRRTSDDY8RBH9NRN
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: VV4RK33GAPTBG3TH
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     744,
		},

		// SKU: VVJ5SUBQXX4XJ2WB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     334,
		},

		// SKU: W2YE6AAHSNBK295S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1712,
		},

		// SKU: WN92GR42V9JH57T8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6624,
		},

		// SKU: WQBTW7GTP4UUYNKG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2232,
		},

		// SKU: WUQTC9EJBEKUJYX2
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2832,
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
			Cost:     51,
		},

		// SKU: XBCMWPRUGMX72957
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3072,
		},

		// SKU: XCWRZWK76Y3URWBZ
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     106,
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

		// SKU: XNP6NENYAFFSS77T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: XSSNPJ2ENGV27H5D
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     920,
		},

		// SKU: XY78F3T44HHAE764
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5520,
		},

		// SKU: Y4MQ2FTHPYD6T6JC
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
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

		// SKU: YRUQJAW9YZ58XRQR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     1984,
		},

		// SKU: YRVP8HFQMABUKBRS
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: Z35SFRZ8SPVX2YA7
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     212,
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

		// SKU: ZVRMYVWQQR247H9A
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2760,
		},

		// SKU: ZVRWCMB89WCDUE9D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3424,
		},

		// SKU: ZW59PRKGFHKSY98Y
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     504,
		},
	},

	"eu-central-1": {

		// SKU: 25DRTGW7U2WTZE2V
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     246,
		},

		// SKU: 2FXKMSNT79U4CF55
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     10608,
		},

		// SKU: 2V2B7DCBZG35C8BT
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     888,
		},

		// SKU: 3AFDFDJ9FGMNBBUZ
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3201,
			Deprecated: true,
		},

		// SKU: 3KCGMZRVWDY4AC5R
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1600,
			Deprecated: true,
		},

		// SKU: 454A4B4XPKWZ6XYA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6576,
		},

		// SKU: 4K2RDTDA5QDSVF79
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: 4NP8EQG6RJTMJ4TE
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     450,
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

		// SKU: 5DVNJYXH349GT28W
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: 5HND2XJGAMC62A8E
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3823,
		},

		// SKU: 5JVECJSYNYZCPRCG
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     15292,
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
			Cost:     53,
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
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       315,
			Deprecated: true,
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
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       129,
			Deprecated: true,
		},

		// SKU: 6CKG7UUQBWNQVNY7
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2700,
		},

		// SKU: 6GR6HHW9M8KXFW8G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 6K9NWBMKGH2EQZC7
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6480,
		},

		// SKU: 6MKRRAUJJGR3DRSS
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5952,
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
		// Storage: 1 x 950 NVMe SSD
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
		// Storage: 4 x 1900 NVMe SSD
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

		// SKU: 774XP29ZBNQKJHC3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     21,
		},

		// SKU: 78CYHANWZQKDHDT6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1096,
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
			Cost:     428,
		},

		// SKU: 7EJH5CWEXABPY2ST
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     18674,
		},

		// SKU: 7SBDBJTKMFNNAHEF
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     444,
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

		// SKU: 7SN7STWTVJW3W2G9
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     272,
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
			Cost:     107,
		},

		// SKU: 7X63DAK78VTPCW8F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: 7XUHUVGHKKGRSWTJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},

		// SKU: 85S4P8X4W8SQNKYV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: 85ZP32Z5B2G2SYVH
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6528,
		},

		// SKU: 8A8BSV9YMVEJJ9S5
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1384,
		},

		// SKU: 8KTQAHWA58GUHDGC
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       158,
			Deprecated: true,
		},

		// SKU: 8MASRMZD7KUHQBJC
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9337,
		},

		// SKU: 8YXBW9KSGDJU89Z9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 93YQACZFNWMAHZBQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4656,
		},

		// SKU: 9PBREHF5JRKB6RBE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: 9SD2KQ3929JVCXRG
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3240,
		},

		// SKU: 9T4XT2YYFV5CB4S6
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4352,
		},

		// SKU: 9TRVX5RCR8BDDV8H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: 9UR7CWCC3D8XJM5D
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5520,
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

		// SKU: A5EA98WBYWTZESQK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2214,
		},

		// SKU: A65VEHYMUBAYJ5QH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2760,
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
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1488,
		},

		// SKU: ANQX7ARPTX828ZSU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
		},

		// SKU: AQY3B8YE4GEUZJB8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4428,
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

		// SKU: BCVY6CZXE89MKZJ8
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     9336,
		},

		// SKU: BG59UMPBQZ8AT89X
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5952,
		},

		// SKU: BJUKPA3DJHPEA7MY
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1620,
		},

		// SKU: BMXENRM9BM54QBEV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5520,
		},

		// SKU: BUQGC5KFEYR2N468
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     194,
		},

		// SKU: BXS2CDUKEC83Q4SH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     230,
		},

		// SKU: C2EDZ5DQN8NMN54X
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       772,
			Deprecated: true,
		},

		// SKU: C8DM7N2QZBAXV2Q9
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     938,
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

		// SKU: CJ5XAG2NXEKBDUUU
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1350,
		},

		// SKU: CJCX42QAZ7YB57C8
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     111,
		},

		// SKU: CNP4PV4Y2J8YZVAR
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       632,
			Deprecated: true,
		},

		// SKU: CPW9N2NGARABH27P
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1746,
		},

		// SKU: CRC8WK79UUN2KF95
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     416,
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
			Cost:     26,
		},

		// SKU: CVRPW534R69RUEMP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
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

		// SKU: D943FQ9E2XKM3NYF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     137,
		},

		// SKU: DFSJNU995AF3SRVM
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1080,
		},

		// SKU: E623EB9XMXG9WG3E
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     540,
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

		// SKU: EPGY4ZGG5ND9GZ6N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2192,
		},

		// SKU: ER456JE239VN5TQY
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       400,
			Deprecated: true,
		},

		// SKU: EUTBMNK4ZKWCEG2H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
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

		// SKU: F8CGM38936KU7KGW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     345,
		},

		// SKU: FAEB6MVJ4PHXAEQX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2328,
		},

		// SKU: FCDR7GHDJDC2G8Q9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     208,
		},

		// SKU: FCTDXPA9FM2E8Y98
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     43,
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

		// SKU: FJGAN929UK7ZM2ZP
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: FVQQQM6YZMWR2CH8
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3264,
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

		// SKU: FZKQVXQSUXXYJWH3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: G8HBU7V7HQK9HFP3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     832,
		},

		// SKU: GDZZPNEEZXAN7X9J
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       79,
			Deprecated: true,
		},

		// SKU: GTSBUY8C6M4SNS9W
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4668,
		},

		// SKU: H6CVBUZJF8MK9XFB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4384,
		},

		// SKU: HGVVQV8MM6YAH4CP
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2334,
		},

		// SKU: HJDQXCZYW2H7N9CY
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     186,
		},

		// SKU: HKGRPXXREBX8H57R
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4152,
		},

		// SKU: HW3SH7C5H3K3MV7E
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       258,
			Deprecated: true,
		},

		// SKU: J2N3CMEQ8BNTQ43Y
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2768,
		},

		// SKU: JG83GAMRHT9DJ8TH
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
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

		// SKU: JSXG89ERGHPGMPFM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: JT8NB7GDBZ7JVSQB
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
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

		// SKU: JX2XB22MQDXYAG7N
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     270,
		},

		// SKU: JZB9AADMBJGQFQMV
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     12960,
		},

		// SKU: KY65WK89RNASCT6R
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1326,
		},

		// SKU: KYFX85FCPCCT57BD
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2064,
			Deprecated: true,
		},

		// SKU: KZ25CYAW7ZZ6SN5U
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: KZVETY6QAHKHG6PX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     984,
		},

		// SKU: MB3JDB58W76ZHFT8
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       516,
			Deprecated: true,
		},

		// SKU: MTQWAHX8C4T4FYVW
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       800,
			Deprecated: true,
		},

		// SKU: MVF6FXYW55EYQ9WG
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     692,
		},

		// SKU: MXZJRDDEUB3D7TE9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     776,
		},

		// SKU: MYC92D68328BCK4R
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4656,
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

		// SKU: N5D4RSFRNZ3SRTJ3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     460,
		},

		// SKU: N7WSYHVUT72KMK3V
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1032,
			Deprecated: true,
		},

		// SKU: NHT5KUS9X5VYNND2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
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

		// SKU: PNCTSZUGECNP3EBA
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     388,
		},

		// SKU: PVWZJDKBDCE2K7EE
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     225,
		},

		// SKU: Q3TGY4VS8MEP9KQF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: Q5D9K2QEBW7SS9YP
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       3088,
			Deprecated: true,
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
			Cost:     214,
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

		// SKU: QEDN2UB5WGCJJ9S8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     492,
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

		// SKU: QR7F3KDAWU8AH7GS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     920,
		},

		// SKU: QWGWYQQNQ33DVTVP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: R428A7B8SA3DSPSS
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: RP7JF6K2NBW9Y4SJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     48,
		},

		// SKU: RQHKQUTACYUYF2EF
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     222,
		},

		// SKU: RR3EBSXWXMCMVMKE
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     30584,
		},

		// SKU: S3BME23KN52QCQ5Q
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     97,
		},

		// SKU: S6UCCNYK82GR2ZVS
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1998,
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

		// SKU: SQUQGX4CJ4DHD594
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     18672,
		},

		// SKU: SR83ERWWZNTNC99E
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5400,
		},

		// SKU: SRT8CVD43PBJZMDH
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3996,
		},

		// SKU: SZ7QZBGVF3PE4M9H
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3492,
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
			Cost:     13,
		},

		// SKU: TAUXPEERSXJGS2Q5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: TKCZSXDAVA4AM6YD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1840,
		},

		// SKU: TNZV3ZSQXGTXM5PY
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
		},

		// SKU: U3HN8F279AJ8S6ZJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     123,
		},

		// SKU: UBSAA4SE7N86SRHF
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     744,
		},

		// SKU: UFBQWTBTF5HAR33Z
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2176,
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

		// SKU: UT7UV5UXYU6926T7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1664,
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

		// SKU: VKC9JFWDJCMTC9PM
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6528,
		},

		// SKU: VKNQMDAYCY4KAX7G
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     37344,
		},

		// SKU: VM9W9M3QTW37FET2
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     346,
		},

		// SKU: VMMJKPPKWN3S5QEH
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1167,
		},

		// SKU: VZ6J7FB3Q77KJBWQ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     21216,
		},

		// SKU: W2TKRCZGWBBYBYMF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     86,
		},

		// SKU: W8BJAXFA9Y653RXQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3288,
		},

		// SKU: WA58EKSMRQWP343Y
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3328,
		},

		// SKU: WADKRMX4T7XH7GYV
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: WVY4KGHQEHERBSCH
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1088,
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

		// SKU: XGAATXHUHNWXTMMR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     24,
		},

		// SKU: XJ5CFKTMG2X7XGD7
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5536,
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

		// SKU: XZKSDYJYNCVGDHBP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3680,
		},

		// SKU: YAV6GV2KYXGEX8FD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     548,
		},

		// SKU: YTPGRBVGFDWTXHNY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     274,
		},

		// SKU: YYWPX2ZCJC7S7XYZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: Z9EDDV2UW43WDYQR
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5400,
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

		// SKU: ZDM2TMP7EZT88VE8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     104,
		},

		// SKU: ZZKZTMBM47YRJJUJ
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     173,
		},
	},

	"eu-north-1": {

		// SKU: 2538YKHJ9U725SUR
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3744,
		},

		// SKU: 2GBMQNJMRMBS2D6R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     816,
		},

		// SKU: 2XXEX3CBT4N4NX7D
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4288,
		},

		// SKU: 37UT54G6JJJMAJRK
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     416,
		},

		// SKU: 3D47FGMU9DAZKRNU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2144,
		},

		// SKU: 3DRQQB52Z8249X9P
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     536,
		},

		// SKU: 3WTVR3S4XJNM2NU9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     364,
		},

		// SKU: 5CB6VVGDRJ9PHTJ4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1638,
		},

		// SKU: 5PDWZQZTJYFCGMXW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: 693PZGA8CDU6HZB7
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1396,
		},

		// SKU: 69R8D37WWB4F5SJR
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     698,
		},

		// SKU: 82XACGR3AGP6WEFP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     728,
		},

		// SKU: 8496CH6MG95ZAQYM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1632,
		},

		// SKU: 8DAKCX6RME893UJM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1072,
		},

		// SKU: 8HSTPE5CSSQ4WX2S
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     326,
		},

		// SKU: AJ8X4KYD8E224WDU
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: AJVNQNC9EUABHKVY
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: AN7RUVM6BV9RAMPJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: AR9MNQT4HKS3PW4E
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     608,
		},

		// SKU: B64VVG3GJHTFCHKE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     91,
		},

		// SKU: C7TQWCDCTARH6NGN
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5216,
		},

		// SKU: CJ9QBK5HYHCP5VC4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4368,
		},

		// SKU: D2JAKC8S78GX4NQZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4896,
		},

		// SKU: D4S28RWJTR77VRDY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6432,
		},

		// SKU: DU75X98EKG473DU4
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     152,
		},

		// SKU: E53HP5JHPGJMUY8X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     102,
		},

		// SKU: EA9V3NER3QJB5RUN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     182,
		},

		// SKU: EH9AEFRDR9572ZBF
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1920,
		},

		// SKU: F2JRKWU67V83ZHJX
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2880,
		},

		// SKU: FU4ZXU9PYSPNWDT4
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     104,
		},

		// SKU: FUB7Q54ZGEMZPF29
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     652,
		},

		// SKU: GEQB5MT5TAU7ZKRD
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     163,
		},

		// SKU: HJX8B7X79YW7M9GR
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
		},

		// SKU: HXE7YA49JNCX8S4S
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3216,
		},

		// SKU: J8BCGA9A49MPKUH5
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     5584,
		},

		// SKU: JAXBJAFKYBDY45F6
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2184,
		},

		// SKU: JDD52ZGEJHPD5JCV
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: JXFQ7BVYUBTV69MP
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     832,
		},

		// SKU: K43BZKPFNHGPERT3
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     960,
		},

		// SKU: M3V4NZPBXRVGQJWR
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     208,
		},

		// SKU: M9QWF3NM7QSQF3BT
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2608,
		},

		// SKU: N3NW5VVSG4XUCSWJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4896,
		},

		// SKU: N7WVP857CT67ZHU2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3264,
		},

		// SKU: PE2EVG6NJS26KQ8U
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     240,
		},

		// SKU: PW7XJ6YW4CQUFJN6
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     304,
		},

		// SKU: Q5A4KTBWGPUCHDHQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3276,
		},

		// SKU: QEY8DDNPR8XRS5ER
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5760,
		},

		// SKU: QF634R69HRJG476W
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     345,
		},

		// SKU: QNSJ72WBYUA54ZJN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4368,
		},

		// SKU: R9UBC8HCET9VFFAT
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1216,
		},

		// SKU: RV646K3D8B5MK634
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2792,
		},

		// SKU: RZ2CPJT3JCVWXCQM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     21,
		},

		// SKU: TERUUJAVNMYFPEGJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: TMZR28G2V6VN7HFB
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7296,
		},

		// SKU: TUUH48TQEUX7VF86
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2432,
		},

		// SKU: TW2GS8E2BK32NE25
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1872,
		},

		// SKU: UWJK7K6ZVM3RJ37J
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     134,
		},

		// SKU: UXXY6P8RA5R545T7
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3648,
		},

		// SKU: V9AFYEMMPNB2B49R
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1304,
		},

		// SKU: VF3DJC7YF75KBYAP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     86,
		},

		// SKU: WGTNH42XRABN9CQH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: XCVN4HZD9XZ33XW8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: XR534VJFQW2WFZAJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     43,
		},

		// SKU: XZ6X3TZ2HUVATTJA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2448,
		},

		// SKU: Y5HV5A3AMSS53ZFH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     268,
		},

		// SKU: YFNKY3SFSUP3M8SJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6432,
		},

		// SKU: Z4C93AUSJ2CWM67K
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3840,
		},

		// SKU: ZW23CV5TV8HJU4CT
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4864,
		},
	},

	"eu-west-1": {

		// SKU: 249WVV2ASQDC3RY2
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: 26VHYCXC57HGZ7ET
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     416,
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
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16006,
		},

		// SKU: 2RYFJZP3BA2RP4E8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2256,
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

		// SKU: 2YQ29UHG9JW3TH39
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     796,
		},

		// SKU: 2ZP4J8GPBP6QFK3Y
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 2ZZGWZ549UJWZAZ4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: 34T56M5BM3P8ATUD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: 38KKRTQP385PX9HY
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7776,
		},

		// SKU: 39VH8W4ANB3RS4BD
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: 3BYWJJNA4YTXHSBD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: 3PRUDQTWGES5EM2Y
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     81,
		},

		// SKU: 43FB7QU6KEK6U6NE
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1815,
		},

		// SKU: 43GJQJWC2SEHCVCG
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3840,
		},

		// SKU: 47NTBKB4KMUU98P8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     45,
		},

		// SKU: 48B3N7VQAWGK5TPT
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4032,
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
			Cost:     100,
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

		// SKU: 4RMQK234M4QU3SVE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     508,
		},

		// SKU: 5A9DKQFHKVD7PWG7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: 5B5SB64HHGZZCSK8
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7680,
		},

		// SKU: 5H7EQNYD42C62HV6
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6000,
		},

		// SKU: 5X96PB535HQA3AUC
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1500,
		},

		// SKU: 66H5Z24TT3M24WUY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1536,
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

		// SKU: 6KCEGJ35VRAHEDVX
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: 6NVB6WVCM8M6K25A
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6768,
		},

		// SKU: 74523BKNYBJFM37J
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     12000,
		},

		// SKU: 7EQQVZ9BYNDH98NS
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     436,
		},

		// SKU: 7TAAUA4VXEWMVYQ4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     163,
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

		// SKU: 89MR4JHXKG6X6V4Y
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     127,
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
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       956,
			Deprecated: true,
		},

		// SKU: 98PB6H94FD83QG5D
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     109,
		},

		// SKU: 9CNYPE9EEBF9K5YP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: 9EZPWFFQ44AXAJFU
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     832,
		},

		// SKU: 9FTHQAJJUUWK3TPQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 9HEJP2JW69U24CFK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1016,
		},

		// SKU: 9M9VGZ9WC5KK5QMV
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1962,
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

		// SKU: B5YN5C6UFVHWGRD4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2196,
		},

		// SKU: BCJ2S5SSVF9MB9K6
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: BD4WKXDZ8JRNMYN7
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: BD8XV3M49PYTY484
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: BDB8Z35FE8QWP8FY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3424,
		},

		// SKU: BFXJQH664EFNQXA2
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2560,
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

		// SKU: BHUE6JWPUTCCSA9H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2032,
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
			Cost:     403,
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

		// SKU: BUZBADMYHNETJAMA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6096,
		},

		// SKU: BZ4QR6ZNEKBJMWVA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     564,
		},

		// SKU: C3M6ZGSU66GC75NF
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       185,
			Deprecated: true,
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

		// SKU: CNPTBB628ZRXNY4B
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     326,
		},

		// SKU: CP6AQ5U62SXMQV9P
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       741,
			Deprecated: true,
		},

		// SKU: CWTBWRW792K88XRM
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: D53CGUF3JPG3SJC3
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: D5VHDHY2NC8Q5A74
		// Instance family: Storage optimized
		// Storage: 8 x 2000 HDD
		{
			Name:     "h1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4152,
		},

		// SKU: D7M225NX27SHFXCT
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     244,
		},

		// SKU: D8HUQ8M6VM587DP4
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3000,
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

		// SKU: DMAXDHUCJUYVP2PR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.large",
			Arches:   arm64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     57,
		},

		// SKU: DS72FUWG3NT9KG9M
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1128,
		},

		// SKU: DXH6X4VVFUT4EUU5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
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
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2808,
			Deprecated: true,
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

		// SKU: EBNDQX7DNG2CR5MJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4512,
		},

		// SKU: EQ6NRVP4KNZ2UBGA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.xlarge",
			Arches:   arm64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     115,
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

		// SKU: ESGE59TCDBSQKCQY
		// Instance family: Storage optimized
		// Storage: 2 x 2000 HDD
		{
			Name:     "h1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1038,
		},

		// SKU: EX9QNZYDRPBGXQDZ
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     500,
		},

		// SKU: F2NGC99ZABCH62MD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: F3VADBY3Z6MMHKTQ
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8003,
		},

		// SKU: F9QY9USH3CC54ZTS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4064,
		},

		// SKU: FP7Z96TTU3VFSX2H
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     107,
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

		// SKU: G236SARNZUU7X8GS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     364,
		},

		// SKU: G47F9R3A5H3EEM43
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     250,
		},

		// SKU: G8HP89ZSMHWP7CPA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3384,
		},

		// SKU: GW5JVTDTM4UUHXUX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     22,
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
		// Storage: 8 x 1900 NVMe SSD
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

		// SKU: HNFU7R9H4NW62ECB
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     13220,
		},

		// SKU: HNKY83Z77VRXC2UH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     214,
		},

		// SKU: HNS95CZU3R6CS883
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3072,
		},

		// SKU: HPPMDRDSYEG4FFEM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: HUR96BZEUM9ZQAKA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     856,
		},

		// SKU: JGXNGK5X7WE7K3VF
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       146,
			Deprecated: true,
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
			Cost:     201,
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

		// SKU: K4G2U7FXQNQNN97R
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: K4MUP8PWYT57AXMQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5136,
		},

		// SKU: K7YHHNFGTNN2DP28
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: KB8TSZN8946JWSX9
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: KG2342MUYQD6T4BK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
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
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     972,
		},

		// SKU: MF8MCNYZWNPNASQA
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: MMNM5NNJRE45GKBE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     488,
		},

		// SKU: MY32DZDWHCRK7CHH
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     976,
		},

		// SKU: MYHPC9DHXHXE4RB8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.2xlarge",
			Arches:   arm64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     230,
		},

		// SKU: N32SVFGZJ9NSZMT2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1728,
		},

		// SKU: N45J86GJAXMBC3MG
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4392,
		},

		// SKU: N6EG4B95S6W9FERA
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8000,
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

		// SKU: NDCYBYSW6DGD7U3G
		// Instance family: Storage optimized
		// Storage: 4 x 2000 HDD
		{
			Name:     "h1.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2076,
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

		// SKU: NWX8FVW4QY5C2SMU
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1280,
		},

		// SKU: P3CTRQJY7SHQ6BJR
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       585,
			Deprecated: true,
		},

		// SKU: P75KF3MVS7BD8VRA
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       478,
			Deprecated: true,
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

		// SKU: PKQPAYBPMT2KWM3J
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5504,
		},

		// SKU: PR4SS7VH54V5XAZZ
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1482,
			Deprecated: true,
		},

		// SKU: PY52HJB9NWEKKBZK
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       2964,
			Deprecated: true,
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

		// SKU: Q84GDGQ87MZGTS74
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     14520,
		},

		// SKU: QA275J7BMXJR8X35
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: QH4BGHBAKTJY73QN
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: QHSUEH845JV2XB66
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3924,
		},

		// SKU: QRP5VBPEA34W72YQ
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       293,
			Deprecated: true,
		},

		// SKU: QSBUA42GSD5GMCW8
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: QSK7P9EBJRFX5DVU
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     32000,
		},

		// SKU: QTAGSZ32A2HQ97XB
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3305,
		},

		// SKU: QVMSWPR6TVZK26JK
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7680,
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

		// SKU: R6PJQPYQCHBBN75Q
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: RASFAC97JWEGEPYS
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       120,
			Deprecated: true,
		},

		// SKU: RFFYMUAWUFFXXZ7T
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: RJX3UJGQA87WZ3E8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6768,
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

		// SKU: RREU9BP2RNRYH6QN
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16000,
		},

		// SKU: S9P349S5694QGC3Q
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: SAHHHV5TXVX4DCTS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     141,
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

		// SKU: SG7W9Y6K3UVR8YUU
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     218,
		},

		// SKU: SGF4MSKWRV77VRXR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     91,
		},

		// SKU: SNYHQPB4TEYWWU9W
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3048,
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
			Cost:     12,
		},

		// SKU: SZGY4A8U8CBJGHRV
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     15552,
		},

		// SKU: T2V37J8R6VYVJAQ7
		// Instance family: Storage optimized
		// Storage: 1 x 2000 HDD
		{
			Name:     "h1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     519,
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

		// SKU: T7NK8WRNK34QBJY4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     182,
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
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: UD5YCYQSPBX5D4DT
		// Instance family: GPU instance
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "p3dn.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     33711,
		},

		// SKU: UK9SADRVCHHZDCVY
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: UNEZG8PVCP3RUSQG
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       73,
			Deprecated: true,
		},

		// SKU: UNN6RX97UG6NVTAD
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     320,
		},

		// SKU: UT99ZKVGEDASFX42
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     26440,
		},

		// SKU: UVWPBR69EG9YV8X5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     768,
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

		// SKU: VE5S7KJY8963VNBB
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     160,
		},

		// SKU: VM3SRW97DB2T2U8Z
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       702,
			Deprecated: true,
		},

		// SKU: VPAFYT3KA5TFAK4M
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       371,
			Deprecated: true,
		},

		// SKU: VU47C5TE8NTFM8FR
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5120,
		},

		// SKU: W5XDNKFVSJANV5WF
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4000,
		},

		// SKU: W6D9CQ4WVM7VXJ65
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     40,
		},

		// SKU: WCTP6U8PQAK422R3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5136,
		},

		// SKU: WDZRKB8HUJXEKH45
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       239,
			Deprecated: true,
		},

		// SKU: WHW892VQX5VFH5XS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1712,
		},

		// SKU: WR44KB22K2XD9343
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: WRHB6C343F4GABF6
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     208,
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

		// SKU: WU2VK73P32QS9UUX
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     640,
		},

		// SKU: WVRMX775F3KEDXSS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.4xlarge",
			Arches:   arm64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     460,
		},

		// SKU: WY3NYRVKMUZBG3W7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     282,
		},

		// SKU: XBNHPCMPN9BQEBYH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2568,
		},

		// SKU: XPB3ZU4KH47VF5ZQ
		// Instance family: FPGA Instances
		// Storage: 1 x 940 GB
		{
			Name:     "f1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3630,
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

		// SKU: Y5E26BH6KCM3SDM4
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     872,
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

		// SKU: YJSPXEVG2RNGYQJZ
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2000,
		},

		// SKU: YKY7DZK3VDHHYUAD
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     122,
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
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       1912,
			Deprecated: true,
		},

		// SKU: YWZ6Y5ZX8EZJMYVC
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: Z3S5HVHF6HYM2YZJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     428,
		},

		// SKU: Z6EVSNG2XMFWCTHX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: Z6YE5Z7JZR44M8U4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: Z8RHGFHFDUFZXY4K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.medium",
			Arches:   arm64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(322),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     28,
		},

		// SKU: ZF7HR96ZVH3JMAX9
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: ZNUTV2KZJ4TRUJXR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     20,
		},

		// SKU: ZVQBGCVZ8VHW2AED
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     254,
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

		// SKU: 2JKBUXX2V2VTC4U4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1776,
		},

		// SKU: 2M3J8ES9HAJPBAP5
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3589,
		},

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

		// SKU: 3A97PSRYB25DP7KG
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     460,
		},

		// SKU: 3G8CZBD3DNZ5FABC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     444,
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

		// SKU: 43ZYQZ9D2MSRN4CC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5328,
		},

		// SKU: 489GD5S7YM9EHXNV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
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

		// SKU: 54T482R6PYW2DSUB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     404,
		},

		// SKU: 5DFSHH53WZHB2BSZ
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     676,
		},

		// SKU: 6732YZTC4X8TFUUC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1429,
		},

		// SKU: 6BZBC9RT9Y98ZRJV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
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

		// SKU: 6GCXB3B28ERWFN27
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8112,
		},

		// SKU: 6MKWCNF69WBBDGAQ
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2704,
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

		// SKU: 785Z2MSN9BUK8YDM
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     879,
		},

		// SKU: 78PYHDUXFKC4592W
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     888,
		},

		// SKU: 7Z5W8DQKEW7UW4AK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: 7ZJW8GF4W4CQTAMV
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1448,
		},

		// SKU: 87SCHP6QRVQNDEXU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2664,
		},

		// SKU: 8UQGPJ7D3AU3BAKZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3636,
		},

		// SKU: 926DSA5WRDVQVWVR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     377,
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
			Cost:     105,
		},

		// SKU: 9577AANGZWJEJK6A
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     28712,
		},

		// SKU: 976FE2M9ZE3XPMKY
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1318,
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
		// Storage: 1 x 475 NVMe SSD
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
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5792,
		},

		// SKU: B2D6JZJWM7HPMFFP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2424,
		},

		// SKU: B4RAU86U3KAUBU4P
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2368,
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

		// SKU: BY9H6X3QND5JDVZG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: C4D4FXYNRFW5GZDC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5716,
		},

		// SKU: C9UZYBCG9RGUAFUF
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     296,
		},

		// SKU: CB5PSNMWB6CBS3MK
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6288,
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

		// SKU: CDWJY64XVYFMJ7XW
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8112,
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
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     362,
		},

		// SKU: CYJNUQDTM6AZ4XXG
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     338,
		},

		// SKU: D72HYX76XGBQN3WM
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     940,
		},

		// SKU: DCATEYMXEQG8JXGV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     42,
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

		// SKU: DK9YJEPQ2GSDJ39P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: DQUGD63TSN8HCMXW
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4192,
		},

		// SKU: E2HPMZ3W833XV8YN
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3144,
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

		// SKU: EHN95Z5ZF6GJUASA
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     169,
		},

		// SKU: EKNKBU8KQ5U9SP6X
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     101,
		},

		// SKU: F28PPHABQTXZUPNM
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5273,
		},

		// SKU: FFJEWJ57TETXVVB8
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: FG6E9NNZZXM5DKF2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     339,
		},

		// SKU: FKV5E8TYHA8WY4W5
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     439,
		},

		// SKU: GHP9N4YDQPV2EMET
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     111,
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

		// SKU: GZMA93CBMYAW2UHJ
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     920,
		},

		// SKU: H83S7MSZN3DR9X27
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1352,
		},

		// SKU: HC6BRCD65E8APNZG
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     220,
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
			Cost:     13,
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

		// SKU: HM7BK6S35SRCRUVE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     169,
		},

		// SKU: J2W3XMF2G83SCG82
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5273,
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
			Cost:     422,
		},

		// SKU: K8FAUV7VRMSM2WBT
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4140,
		},

		// SKU: KFCSVCS6P27ZD4F6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
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

		// SKU: KXJ496M7NVD69J5U
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4848,
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

		// SKU: M8WRXY5Y56F9Y8P7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: N52DZZ3AUVZQBY34
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3552,
		},

		// SKU: NK657278DYAC2VES
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     14356,
		},

		// SKU: P6XMZ2KM6FJMU793
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2070,
		},

		// SKU: P7KCGP5FFKC5WNEC
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6288,
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
			Cost:     211,
		},

		// SKU: PMTRRVTDZ2B7RB7C
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: PVEHFX4YQJZRU7KT
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4056,
		},

		// SKU: Q96JYKUK3JWWSAVE
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     262,
		},

		// SKU: QDC38EQSB9NXESVP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3552,
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

		// SKU: QES643QE9XDNFQCM
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2858,
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

		// SKU: RQH8FMT3RJSQ9ES5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     592,
		},

		// SKU: RTSJ8WXY6YDTQMDC
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     230,
		},

		// SKU: S9P33Q5WT5T2S9GS
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2636,
		},

		// SKU: T69NSFFM7AGQ327Z
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5408,
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

		// SKU: U7AMQSMEGBHPBJ75
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     131,
		},

		// SKU: UR4GKNRHYYJ9SR9G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     222,
		},

		// SKU: USFSWHMQ59UE8V4M
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     808,
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

		// SKU: V42YNWUT5TNWQUYB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5328,
		},

		// SKU: V8J24S7ZE77VTCBQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     148,
		},

		// SKU: VHD9GD2XAU3N8J7R
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: VN4E7YXFSEGMW9H7
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8403,
		},

		// SKU: VTSBWGPAJJH7K9SS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: W4VMNNNA3S97HYQH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     21,
		},

		// SKU: WP9QSXA5MSKVTVNJ
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2896,
		},

		// SKU: WQDV73KPVNMZYQVG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7104,
		},

		// SKU: X485N7MQKCPGFQZJ
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     524,
		},

		// SKU: X8GXXHTH8UAUZM2T
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1184,
		},

		// SKU: XB2ZQ8PH4QGJT85F
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1818,
		},

		// SKU: XBEB7XGYUK4BRES4
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2096,
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

		// SKU: XUYJW48V36FZH4CM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7104,
		},

		// SKU: XYT4SBQZZAG4Y6SA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4736,
		},

		// SKU: Y6DK4ETN32KZE2ZQ
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     724,
		},

		// SKU: YAYRNKTR6XU2SWXY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: ZMTU52JUTH57RXFQ
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1048,
		},
	},

	"eu-west-3": {

		// SKU: 28K3MMZM3JY27J4Z
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     528,
		},

		// SKU: 29QDFBY626457QNP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2688,
		},

		// SKU: 2GPTY4FHZ67Y99YH
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8403,
		},

		// SKU: 2SBHKHAFA355N6F2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(60),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     105,
		},

		// SKU: 2ZMKU9GKG25EJ7J9
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6176,
		},

		// SKU: 356WG7BEU79PHD63
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: 3JFQECPPCVVMUT9B
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3552,
		},

		// SKU: 3JHJWMGYSE5ZRTAN
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1056,
		},

		// SKU: 4CERZQ2UH98YKN9Z
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3168,
		},

		// SKU: 4DHYC6H42WPWSN2X
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6384,
		},

		// SKU: 4FHWXDGNB4JEHZG2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     404,
		},

		// SKU: 4YRX29MYKAUKSSAF
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4056,
		},

		// SKU: 53T2ZQT5MYSE29EM
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3088,
		},

		// SKU: 5CY42GXBRZGVE7ER
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     920,
		},

		// SKU: 5G3WEM5XVUACDEKE
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2112,
		},

		// SKU: 5GUXEVE4832H2E5Q
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7104,
		},

		// SKU: 5MXXBHFQERJ4RXWQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: 5SFED2PQECBXFWU6
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     362,
		},

		// SKU: 5T57362F6DVC6G4S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3232,
		},

		// SKU: 63DUDHAM6SVDUZWG
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     181,
		},

		// SKU: 643YA83F8AWEU6H4
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5408,
		},

		// SKU: 6K9VVX2XWFP5PGZQ
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     264,
		},

		// SKU: 6NBCXWJMBXERZZKF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: 74T694YCDPQEK5EB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     592,
		},

		// SKU: 7946EKBRWHG4VNZ6
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     132,
		},

		// SKU: 7CQNPJ3TGQ93XBT9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     224,
		},

		// SKU: 89YAB55PARJAUR5B
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     47,
		},

		// SKU: 8KMARXUHY8RWA9J9
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

		// SKU: 8KPFZFCJTHBGV94R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     21,
		},

		// SKU: 9A3XZGTF5Z3WTR7Z
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     101,
		},

		// SKU: 9B4KBBMWWXG9UERC
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2128,
		},

		// SKU: 9QVN7PQUHX4FQ7BP
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2896,
		},

		// SKU: 9XMCZ9Y6ZEAVE4T3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1616,
		},

		// SKU: BCZEGWADVTCBZKM3
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1448,
		},

		// SKU: BSHEVJCMB8VDNF4W
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     460,
		},

		// SKU: C4J4NA9ADWN3RH99
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2070,
		},

		// SKU: C6JVMHC6VEXQF3QH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     94,
		},

		// SKU: CNAEA455D5C3XG6Z
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     296,
		},

		// SKU: DBDM32KRCEPMC4FU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5376,
		},

		// SKU: E2EUV5HMZAPKGXWR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     808,
		},

		// SKU: E5FBDPC35HJQ43MD
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

		// SKU: E5WX8TZA5VT6PA2W
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     169,
		},

		// SKU: E6R9QUXJD4HP5HCS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3584,
		},

		// SKU: ERV54JNPFQBTV33T
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1184,
		},

		// SKU: F7CBG2583W59W7F3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: F7YB988783FMJZ2F
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     115,
		},

		// SKU: FE7V7C9267P6CAF2
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8112,
		},

		// SKU: GN7KN5GM7UGPDPDW
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4224,
		},

		// SKU: H5EU6HSTV5R8XJCS
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     230,
		},

		// SKU: HGSJ458D66CX25EE
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     676,
		},

		// SKU: HZD8UHQ4F5VFSE93
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

		// SKU: J2HTQFGTSCNKJ93N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4736,
		},

		// SKU: J564RX2ABW4X5NJ5
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

		// SKU: JXHQCZU6SB3P63HH
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5792,
		},

		// SKU: KFAVR5EX3WDDGA5V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     377,
		},

		// SKU: KZ56ZZ7Q5JVGG7AC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1792,
		},

		// SKU: M4AFRV7XD3KUUH6A
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     724,
		},

		// SKU: MWBSWJGR7BYMKR32
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2368,
		},

		// SKU: N4F2UQVRMBZA8AZS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     23,
		},

		// SKU: N5HR4YFG74YK355Y
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: NFB4YEKDG2PVCQPU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5376,
		},

		// SKU: NUFYC792DC7AEAVF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: NUV5NNNMMYGGXPC2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: NZGFTJ9DVYK5N2S5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3636,
		},

		// SKU: P9VU9FCJC9KBKRCD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: PF5UPKSKNQN44GQJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     148,
		},

		// SKU: PG6C8VAAYGWA69YQ
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     169,
		},

		// SKU: PNAWYC9YY3YKNPSS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     422,
		},

		// SKU: QBWBY8JHZKBB6YDT
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     338,
		},

		// SKU: QCRTUS9RFEJHGS4F
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6336,
		},

		// SKU: QEAFMQ7QXMXCH52E
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8112,
		},

		// SKU: QEMZCMJ3XVY4XHED
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     339,
		},

		// SKU: QQAPA2CTHZ35JQ6E
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     133,
		},

		// SKU: QVZ5JDVVSTTKYK7J
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

		// SKU: R2PKZ3M8BWW7CK49
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

		// SKU: R633M33UG7H7BAWW
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: RAFC5CVP2JMXFD7K
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2424,
		},

		// SKU: RK2KC3999YSC7NTG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     896,
		},

		// SKU: S25SDEHGZ6QDWT3K
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1818,
		},

		// SKU: STDGQS9F5PZRC8NY
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

		// SKU: SZM93WEEWTTQK73V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     211,
		},

		// SKU: TGCUXP7FUNC9KV8T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: TM34VPBSJ334K7AJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     448,
		},

		// SKU: TU2RXUW84MDWUR38
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2704,
		},

		// SKU: TZ239R65PXU2PJ2E
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1352,
		},

		// SKU: U3ZJ523M8224DEPA
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     404,
		},

		// SKU: UM77GN2Q395CVTGX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4848,
		},

		// SKU: UXGF9Y6KGGP8N9PA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: VGGFGDR4T85XZHS5
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4140,
		},

		// SKU: VJNW2M9RSKVQ63D5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     101,
		},

		// SKU: VYKWS533GCBBGG7A
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

		// SKU: W25KCX78HTGGGPDE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2424,
		},

		// SKU: WHENTS956M4684EP
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16806,
		},

		// SKU: X57UV9TE5NRJAKDY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4256,
		},

		// SKU: Y8PJUSAMAPA399WN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: YFK2A6Z3HYC7GR45
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

		// SKU: YQAAPU88TXUE6558
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: ZAPKZBA9C2W6KXP6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7104,
		},

		// SKU: ZHPN37KR4HH6FCTT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     42,
		},

		// SKU: ZKYT5RP4JYNGSK5T
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     808,
		},

		// SKU: ZMRAAPRJDYB4GWTJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: ZRGWVV6KFXGSNMBJ
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

		// SKU: ZSPM59QD927JMDYC
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6336,
		},

		// SKU: ZVF8ZSR5KQUSTKJB
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

		// SKU: ZVRZY433TQ68D6ZS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     112,
		},

		// SKU: ZVUZWUYSVNUM2G8K
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
	},

	"me-south-1": {

		// SKU: 358T9BNCKNDCWTCV
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     120,
		},

		// SKU: 3JE5B5X8JKKPDR4R
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1408,
		},

		// SKU: 4FM9NZ6EPJQXU3G5
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     554,
		},

		// SKU: 4G3ZZMPZWCTZSBVN
		// Instance family: Compute optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "c5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     5755,
		},

		// SKU: 4KDSUCYWKGSQ7RZH
		// Instance family: Storage optimized
		// Storage: 12 x 2000 HDD
		{
			Name:     "d2.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5376),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     3234,
		},

		// SKU: 4QPQ6C2YYEV5TG34
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4963,
		},

		// SKU: 4TZ8CQGHWWY9J6GS
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     352,
		},

		// SKU: 5WKGEF8YWH5EDFWA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     401,
		},

		// SKU: 639AZ99C8AWDUH45
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4435,
		},

		// SKU: 6Q2FRK4X5Q6J4MS8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     50,
		},

		// SKU: 6SCSGDM7DJCY9NVX
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2878,
		},

		// SKU: 77FXAFEXH7XQ3PN6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     310,
		},

		// SKU: 77RX4G62JAUNWTKD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     235,
		},

		// SKU: 7WFHTRZ6TRVM9Z5M
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     155,
		},

		// SKU: 7WWTPXUP2GN7EANE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: 7ZSXM92CDSBS7BUJ
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6653,
		},

		// SKU: 8F28X3TXGJ36PMKM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1901,
		},

		// SKU: 8URTE2F3MB78JU2K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     620,
		},

		// SKU: 9C86YP7HT9SEP6PG
		// Instance family: Storage optimized
		// Storage: 3 x 2000 HDD
		{
			Name:     "d2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1344),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     809,
		},

		// SKU: 9M8U5J9M58SU9VDJ
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     378,
		},

		// SKU: AME9E8WCUZGF7YKQ
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3326,
		},

		// SKU: B3W5E2V7FDU3ZKHJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     845,
		},

		// SKU: BDDXV64BKDE2WUZ7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2482,
		},

		// SKU: BGYF75R8MC4SVADD
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     240,
		},

		// SKU: BSEX83N32KEF9ZGX
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     704,
		},

		// SKU: BZHEW6TUDHJ4MNAH
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1514,
		},

		// SKU: CGCC77ADAEW9RK8V
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     959,
		},

		// SKU: CSHFHNJ3GP3S8J2H
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     100,
		},

		// SKU: EMXUEHBDFF7X2JPN
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8448,
		},

		// SKU: F4M9XENGKUBF6BR8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     471,
		},

		// SKU: FZE6ZUV89F7EMY2N
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8448,
		},

		// SKU: G74C7FAYYACZ3HSP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     106,
		},

		// SKU: GS7QE6YXPMQVT9H2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3766,
		},

		// SKU: HBWBRRVXCH64ATKZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3722,
		},

		// SKU: J6U9JC9BYNEWP55Z
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6653,
		},

		// SKU: JA5Y68UTDAYF9Y4D
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3027,
		},

		// SKU: JBUY2KSVKM6EHC7F
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5632,
		},

		// SKU: JKR3SENFUEGN3TRC
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     176,
		},

		// SKU: K5HCDBW4P62R5ETC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3802,
		},

		// SKU: KBZJY8RKTN3YTD64
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1883,
		},

		// SKU: KHNNKMETHZGUHBC8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: NGKP8P8WJK6BZGR3
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6054,
		},

		// SKU: NSC89X9RBBY4UB8R
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1241,
		},

		// SKU: PAECQR5DBWUR5793
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5650,
		},

		// SKU: PGR3NWZ3QU5JWNRM
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     277,
		},

		// SKU: PMMKN5P42Y2T7PJW
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     139,
		},

		// SKU: QH9RMBUDEGYRW47G
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5069,
		},

		// SKU: QKZSKCK9V9FJKHPV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     211,
		},

		// SKU: RGT3TZHQX9544YP6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     942,
		},

		// SKU: RKCAR9J4GFBBE2XP
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7445,
		},

		// SKU: SSE8NV5JG45ZTXN7
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     757,
		},

		// SKU: ST4WFBYWKQAJ6YW9
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: SZWKPZ7SN6STN35P
		// Instance family: Storage optimized
		// Storage: 24 x 2000 HDD
		{
			Name:     "d2.8xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(12096),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     6468,
		},

		// SKU: T3JQTNCY2NUEWGVZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     25,
		},

		// SKU: T7XRAJQQ5VDSVSN5
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2158,
		},

		// SKU: TUC488Y6MAJXNKKC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5069,
		},

		// SKU: UJA2UWWUMSUEGZ8W
		// Instance family: Compute optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "c5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     5755,
		},

		// SKU: UPB4DKQ9NHQC35YK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2534,
		},

		// SKU: USH7UVD8A3S6MGS5
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4316,
		},

		// SKU: UZZG8A2RMP9DU8W9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     118,
		},

		// SKU: V3R9RS98Y7RV8P8D
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1109,
		},

		// SKU: V657PWAB3TPFWJKX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     422,
		},

		// SKU: W8T37C4PJ52MQNQJ
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     189,
		},

		// SKU: WMGKGJTB9EV6JBZR
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2816,
		},

		// SKU: WYNJ6QVAVUNWXYWX
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2218,
		},

		// SKU: XM2XG6RUAEC3DB68
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5650,
		},

		// SKU: YAGURAXVZ4WRHPVA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7445,
		},

		// SKU: YEBVK489ZTEM5CVJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2825,
		},

		// SKU: YY6AYQJQSAYMAVD9
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4224,
		},

		// SKU: ZDE3NHMGC69X97YY
		// Instance family: Storage optimized
		// Storage: 6 x 2000 HDD
		{
			Name:     "d2.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2688),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1617,
		},

		// SKU: ZEVNS2QXFBNRZ93A
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     200,
		},
	},

	"sa-east-1": {

		// SKU: 2DQW6R4PKSZDG2T6
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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
			Cost:     148,
		},

		// SKU: 3KRHXWUDH2BDV4Y8
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     2288,
		},

		// SKU: 3R483THWA8HXM8WP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     7344,
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

		// SKU: 4K6KRGSBWUXMTM73
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     131,
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

		// SKU: 5HXV6Z3WNRN5C694
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2358,
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
			Cost:     9,
		},

		// SKU: 7JK3Y822S2U92G49
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       1399,
			Deprecated: true,
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
			Cost:     37,
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
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       700,
			Deprecated: true,
		},

		// SKU: ADMZJH7G4TK3XW72
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       95,
			Deprecated: true,
		},

		// SKU: AGV5N34XYJNRXKRG
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       325,
			Deprecated: true,
		},

		// SKU: B6W433SSVKEY68BH
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       761,
			Deprecated: true,
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
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       190,
			Deprecated: true,
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

		// SKU: CV9YGAP64YA5VP2R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     537,
		},

		// SKU: DE4F8GSMG9ZHARG8
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1300,
			Deprecated: true,
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

		// SKU: E87H92C56AN7QPDZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     134,
		},

		// SKU: EFW3QVTF33JCUCBP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     153,
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
		// Storage: 1 x 1920 SSD
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
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       381,
			Deprecated: true,
		},

		// SKU: FM6RT86RTAN9G67D
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
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
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     572,
		},

		// SKU: GFJESH549VTK9EBY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     306,
		},

		// SKU: GQ84YTKA8GK2HSWH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3672,
		},

		// SKU: GXN99J4XZAB2A9UN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     16,
		},

		// SKU: H8BY38DSCNH87FAD
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       163,
			Deprecated: true,
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

		// SKU: HP9H83HAT3CYS64R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4896,
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
			Cost:     18,
		},

		// SKU: JRRFY2C2V3KB63CJ
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
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

		// SKU: NVUUK68HKWKTGHGY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     33,
		},

		// SKU: NXAU2M3KSE5AEQT5
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       350,
			Deprecated: true,
		},

		// SKU: P8TYCUD23CFMB728
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     67,
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
		// Storage: 8 x 1900 NVMe SSD
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

		// SKU: QAMH5U6WYKDZZPH8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     262,
		},

		// SKU: QBPF6VCNHZCVW9FP
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1144,
		},

		// SKU: R9CEGK66VUD8J9MT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     268,
		},

		// SKU: S8SHPWRKY2EMQG6C
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     1048,
		},

		// SKU: SHPTTVUVD5P7R2FX
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2600,
			Deprecated: true,
		},

		// SKU: SUWWGGR72MSFMMCK
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       650,
			Deprecated: true,
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
			Cost:     74,
		},

		// SKU: UFS4KMTBWWWQ43VK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     612,
		},

		// SKU: UP3NUBJT76RUD53J
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     524,
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
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       2799,
			Deprecated: true,
		},

		// SKU: YFYS3RGFGR7H4YBP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1224,
		},

		// SKU: YW6RW65SRZ3Y2FP5
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       5597,
			Deprecated: true,
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
			Cost:     595,
		},

		// SKU: ZDXFABEESHGE4M5P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2448,
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
			Cost:     297,
		},

		// SKU: ZP9UZFWN4HYJUA9P
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     8,
		},

		// SKU: ZS3N9NNQK7TJB9D7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4716,
		},
	},

	"us-east-1": {

		// SKU: 22PCVUMSTSHECWJD
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: 2AYU3ARCNA2MQQ84
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2064,
		},

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

		// SKU: 2KDPHNSBHFB3KFYK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: 2N4FCB37PW649DB6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     504,
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
			Cost:     371,
		},

		// SKU: 35AEEWH98DECPC35
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     170,
		},

		// SKU: 38RWWF4EE9U48B26
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2472,
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

		// SKU: 3NY3EX7YAET2WWYZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     4,
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

		// SKU: 3UJVEWT6H9RWC6GQ
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     186,
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

		// SKU: 3ZXYJSGAQKRQJ7BG
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: 44BFC6CSKFS3KEJP
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: 44BG498Y4VQ5GM28
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.xlarge",
			Arches:   arm64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     102,
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
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       650,
			Deprecated: true,
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

		// SKU: 4EJ6YKYP3U22GDYZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1530,
		},

		// SKU: 4FTG75FPGD5Q3AJ6
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
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

		// SKU: 4NUJYJRGQNN8CX5U
		// Instance family: Storage optimized
		// Storage: 2 x 2000 HDD
		{
			Name:     "h1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     936,
		},

		// SKU: 4PMUBZD8SN3VX7GB
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: 4TCUDNKW7PMPSUT2
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       2660,
			Deprecated: true,
		},

		// SKU: 58HUPRT96M5H8VUW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: 5G4TA8Z4MUKE6MJB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
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

		// SKU: 5SEHB4QTDFZ8HQK8
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     750,
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

		// SKU: 6AUAERFUWRVM7MMK
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: 6C86BEPQVG73ZGGR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 6F96E9J622X4BZCX
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1116,
		},

		// SKU: 6HV26SM97R6EC9RB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2016,
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

		// SKU: 6U6GZ2DN4RFCJ7D9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: 6V9PSA8P35J8R29W
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: 7CX9EBNP8K95F94K
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     288,
		},

		// SKU: 7E4UP2UZB8HWCES2
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: 7PYABBCJQRSUWFPD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: 7QJ46D6EDXQS6HTH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     3616,
		},

		// SKU: 7YFC8DX6JB9UEFUF
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     26688,
		},

		// SKU: 88VG68PMGN3M4ZEY
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13344,
		},

		// SKU: 8G6KQ3BF6E7YCMJU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     18,
		},

		// SKU: 8M9ECG8SU9BVTRAX
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: 8QZCKNB62EDMDT63
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13338,
		},

		// SKU: 8RS29HNKA7WV6C78
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     384,
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

		// SKU: 93DY9HWWFRCUFEBC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: 9CDDEWMC2G4YQACQ
		// Instance family: FPGA Instances
		// Storage: 1 x 940 GB
		{
			Name:     "f1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3300,
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

		// SKU: 9HWRHSPS3H3EZSSA
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3144,
		},

		// SKU: 9Q38J5GDVZ3SQETX
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: 9RMTQNKNXZUP56E2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: 9USH93WEV7YHKN3N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: A67CJDV9B3YBP6N6
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2600,
			Deprecated: true,
		},

		// SKU: A83EBS2T67UP72G2
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       266,
			Deprecated: true,
		},

		// SKU: ADBPAJMF8TT82XWT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.large",
			Arches:   arm64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     51,
		},

		// SKU: AFC75TEPA8GA7PSQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2304,
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
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       67,
			Deprecated: true,
		},

		// SKU: AW4EHTYD5EGGQDYR
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     524,
		},

		// SKU: B4JUK3U7ZG63RGSF
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       840,
			Deprecated: true,
		},

		// SKU: BPC4TM9XDNEB8ZKH
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: CEC547W2ASCGJKER
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     37,
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

		// SKU: CRAJUW7BTXFMT2UJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
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

		// SKU: D3YXS6CU28JAFC5C
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
		},

		// SKU: D5JBSPHEHDXDUWJR
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       665,
			Deprecated: true,
		},

		// SKU: DA6SB8KDJDKA9XF7
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     192,
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
			Cost:     185,
		},

		// SKU: DQY8UD78YMRHHGF8
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     10848,
		},

		// SKU: DW64VZC89TS9M2P2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: EBRWZVHDHP2KJAMQ
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     3336,
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

		// SKU: EF9CFHW44C2Y8P2K
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1152,
		},

		// SKU: EJA7KBCVS4SWHAJT
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1356,
		},

		// SKU: EN85M9PMPVGK77TA
		// Instance family: FPGA Instances
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

		// SKU: F7Y95HABXHUJF4C7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     150,
		},

		// SKU: FHFGWVJGRUAB5YUF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: FR98VXGMWMQS3Z6X
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     824,
		},

		// SKU: FYMRC482WM7RTRH8
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: GD2T3UWQ5KHPJY5W
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1944,
		},

		// SKU: GEDBVWHPGWMPYFMC
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       210,
			Deprecated: true,
		},

		// SKU: GS55YKEHKREDFCBY
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     131,
		},

		// SKU: H48ZRU3X7FXGTGQM
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       105,
			Deprecated: true,
		},

		// SKU: H6SW868SY2Q99GC8
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     412,
		},

		// SKU: H6T3SYB5G6QCVMZM
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       1680,
			Deprecated: true,
		},

		// SKU: H9ZN7EUEHC2S7YH5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     340,
		},

		// SKU: HBVJM3Q9S8K6MNPJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     83,
		},

		// SKU: HGB4YKTGHH4Y4YDF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: HJFDBCJN3HB2XFVJ
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: HPBU32YXM9WA52T9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3072,
		},

		// SKU: HQ3KH9WDMB6YS3JR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.4xlarge",
			Arches:   arm64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: HR52RACCFKFG3HCH
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     372,
		},

		// SKU: HRHNQ2H7D23ZK4T6
		// Instance family: GPU instance
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "p3dn.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     31212,
		},

		// SKU: HWNVG5TF6PTW54ZB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     113,
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
			Cost:     11,
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

		// SKU: J5M87SVSFJQPCRPN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: J5XXRJGFYZHJVQZJ
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       333,
			Deprecated: true,
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

		// SKU: J89B5AAND568NGBY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3888,
		},

		// SKU: JVT9JXEVPBRUMA3N
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: K2JEMC88BQXQYSXC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: K6JPTMCG62A8HANJ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: K7ERD2Q28HHU97DT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: KD9S9DZJR8EE2RFW
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2232,
		},

		// SKU: KDAMJ2UVCTJTUEBX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     2712,
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
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: M2GJ3PRXMAKETJ2R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: M3H68G7NCU65G8U3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.2xlarge",
			Arches:   arm64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: MFN4PHX29R79E58Q
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1808,
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

		// SKU: MP9JYZUWJHT7FTFJ
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     96,
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

		// SKU: N2K68WEWR4W3MPBT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     86,
		},

		// SKU: N65QRDXHQYHRRKS7
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: N6F3FXP6CXEGAGDK
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: N6JH4CDBYSGBHA2K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: N7HF529JXCSHT2D5
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
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

		// SKU: NCMD4UCTCC76ZJ75
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: NCTEFNHKJC398BJB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: NN4EGUUQRWVYP98C
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     41,
		},

		// SKU: NRDNUQKUAH28Y2NJ
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     206,
		},

		// SKU: NW5879DE7RSBVE8U
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: NX7C55M75MVZ7A3P
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4944,
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

		// SKU: PB7MPZWURBQSPBJW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: PJ52BSW34UB6CMXE
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6288,
		},

		// SKU: PSXMMQWTBA57FKDB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: Q4R432QKMN3WRXN7
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: Q9SS9CE4RXPCX5KG
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: QA3NBPZEQKZ2K9AR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     20,
		},

		// SKU: QCPH4NB3MBPPU9Q6
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
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

		// SKU: QCUNMEG52NZ8CYGE
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     744,
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
			Cost:     92,
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
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       532,
			Deprecated: true,
		},

		// SKU: QTKEP7Q3GY8UQDGU
		// Instance family: Storage optimized
		// Storage: 8 x 2000 HDD
		{
			Name:     "h1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3744,
		},

		// SKU: QW4FHUGEZYB74TW8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     680,
		},

		// SKU: QXKKJBM3K85S8H5R
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     768,
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
			Cost:     46,
		},

		// SKU: RFGVEPVZTEFFRRQ3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1536,
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
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1330,
			Deprecated: true,
		},

		// SKU: RUFVJ4KVTVSSSHEU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: SDH7JHR69GKRHZE7
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     834,
		},

		// SKU: SEPZUZKD53RX92J8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2040,
		},

		// SKU: SGW3B2WMX7NA6KBX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2752,
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

		// SKU: T4Y25CGHWPQ5TUTQ
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     103,
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

		// SKU: TN5GRYE6RHCET7M3
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     1668,
		},

		// SKU: TTMEAGS3CX5C3W8X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.medium",
			Arches:   arm64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(322),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     25,
		},

		// SKU: TYE7KBFYUK9UE9VR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     9,
		},

		// SKU: TZG8KMRSS5ASP78Y
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: U2Z4RCYVUV8J2GPV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: U3KDJRF6FGANNG5Z
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       166,
			Deprecated: true,
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

		// SKU: UAATQ9C5ZN887JMA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: UC54XWQVHQPRYB9K
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6672,
		},

		// SKU: UJQBNT9Z2N4FF5KS
		// Instance family: Storage optimized
		// Storage: 1 x 2000 HDD
		{
			Name:     "h1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     468,
		},

		// SKU: US4KNUGYQKAD8SVF
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     12240,
		},

		// SKU: UVEWW56VXCE7Z3BP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     332,
		},

		// SKU: UYYVVJAD6TPQ57KS
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3616,
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

		// SKU: VBCEKXAY9ZGYR5SP
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: VDEFC4X4WEBZM9RA
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1728,
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
		// Instance family: FPGA Instances
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
		// Storage: 4 x 1900 NVMe SSD
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
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     7200,
		},

		// SKU: WF68Q2FH2F5QR8Q3
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: WFMWPV2NFFRWRASD
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     144,
		},

		// SKU: WQ8X428F2YDYTBNK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     75,
		},

		// SKU: WWPYQARS34Q8PE6D
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     1808,
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

		// SKU: X634X9DKWKQ3HC94
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
		},

		// SKU: XES86TS9BX33Y86Y
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     24480,
		},

		// SKU: XGAGF5GBAPT4M7WM
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: XKM2TWXAJAXQMAJ4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     5424,
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

		// SKU: XQ9F7S85JQ78K6NV
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: XRJKUE4YQY5SJWQZ
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     262,
		},

		// SKU: XVDMNM2WMYBYVW3T
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: Y3YY4G7632HNZ9JQ
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     576,
		},

		// SKU: YBUW2SRTVYQTE92J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     300,
		},

		// SKU: YGU2QZY8VPP94FSR
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       133,
			Deprecated: true,
		},

		// SKU: YMQM897TE9VFRBD8
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: YNFV4A5QUAMVDGKX
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
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

		// SKU: YZN7AXEPB8PHJCJV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4128,
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
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       420,
			Deprecated: true,
		},

		// SKU: ZDK96RPJGJ9MFZYU
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
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

		// SKU: ZGEXUD6EEZ79TGW9
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1048,
		},

		// SKU: ZGZAXYEA7XDRXSH2
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: ZH8KU2QB7FHAJJXW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
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

		// SKU: ZPJVJX9NBKBMS6TP
		// Instance family: Storage optimized
		// Storage: 4 x 2000 HDD
		{
			Name:     "h1.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1872,
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
			CpuPower: instances.CpuPower(10304),
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
			Cost:     46,
		},

		// SKU: 2PAE8XWKR2MA9MG4
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 34ZEJZENQ3WGN6MA
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       166,
			Deprecated: true,
		},

		// SKU: 3VWERDY4UHEZUS9F
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       2656,
			Deprecated: true,
		},

		// SKU: 3XXU2UHMQUGZ2H7C
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2040,
		},

		// SKU: 3YNRUJVA7RYEWVN5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     680,
		},

		// SKU: 495PTJUX3QUEQ3P5
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     824,
		},

		// SKU: 4E7EDUZD2S93FX73
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1530,
		},

		// SKU: 4HGTTE28P6Q6Z5CH
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     170,
		},

		// SKU: 4QSHENQKE5VDAGGR
		// Instance family: Storage optimized
		// Storage: 8 x 2000 HDD
		{
			Name:     "h1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3744,
		},

		// SKU: 4R9AAXWRV88MGH2M
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     432,
		},

		// SKU: 4YHB8A4RWJM6VBZP
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: 4ZZQPY7KJN7SGNCT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.4xlarge",
			Arches:   arm64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: 5892A64F599FA7UY
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: 5AN96378Q97PAQKS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
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
			Cost:     185,
		},

		// SKU: 5HZRWJ67NRH9FRHM
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     412,
		},

		// SKU: 5KJU7G42EKAGH3H4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     37,
		},

		// SKU: 5S6FV4VGE8UW9QJW
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     3336,
		},

		// SKU: 5X7VYJGK3PY7WUAN
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: 64UZ59KRS55PP3WM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: 654M955HUWUBFBDB
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     524,
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

		// SKU: 67TZ2FJNUK7KJEEC
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1048,
		},

		// SKU: 7G6ZHQDD8B55YKHH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: 7J4E3V54P4EYVX4D
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: 7KZQJPJ9DMP9TKHT
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: 7NNK7ZC4ND2AXRXU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     340,
		},

		// SKU: 7YDWA7NKU6XSKFSU
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     206,
		},

		// SKU: 82N8PRZ8U7GE8XRM
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: 82WJEV73M7J3Q5WS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: 8476NEEEXV7W9W5Q
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: 88H8KNDPXNRCEBAU
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3888,
		},

		// SKU: 8D49XP354UEYTHGM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: 8GK4KCC5UY3FM5FX
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: 8KXQBUFQSAE2569F
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4944,
		},

		// SKU: 8MUE5GW7RQ939SBD
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3616,
		},

		// SKU: 8SJJWBQEQ55NTABM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: 8X9B68EH66TVHPDD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1536,
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

		// SKU: 94WSUX7ND6EM4YNA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4128,
		},

		// SKU: 9DMUVQNHGNHC7R92
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       332,
			Deprecated: true,
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

		// SKU: 9FSPCM7DXJXHRZ8V
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6672,
		},

		// SKU: 9H2GQTY527HSSJFF
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1356,
		},

		// SKU: 9MXQF8NSPZESJJUW
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6669,
		},

		// SKU: 9QWMZQHGYJT4FCNH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.xlarge",
			Arches:   arm64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     102,
		},

		// SKU: 9W9WUN8874RQJPYR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     75,
		},

		// SKU: A3BSWF7X4VU47WXG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: A6PYBM8K8FAMKCGJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.2xlarge",
			Arches:   arm64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: AHRWUFJ3AJQ6MADP
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     750,
		},

		// SKU: ANKQJK29HWAX7AW8
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     10848,
		},

		// SKU: ARA3692KFAJ39EUX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: ATZSNY3GAKQTD4XJ
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     262,
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
			Cost:     92,
		},

		// SKU: B9QJCD97ZQ34V6EA
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: BQPEB7HYTVEZPKJW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     20,
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

		// SKU: C47DG55MKHUP8BQ8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     86,
		},

		// SKU: CEQV2KXNA3CB7Q8X
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: CWBK779A7QPAG983
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     144,
		},

		// SKU: DBBN8V6AE5WQCCGZ
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: DBKA4F5TG2FX3D2E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2064,
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

		// SKU: DU4PGFNABH466WST
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: DWQDRJ9AWUJ8BZKH
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13338,
		},

		// SKU: E9R6RMUH4FPR7URB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: EHXY2FDRBX9SBHMU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: F5AM4SNB3BPQEVR7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     1808,
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
			Cost:     11,
		},

		// SKU: FR8BQ49DN9U32H8Q
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3072,
		},

		// SKU: FSZC3TSJBXWHNWXF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: FV7PUC9Y973899CS
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.large",
			Arches:   arm64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     51,
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

		// SKU: GY4EP95U8PKCRZ58
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     83,
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

		// SKU: H2THDDW6B7P8U78G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.medium",
			Arches:   arm64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(322),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     25,
		},

		// SKU: J2U64VDM9T2PQM6K
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1728,
		},

		// SKU: J3ASENPUVKBMMQG3
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: J6F9NSBAQWA7NC8H
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: J6FHJ5W788GQPSCR
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     12240,
		},

		// SKU: J8F5Q9BVHKTDD4YC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
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

		// SKU: JNDV9943X8PU79JA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: JTMEBXHMXFMQ74R4
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3456,
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

		// SKU: KQDYBADUXPB4WNNK
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     26688,
		},

		// SKU: KXMWDSQ7QVNTCSV7
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: KXZVS9AJBTN5NBSE
		// Instance family: Storage optimized
		// Storage: 4 x 2000 HDD
		{
			Name:     "h1.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1872,
		},

		// SKU: M3HKXCNKPGCZDMCG
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     24480,
		},

		// SKU: M6S6AKBD4WYEHANV
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1328,
			Deprecated: true,
		},

		// SKU: M9KYZQ9AUQJN8Q23
		// Instance family: Storage optimized
		// Storage: 1 x 2000 HDD
		{
			Name:     "h1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     468,
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
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: MNQK42E37YV33WD8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: MQPHBZP7D63TUEBM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: MU9EJXKJ6TW9T65R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     300,
		},

		// SKU: MW8UQZATN9TSYX2K
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1248,
		},

		// SKU: N7KJKBRXVX94TQW5
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: NC47F9MHVAV558W3
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     834,
		},

		// SKU: NVPBFSTB7Q5WP4X5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: NXNRJ4X9TQR2CVBZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: NZX5U28B8SUFVNUN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     332,
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

		// SKU: PUVQARX8GDMUJC2E
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: PVR9E4FZUNNNAM9W
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2752,
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

		// SKU: Q3PQCVWR493GW5JR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: Q3YJ6KRRTZ6VGRGK
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: Q78HZWP6W7SUFFC7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: Q9U6XESRG6MJHQHV
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: QA5JVUUHB8QXYEYM
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
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

		// SKU: QVDGKAHRZKNRS5BV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: QZ9RTRDR6FSDSDTU
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     288,
		},

		// SKU: QZN43S5CUE94W75G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     172,
		},

		// SKU: R3VHSB87F9T6FGA9
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1808,
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

		// SKU: R6GPP3DAS3SFJQEQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     5424,
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

		// SKU: RQ5MTXDCVMC97PG6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2016,
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

		// SKU: S89PRF6W86HYCKPG
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: S9P8MT7D8KG5ZA9B
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
		},

		// SKU: SAAY6F3PMZQGTKU3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: SDQK54UK2HRQTUXV
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: SEMV77DSB4WM675R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: SF5DE6CG3JJC4T33
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13344,
		},

		// SKU: SGFUDXSZXY8TA5AX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     9,
		},

		// SKU: SJ8W88EE5U3YBGYN
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: SVQWW9JRCF3MPR9R
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     103,
		},

		// SKU: T54WSMXQE5PWCK2Y
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6288,
		},

		// SKU: TBV6C3VKSXKFHHSC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     150,
		},

		// SKU: TEVGVY2M4SJF9D6A
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1152,
		},

		// SKU: U4X443UQC5K8CM42
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: U89ZGFBT83MY73GX
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: UAHVXR6M5PWGYBDN
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: UMK29BWNESP838CA
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: URPTCUXU96S8XJUV
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       664,
			Deprecated: true,
		},

		// SKU: UVU9TSGG6QHSMMRD
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: V7N8YPWWSY33BMDU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: VEVCBWPER9F5N296
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1944,
		},

		// SKU: VP2ZAWC24JKYZ8BU
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: VQA7P2UH5V6YG4C4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: VQHZDZ9JA2WEK8JD
		// Instance family: Storage optimized
		// Storage: 2 x 2000 HDD
		{
			Name:     "h1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     936,
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
			Cost:     371,
		},

		// SKU: VWC546ENDWKDXT2T
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
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

		// SKU: WD7DZ22R4WVPGGVW
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: WTADZ55ETP6ZEN73
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: WTHRW5EZ8T22UFYZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     4,
		},

		// SKU: WXTX9CDNRB93XCCA
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3144,
		},

		// SKU: XBRVYNQEYQU6T3AV
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     131,
		},

		// SKU: XKNQ264P5N9AYCD2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     18,
		},

		// SKU: Y3SUYSYH3DS2J5JK
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2472,
		},

		// SKU: Y6WNUUFEJZ2F8Q3U
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     3616,
		},

		// SKU: Y73QEJPSBYNQCFQA
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     576,
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

		// SKU: YNYQ6X5V79PKWGEB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: YQ4YBPGFTJTMW8AW
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     1668,
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

		// SKU: YR3MEJZD3USM8NC3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     41,
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

		// SKU: ZMUUYVBE4X9YWP8X
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: ZSX3NBYHSUTH7852
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: ZWW6ZZSFWQM7H534
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
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

	"us-gov-east-1": {

		// SKU: 2JP2RZ2UJV9ZNZ44
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     218,
		},

		// SKU: 3BSCYEGMN6MDKE4A
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     272,
		},

		// SKU: 3MSS6P2NEM9K2TX2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     130,
		},

		// SKU: 3T6STNYJFAW8S33C
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     390,
		},

		// SKU: 42877UXD5ZZANTV7
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
		},

		// SKU: 4B3MDE2DZQEYBYN5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3624,
		},

		// SKU: 4B7TQFB6R5WR9QWV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     102,
		},

		// SKU: 4CTYYA8B7B8BMS2P
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     302,
		},

		// SKU: 4ZPACZYD4WQVJABW
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     116,
		},

		// SKU: 5BQHU4EB2XYRHTTD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5808,
		},

		// SKU: 5KVP4KP4GBF37USW
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8003,
		},

		// SKU: 5PDD9MBZEZ24EN9J
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2416,
		},

		// SKU: 5PNAZFQEV2Q3V4BA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     24,
		},

		// SKU: 6AF48G4NKPDY9NPS
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     816,
		},

		// SKU: 6E26BGBRXA5V4BFZ
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6864,
		},

		// SKU: 6P2CDHHADRT38968
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8000,
		},

		// SKU: 6RGZEZZ6B7G547GG
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     32000,
		},

		// SKU: 6VZ35KPQMK9QN634
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     752,
		},

		// SKU: 7JSU4T632MZDZ3FA
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6016,
		},

		// SKU: 7XCFXQQRZMESY82A
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5536,
		},

		// SKU: 7YW7574VDRCAMZCF
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     232,
		},

		// SKU: 885A6CTBYSBM4TXU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     48,
		},

		// SKU: 8MZ7XQWQCMDCJXBX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2176,
		},

		// SKU: 9C7YF46K8XDGZ6H4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     1040,
		},

		// SKU: 9Y9MYMMP39URV28F
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: A57DAG6Q6KANZYFF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: AACFXKCCEPN44P3E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1744,
		},

		// SKU: AC5FBVYHCXHX6QUW
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     464,
		},

		// SKU: ADEEY3CR577FG7RF
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1836,
		},

		// SKU: AJTK82YPWW68T6RZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     109,
		},

		// SKU: BKG672UU7WX3K5UV
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     143,
		},

		// SKU: C6S89RGC8T8CQKRD
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     87,
		},

		// SKU: DAG7WTTKJQQ98J6A
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6864,
		},

		// SKU: DKVF2W52A4BRNHDJ
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2088,
		},

		// SKU: E9PSPMYM8T5XVVED
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: EWDN2VY54HQ5DKN4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: EWQS3T2TKFVNU29G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     175,
		},

		// SKU: F3ZZK877K7FKTECE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1208,
		},

		// SKU: FFDDW45X54SBTZFX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: G9FUGNWMAFT47VYR
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2340,
		},

		// SKU: GCB5CGQS387UTE2J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     351,
		},

		// SKU: GXMHUG8R7BN5UFK8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: HD598PVEM3ZDZZXK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1088,
		},

		// SKU: HXWX3CZM593M3277
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: J4TF9UR8GKBGY5YY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3488,
		},

		// SKU: JBC8FDMRYZGXPN79
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4176,
		},

		// SKU: JBP4Y3UCSNVS2QA8
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4152,
		},

		// SKU: JJ24SW259JKKJPAB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     97,
		},

		// SKU: JSWSYTZBQZ4YBV3A
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4576,
		},

		// SKU: JUWFKAMPQHWRMTGY
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: JWVTMBF5WGMXNN8F
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     173,
		},

		// SKU: K66X3AAUUA7XKK6A
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: M9M723K8ST4FKVVX
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5232,
		},

		// SKU: MEG367PKZ4YJUFB2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     242,
		},

		// SKU: MEVFQWGV247YJKSZ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     22,
		},

		// SKU: MMDBHSHVDWTVBK36
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     436,
		},

		// SKU: NDGNUDQRX2JJHV4V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     195,
		},

		// SKU: NH3VB5DD6YBVG942
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     520,
		},

		// SKU: NTF8SWN2494FB363
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3672,
		},

		// SKU: NYJZKPSE5EZGRZP2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     872,
		},

		// SKU: P79MA5PN8FQMRVCT
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4832,
		},

		// SKU: PGESBEKWBGA7AYQQ
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16000,
		},

		// SKU: PJXAZ94HEGUDKQCN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: PU5PCUP9B8H9XMJP
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     572,
		},

		// SKU: PXDS4AUAJKDEC536
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1384,
		},

		// SKU: Q5P22SX4GF385EQ9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     260,
		},

		// SKU: QBDMXXC3W8BWVRFE
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4000,
		},

		// SKU: QBJ82A9GH2KDSCFV
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     346,
		},

		// SKU: QDJ4GG4D7VXRVJD9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3872,
		},

		// SKU: QRURV6FNCGKE5Z59
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2904,
		},

		// SKU: QS9BQ4HKAP9VDY7R
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1144,
		},

		// SKU: QZTDUWT3EDE9F7KM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3264,
		},

		// SKU: R4HW4HR6VJ7XTZJ6
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     286,
		},

		// SKU: RB8VJPVZZFHQACRG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5808,
		},

		// SKU: RC7URFHQM8Z48KBE
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4352,
		},

		// SKU: REQ6CTDU7NTX4T35
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: REX4A8M62Y2CEAFY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: RK3PD4KKH7FWBE3R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1936,
		},

		// SKU: RKBZQD4T69K7WUWK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4680,
		},

		// SKU: RQ5G6CAHKE4RHPYB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6528,
		},

		// SKU: S62D55ANHFGZVRCZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     604,
		},

		// SKU: S8VXNDPFFBJ8N4WM
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3008,
		},

		// SKU: T6Y46EG2ECGDC83P
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2288,
		},

		// SKU: T8UD3M4M99H2UW67
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     692,
		},

		// SKU: TFDF2GWHXZQ4VHPK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: V6CT9C678B38GJZG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     43,
		},

		// SKU: W7DXMZ37WZBDSTWE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     484,
		},

		// SKU: W7SKXU5HQ2R7Q9FN
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     121,
		},

		// SKU: WJBC9BHSS3DRPZ8W
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     928,
		},

		// SKU: X57FTBXHKXQWVAG9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2616,
		},

		// SKU: XDDM68TGB65356ND
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
		},

		// SKU: XDUGQNCQYCT3JS9K
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2000,
		},

		// SKU: XEVCBCVN5VRCEVBM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     968,
		},

		// SKU: YS4Q5EMW2MWDFEFR
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3432,
		},

		// SKU: YZWFWZJEAPV9MXBM
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1504,
		},

		// SKU: ZCVYTVSQTUZ8RRY4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     151,
		},

		// SKU: ZN5QSWX945VZRW9S
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2768,
		},

		// SKU: ZT8PWT4Y4SJMKPSB
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
		{
			Name:     "x1.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16006,
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

		// SKU: 2K725845PKPNPEWY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4896,
		},

		// SKU: 2NZKSFZUY66PW2EW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     121,
		},

		// SKU: 2QNMBVWSZXRZWZ2C
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     692,
		},

		// SKU: 36MHZ5RTTAQCE8NE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     872,
		},

		// SKU: 3GCCV4BFBUGPG9KP
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     6016,
		},

		// SKU: 3XKAD26QBSE5WU9H
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     43,
		},

		// SKU: 3ZXMHJ45NPHA6SFS
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1208,
		},

		// SKU: 4MA5XQFXJBM6QN2U
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: 4N4NEZXJBYT76PG7
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     15840,
		},

		// SKU: 4RK5NRQFCBCGADRB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2176,
		},

		// SKU: 52VDEB7HG755EUT9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3672,
		},

		// SKU: 56J8SAHNA3VHES8R
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     544,
		},

		// SKU: 5FR2446ZVKVMX5JH
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     752,
		},

		// SKU: 5HJ2YHEMYC92SVTK
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3792,
		},

		// SKU: 5RYDEBNAFS5MXW6W
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     273,
		},

		// SKU: 65ZFHGKUSE5MB7ZS
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2288,
		},

		// SKU: 663Z8A4VB2UCQRKZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1836,
		},

		// SKU: 67CCVDAN2HAKD4T9
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3672,
		},

		// SKU: 68CYGFBKN5JU2WD4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: 6CVNTPV3HMNBWRCF
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       84,
			Deprecated: true,
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

		// SKU: 6EAJ4QBCEPTC9BHH
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: 6GSR69QXVXUSEGUC
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3432,
		},

		// SKU: 6J7ZTVPXJKAX6EB3
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       672,
			Deprecated: true,
		},

		// SKU: 6MTS7X6337MP3TAN
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1144,
		},

		// SKU: 6PMSJ4V5N26J36BG
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       399,
			Deprecated: true,
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

		// SKU: 6SQU67HGRKCYPDQD
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     262,
		},

		// SKU: 6U6EZXUHGPZ68BDE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2448,
		},

		// SKU: 6VTUAGVX6J37U9GQ
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       1008,
			Deprecated: true,
		},

		// SKU: 73J9DPY5G29AEFPN
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4000,
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
			Cost:     435,
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

		// SKU: 7RFMHDMZ3VF3UQJP
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8000,
		},

		// SKU: 7UN7N6MPJA6PBYGC
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3624,
		},

		// SKU: 7ZFUB5QX3YSRBCWZ
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1264,
		},

		// SKU: 85M6SD36VCRCB7GC
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     464,
		},

		// SKU: 85WTNMVMU8CS72PE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     97,
		},

		// SKU: 89EV5BSMPDHAKNGR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2016,
			Deprecated: true,
		},

		// SKU: 8CAU2DVQR77EN5WK
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4832,
		},

		// SKU: 8EGTMUXWUDRZRFS6
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     928,
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

		// SKU: 8HP9A5XCZQEVA9CM
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4152,
		},

		// SKU: 8KYWYJ8XJ9JWJ92B
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     484,
		},

		// SKU: 8Q7ZY3FKN9C5MFMZ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     29376,
		},

		// SKU: 8ZHG5UU6VBR32AEW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2616,
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

		// SKU: 9UMWX3HSF4U3888R
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     520,
		},

		// SKU: 9UXCFAE7FMJYNC4X
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1088,
		},

		// SKU: 9ZDJD42NCMFZZ9Y4
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     102,
		},

		// SKU: A5EN5X5DCNGRXPV6
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     188,
		},

		// SKU: A6HSZKUTZDMMFS9P
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     17280,
		},

		// SKU: AFDPGFY2X4SY8NGG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     109,
		},

		// SKU: AKPPTYFFG8PF9Q7C
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5232,
		},

		// SKU: AKTZV7N75S4UB57N
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     131,
		},

		// SKU: APQCW52KSG39799Y
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     130,
		},

		// SKU: ARJKD76SHK5JUB4R
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     260,
		},

		// SKU: B7ZPXZR5RANUT4G7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     22,
		},

		// SKU: BFSWC36NDW9D3ET5
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     14688,
		},

		// SKU: BG8ZZS943JVR6HGP
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1384,
		},

		// SKU: BJWD9AQ7FJRUQH36
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     346,
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
			Cost:     108,
		},

		// SKU: BRJ2ACG4CABVZU42
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2416,
		},

		// SKU: BT5SMY36R7A5QJWP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     351,
		},

		// SKU: BW28RAXSZTKZDY5X
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     1040,
		},

		// SKU: BXAR9D46EJJXYWD9
		// Instance family: Compute optimized
		// Storage: 2 x 80 SSD
		{
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       504,
			Deprecated: true,
		},

		// SKU: CJWVHDR9P8PQBFAU
		// Instance family: FPGA Instances
		// Storage: 1 x 940 GB
		{
			Name:     "f1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3960,
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

		// SKU: CREUXG6M3HP4VZVA
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     286,
		},

		// SKU: CXM9YR66XHMFFJDD
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       168,
			Deprecated: true,
		},

		// SKU: D3BZ5Y4A8AZ9RPF6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1936,
		},

		// SKU: D7PCHCCFFBP5Q3XA
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2768,
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

		// SKU: DQ59RPFS2B6D4TB6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: E6FC3J7P796W9BB5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     242,
		},

		// SKU: EB4ANNF24ENUY33S
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1092,
		},

		// SKU: EC7MHMXZTSUXKK5K
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3488,
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
		// Storage: 2 x 1920 SSD
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

		// SKU: F87GHBA3KTY9Z8PB
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6528,
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

		// SKU: FS76Z6C3M6EAKEN2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2904,
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

		// SKU: GMFGDG364YV2QUTZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: GRP3YHTBDHMGVA7S
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     390,
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

		// SKU: GZWT5MT5CBD6Q5C8
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1048,
		},

		// SKU: H2JDFG4KAR24BKSQ
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3008,
		},

		// SKU: H8PUMXYJMJJXXKNE
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     524,
		},

		// SKU: HBTM8FV4QZ3QKCTJ
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3276,
		},

		// SKU: HG3CK5KRF5FG2E4M
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     24,
		},

		// SKU: HG6MMP6EXHHDKJMM
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1638,
		},

		// SKU: HHRWRMAJ8BWHRQEH
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2000,
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

		// SKU: HZBZAYZCXFSEW2JS
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6864,
		},

		// SKU: J6DM639K6488JBRH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5808,
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

		// SKU: JCUEBSR62P6TXZK2
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     32000,
		},

		// SKU: JG2FE8MWANQ723FP
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4176,
		},

		// SKU: JURTRRHRGBD6C9FQ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     604,
		},

		// SKU: K5CWXN5HSW7SME2R
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       798,
			Deprecated: true,
		},

		// SKU: K5YNT8Y77VE95C4X
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5536,
		},

		// SKU: KMX34A7NQZQRJAED
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
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

		// SKU: KSRA82HYG8MVA2AH
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6864,
		},

		// SKU: KT5XYNZBAZ6PAHHJ
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     376,
		},

		// SKU: KZASPVGTJWQWZSTH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     151,
		},

		// SKU: MAE7DQAWJXBHQ3GQ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     816,
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

		// SKU: MHEZVMT9AZ2399K5
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     158,
		},

		// SKU: MJUQS6SV9MH6KENV
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1980,
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
			Cost:     217,
		},

		// SKU: MT4K7W9KTWWCVEXY
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: MZ2Y48Q6JW6FQAW6
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6552,
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
			Cost:     13,
		},

		// SKU: NS2RS35DZJVM4XV9
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3144,
		},

		// SKU: P26K99WGYAQ64KU3
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     16000,
		},

		// SKU: P76AN6DXWYCD69GD
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       200,
			Deprecated: true,
		},

		// SKU: P98E5YPSYQERNYZJ
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     232,
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
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     8003,
		},

		// SKU: PR3XTEQYEMKGK56X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1744,
		},

		// SKU: Q5SR4HEEE49HTJVD
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3264,
		},

		// SKU: Q7NZ6NXAFJHFS2XM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     436,
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

		// SKU: Q9S4SVWHPSQ62JK4
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7248,
		},

		// SKU: QFNETRGV4DS2M843
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     195,
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
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1504,
		},

		// SKU: QHB3JCWPKY4VMW5X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     968,
		},

		// SKU: QRAX5KVV6V4U6VNW
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2088,
		},

		// SKU: QU5F7BQXB5HEP9GM
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     136,
		},

		// SKU: QZW5BCNW7UGVZMW7
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     13104,
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

		// SKU: R8BTRHAXRUJUXFUB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3872,
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
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       252,
			Deprecated: true,
		},

		// SKU: RV9B2TDJGG9PNYKR
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     8640,
		},

		// SKU: RY97T2T2385G4KXW
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1596,
			Deprecated: true,
		},

		// SKU: S4GDPTPB4XRFAVK2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     87,
		},

		// SKU: SB286V7BKUKESM8K
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4576,
		},

		// SKU: SBKREW9XBMMZC92R
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     8304,
		},

		// SKU: SKBB4M9KK763WCPP
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     632,
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

		// SKU: T86C44HJYZE3B87H
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     48,
		},

		// SKU: T8CH83JJQVY2JM2Q
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6288,
		},

		// SKU: TESR8D5WV58EK4Y9
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     1080,
		},

		// SKU: TGWX9AZUFZC5ENDA
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     6016,
		},

		// SKU: TUAPFFJ9NU633BRC
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     173,
		},

		// SKU: TWFCEA7E4N3YPC35
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4680,
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

		// SKU: UMXTGE2C75WWW746
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     272,
		},

		// SKU: UQD9RGYY5BJ764UW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5808,
		},

		// SKU: UV3Y2PYJ62CGS6VK
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     143,
		},

		// SKU: VJTWT88A3FF7YSQG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     302,
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
			Cost:     27,
		},

		// SKU: VXKKRPEQERAMFSFJ
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       336,
			Deprecated: true,
		},

		// SKU: W6ABH88PE5BKSJZQ
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       3192,
			Deprecated: true,
		},

		// SKU: WEYBDECTM5UQP75N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4352,
		},

		// SKU: WK9EUC4S5NXYW46G
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7584,
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

		// SKU: X23ZPVWPEJK2FQE5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     175,
		},

		// SKU: X5FANMGVETDMD583
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
		},

		// SKU: X6NCVDB2HVPPBYAW
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4896,
		},

		// SKU: X7CV8KAZMBGK56NH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: X9QQDFJ8MS8FB32D
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     316,
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

		// SKU: XDEC58SRN2MK7F78
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     572,
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

		// SKU: YN5S2Q9BCQSEX4Q9
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     116,
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
			Cost:     54,
		},

		// SKU: ZVUBM6W3KFQ992U7
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     546,
		},

		// SKU: ZW4PMFXWG3VRVNQT
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     218,
		},

		// SKU: ZZ36VPB4D4R6MZDB
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2340,
		},

		// SKU: ZZTR42B59D85VUCY
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       126,
			Deprecated: true,
		},
	},

	"us-west-1": {

		// SKU: 288NWCDVAPRH3QVZ
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     266,
		},

		// SKU: 29RD5CTAQBQAK4XU
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     648,
		},

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

		// SKU: 2H4VRPX8C966P9HV
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     250,
		},

		// SKU: 2M44JQQN3ZP9874A
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2808,
			Deprecated: true,
		},

		// SKU: 2UKWKKWFSYDMCT8R
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3584,
		},

		// SKU: 3BMMVHGDPC8U7FUS
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5184,
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

		// SKU: 3CXW7HSXSH7UGYW5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4848,
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

		// SKU: 3QGUTWMS5CQT2QBR
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: 3S2REVZBANWCJMV4
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     162,
		},

		// SKU: 48KG2ZJ2XRJK5HGE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     89,
		},

		// SKU: 4GAV6VD5FWD8W8B4
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       1912,
			Deprecated: true,
		},

		// SKU: 4NMSK89RC7VGAR4V
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2240,
		},

		// SKU: 55XSQ77GD8XY6882
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     844,
		},

		// SKU: 57Y9AS6APUSGYHMU
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     2160,
		},

		// SKU: 5EDHWY2G68899HDR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     6,
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

		// SKU: 5UQKWM2AG2998KAV
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1296,
		},

		// SKU: 66RZMKMKHA5H5VT9
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5064,
		},

		// SKU: 6CQ5UBR3M3YEXGAW
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     120,
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

		// SKU: 6HSUA77D4PHGHSKF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     112,
		},

		// SKU: 6NNNM36C5C8XQQ7Q
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     140,
		},

		// SKU: 6RAPYW9Q7M9FWF8K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: 6RPPWGMT84SMF5Z6
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3232,
		},

		// SKU: 6ZT2UARGHJXH8BU5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2688,
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
			Cost:     220,
		},

		// SKU: 792U3X9RH336KUZH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     22,
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
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       478,
			Deprecated: true,
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
			Cost:     55,
		},

		// SKU: 7XJFQJMS96ZXXUKY
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6384,
		},

		// SKU: 7XUFU6HE6CP4M3PP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     202,
		},

		// SKU: 85885MGYVC63N4D2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5088,
		},

		// SKU: 87ZU79BG86PYWTSG
		// Instance family: GPU instance
		// Storage: 1 x 60 SSD
		{
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       702,
			Deprecated: true,
		},

		// SKU: 8RXDZ4H62GHK7Y7N
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       154,
			Deprecated: true,
		},

		// SKU: 8VWVTP4EDUAYRGEP
		// Instance family: FPGA Instances
		// Storage: 1 x 940 GB
		{
			Name:     "f1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3826,
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

		// SKU: 9AMRUT5XMFV878DK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     848,
		},

		// SKU: 9DTEZYGH5YCGDVG5
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     500,
		},

		// SKU: 9DU7KZEEYV78WSBR
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1000,
		},

		// SKU: 9V4YEGESP35GKP5B
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     896,
		},

		// SKU: AAYJQZRJAETAHGUX
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6384,
		},

		// SKU: AC24BB2N8MSTAUUX
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     324,
		},

		// SKU: AJSC7HEVXUFD9MZN
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7776,
		},

		// SKU: B5Q7NF72EUPHKS52
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3192,
		},

		// SKU: B9UZAF83X7Q499JR
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     4320,
		},

		// SKU: BHCQX2AQAPW58YSA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: BYPM4YE43QWDFJZX
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3816,
		},

		// SKU: C92BKZX2RSH8PCV8
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1792,
		},

		// SKU: C9T4V58VZ3SK8FMK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     24,
		},

		// SKU: CKTY9GB26YVYKQB4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     99,
		},

		// SKU: CPMW22QCMCVDTTBH
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     960,
		},

		// SKU: CTG879VYY65QE94C
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       616,
			Deprecated: true,
		},

		// SKU: CTW2PCJK622MCHPV
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       2964,
			Deprecated: true,
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
			Cost:     110,
		},

		// SKU: D9QYX5W68MPNUNR3
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1266,
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

		// SKU: DFTSKTFNKWM4G5WC
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     480,
		},

		// SKU: DPATDARKDKR94NR5
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     11,
		},

		// SKU: DY2N5SD2ERNEAQQQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     357,
		},

		// SKU: E4YZCUR3KN9FNNXC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     44,
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
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: ET3ZNXJBEAKFNCWT
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     280,
		},

		// SKU: EVU8KWYYY4WYD5A2
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     1064,
		},

		// SKU: EVYB78ZE853DF3CC
		// Instance family: General purpose
		// Storage: 2 x 40 SSD
		{
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       308,
			Deprecated: true,
		},

		// SKU: EW2CSSRETV5MW6DD
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3000,
		},

		// SKU: EYHRFJPGE2JJZNRK
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     6000,
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

		// SKU: F7JBXS8VUCMWV3EA
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     504,
		},

		// SKU: F9XWRY93YTTZ4UCH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     560,
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

		// SKU: FEX77SY68SVDW2VP
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     2128,
		},

		// SKU: FM4FMMAPY4SD8Y9T
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     211,
		},

		// SKU: FNF3D2E8J5ABDD2W
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3360,
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

		// SKU: G3KCCCZ7S3K93YZ7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6720,
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
			Cost:     6,
		},

		// SKU: GSHUSSEQCN7RYB45
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     5088,
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

		// SKU: GXRKVZ6K4332G4MT
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2592,
		},

		// SKU: H8QFMXWT89NGP6VU
		// Instance family: Compute optimized
		// Storage: 2 x 16 SSD
		{
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       120,
			Deprecated: true,
		},

		// SKU: HADSVEHF9DYR4P7T
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4480,
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
		// Storage: 2 x 1900 NVMe SSD
		{
			Name:     "i3.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: HZYY3GHU6ZDWFGAJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1616,
		},

		// SKU: J5UNF2XTPQCS5N59
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1482,
			Deprecated: true,
		},

		// SKU: J6ATSGQU69WY57Z8
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
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
			Cost:     13,
		},

		// SKU: J8NWUR4HHDFDPBW2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5376,
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
			Cost:     27,
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

		// SKU: KDN94QDQX8YG36MV
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     422,
		},

		// SKU: KWHQTN3VC9TZDMVK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     212,
		},

		// SKU: M9DKHMVT4ZDGZVUU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: M9QTFFFRQC7KYS3Q
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: M9YXXJD2MQWVXXNK
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5064,
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

		// SKU: MUM5ATP7A8KAJAMZ
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3888,
		},

		// SKU: MUR692RSQPYGJKT6
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: N4KG79XZEJHQR4RF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     198,
		},

		// SKU: N8JKAXF693B9PWC2
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     5504,
		},

		// SKU: NKEWF2C6MB8W9EDQ
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     12000,
		},

		// SKU: P2U3FATXBD2AZZRJ
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     133,
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

		// SKU: PBWSRJYPB4NAU7KA
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     15304,
		},

		// SKU: PQB2RP39S4D4GZ9T
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     808,
		},

		// SKU: PSNEQGH9XVX3FSE8
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       956,
			Deprecated: true,
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

		// SKU: QJP2PXQQVHNBCV2E
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2532,
		},

		// SKU: QUWUPBD4TYK6WWX8
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     106,
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
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       371,
			Deprecated: true,
		},

		// SKU: RQAXWDYSUKFXXEMZ
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: RT9NWVFZWQDDRNES
		// Instance family: General purpose
		// Storage: 1 x 4 SSD
		{
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       77,
			Deprecated: true,
		},

		// SKU: RTM3GBM82TKKK3QC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5376,
		},

		// SKU: RVN23FKVQYG7ZY9N
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     7776,
		},

		// SKU: S4V7MN5TDJSWKM4G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
		},

		// SKU: S6XCDNADM5DDPNUP
		// Instance family: Compute optimized
		// Storage: 2 x 40 SSD
		{
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       239,
			Deprecated: true,
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

		// SKU: SAH3QX23HKZWRM89
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2544,
		},

		// SKU: SBSCQDRQ3CNSZR8F
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     178,
		},

		// SKU: SN8ZDG73ZF6HJWHY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1120,
		},

		// SKU: SSJCQD5PGMEAPN84
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1500,
		},

		// SKU: TE3NJKA4GYN9Z5CA
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     532,
		},

		// SKU: TJVQCXR689T6F2WB
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     396,
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

		// SKU: TYJPPC4RGCTVTZBK
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     448,
		},

		// SKU: U5UGF8V3EG7QA58Z
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     224,
		},

		// SKU: UEEQUH9C49476TP9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     12,
		},

		// SKU: V26E52XF5QEYGY6H
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     5504,
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

		// SKU: V8VDPH7296G3N253
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1908,
		},

		// SKU: VH7ZQKRDQCTF7FS3
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     1008,
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

		// SKU: VWJ8EAZQXR45BUB6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6720,
		},

		// SKU: VZ7V29X35F98VENC
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       741,
			Deprecated: true,
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

		// SKU: WEK8ZYZ7MK9F3N3J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     404,
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

		// SKU: WNMENH9XJUQM9SDU
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: WQ699R6PGW5ZYUXA
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     4256,
		},

		// SKU: WUBJ9UB3Q9FCFGD4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     49,
		},

		// SKU: WV35NXKAWHETXNZ2
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     424,
		},

		// SKU: WXD9TBEW8KKAWMRU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2424,
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

		// SKU: XAVV4MFA7X8RVDG2
		// Instance family: FPGA Instances
		// Storage: EBS only
		{
			Name:     "f1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     1913,
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

		// SKU: YD5777Z69GQ46KFY
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: YEBYKWFDGUVYHGNM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     101,
		},

		// SKU: YMBFM4KJMDQUM2ZU
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     240,
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
			Cost:     441,
		},

		// SKU: YY26V92H8QNEPQER
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       185,
			Deprecated: true,
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

		// SKU: 22XBSF5QFVFX722A
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     170,
		},

		// SKU: 24A8QU3TNXSCKF57
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     85,
		},

		// SKU: 293H6XPEKTRESH8X
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.xlarge",
			Arches:   arm64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     102,
		},

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
			Cost:     46,
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

		// SKU: 2TWCWD5GP8UH398Z
		// Instance family: Compute optimized
		// Storage: 1 x 200 NVMe SSD
		{
			Name:     "c5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: 2VEE5YPDDGDW3STK
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     340,
		},

		// SKU: 34G9YZGUJNTY6HG9
		// Instance family: Memory optimized
		// Storage: 2 x 1920 SSD
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
			Cost:     371,
		},

		// SKU: 3XWVYA8K9P69F62C
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     126,
		},

		// SKU: 3Z49P3MN2R3G6NNH
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: 3ZD39ECB23FRWNNP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     166,
		},

		// SKU: 4AGTNYA38KJHC7JR
		// Instance family: Storage optimized
		// Storage: 1 x 2000 HDD
		{
			Name:     "h1.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     468,
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

		// SKU: 4JUJ6N8SE9QCPFQV
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     3888,
		},

		// SKU: 4RXXTS8AN9PDKJUA
		// Instance family: Storage optimized
		// Storage: 8 x 7500 NVMe SSD
		{
			Name:     "i3en.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(41664),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     10848,
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

		// SKU: 58Z6TCNJT8BEMMJX
		// Instance family: Memory optimized
		// Storage: 1 x 480 SSD
		{
			Name:     "x1e.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     3336,
		},

		// SKU: 59UCQMPPJJA5QRY4
		// Instance family: Storage optimized
		// Storage: 2 x 1900 NVMe SSD
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

		// SKU: 5TB6N73PCPRTRS2N
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     452,
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
			Name:       "c3.large",
			Arches:     both,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(783),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       105,
			Deprecated: true,
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

		// SKU: 6QPC7M9QGUBM2P33
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: 738PEGF9B547JA5Y
		// Instance family: Storage optimized
		// Storage: 1 x 7500 NVMe SSD
		{
			Name:     "i3en.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(5208),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1356,
		},

		// SKU: 7528XD9PPHXN6NN2
		// Instance family: GPU instance
		// Storage: 2 x 120 SSD
		{
			Name:       "g2.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11647),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       2600,
			Deprecated: true,
		},

		// SKU: 7RMRB492WTPDQ5Z4
		// Instance family: Memory optimized
		// Storage: 1 x 32 SSD
		{
			Name:       "r3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        15616,
			VirtType:   &hvm,
			Cost:       166,
			Deprecated: true,
		},

		// SKU: 7TXU2CNQ5EQ3DWYS
		// Instance family: Storage optimized
		// Storage: 1 x 1900 NVMe SSD
		{
			Name:     "i3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     624,
		},

		// SKU: 83H2W4FZK86NAGZF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     41,
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

		// SKU: 86J28WAK2EGUPWT8
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3024,
		},

		// SKU: 8942X3VWHAHGAK6E
		// Instance family: Storage optimized
		// Storage: 2 x 7500 NVMe SSD
		{
			Name:     "i3en.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(10416),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: 8AWP3N55PXMB23X7
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: 8JAR3VJZ6YMXY7ZQ
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6288,
		},

		// SKU: 8UZJBKH62FYWW5EG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     20,
		},

		// SKU: 8XZPEU2QRPPKMP9G
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     332,
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

		// SKU: 9GC24HRY3CSARP8W
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.metal",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4464,
		},

		// SKU: 9GHZN7VCNV2MGV4N
		// Instance family: Compute optimized
		// Storage: 2 x 160 SSD
		{
			Name:       "c3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(6271),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       840,
			Deprecated: true,
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
			Name:       "c3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1567),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       210,
			Deprecated: true,
		},

		// SKU: 9NYZPXNGMMFDRHXQ
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "z1d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     744,
		},

		// SKU: 9P8S248T73959JDU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     86,
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

		// SKU: 9VVY42SVXEGDEQN4
		// Instance family: Memory optimized
		// Storage: 1 x 240 SSD
		{
			Name:     "x1e.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     1668,
		},

		// SKU: A36KJNEX3EM3YH8E
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
		},

		// SKU: A9QN78QAH946UB5M
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: ADANQRUBZ2YCFVRE
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     262,
		},

		// SKU: ADDC9QHKZMBQ3X4K
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     144,
		},

		// SKU: AHGHN23DCREK9Z2J
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2064,
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

		// SKU: ASSHEBTQZZE5YCR6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     2712,
		},

		// SKU: AUK8MP628YXA2Z3C
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     688,
		},

		// SKU: AYQQ5QHPH89ZCU32
		// Instance family: Storage optimized
		// Storage: 4 x 7500 NVMe SSD
		{
			Name:     "i3en.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20832),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
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

		// SKU: B87CFB7Q7JBYBRYH
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: B9CHR4Q4RD7H5AKW
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
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

		// SKU: BS76WYFGRD22TJFD
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: BTTKT9HYGFN28U52
		// Instance family: Compute optimized
		// Storage: 1 x 50 NVMe SSD
		{
			Name:     "c5d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: BURDHCBQ43KQJK2B
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1008,
		},

		// SKU: C4QTB4RTUHBDCE37
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      43008,
			VirtType: &hvm,
			Cost:     864,
		},

		// SKU: CP2TNWZCKSRY486E
		// Instance family: General purpose
		// Storage: 2 x 80 SSD
		{
			Name:       "m3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        30720,
			VirtType:   &hvm,
			Cost:       532,
			Deprecated: true,
		},

		// SKU: CPVU2ZPWWE5VFCPA
		// Instance family: Memory optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "z1d.6xlarge",
			Arches:   amd64,
			CpuCores: 24,
			CpuPower: instances.CpuPower(2400),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2232,
		},

		// SKU: CRRNK6SJUP8ZYXTP
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      512,
			VirtType: &hvm,
			Cost:     4,
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

		// SKU: DBE8EZ6UJHBK8JHQ
		// Instance family: Compute optimized
		// Storage: 1 x 900 NVMe SSD
		{
			Name:     "c5d.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1728,
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

		// SKU: DXE8XDENS3K5VDDP
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: E7T5V224CMC9A43F
		// Instance family: General purpose
		// Storage: 1 x 32 SSD
		{
			Name:       "m3.large",
			Arches:     amd64,
			CpuCores:   2,
			CpuPower:   instances.CpuPower(700),
			Mem:        7680,
			VirtType:   &hvm,
			Cost:       133,
			Deprecated: true,
		},

		// SKU: EKD5U9DBM7RJP86K
		// Instance family: Storage optimized
		// Storage: 1 x 1250 NVMe SSD
		{
			Name:     "i3en.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(868),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: ENHKAF34GECXM7JC
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5d.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     576,
		},

		// SKU: EPVENCXNXBPPPRCU
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2304,
		},

		// SKU: ERP9C2HEMDCRYXH4
		// Instance family: Storage optimized
		// Storage: 2 x 2000 HDD
		{
			Name:     "h1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     936,
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
			Name:       "c3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(3135),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       420,
			Deprecated: true,
		},

		// SKU: FFJFJF6E9ACSJXTV
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.nano",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      512,
			VirtType: &hvm,
			Cost:     5,
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

		// SKU: FRHRUSDHQKK7J6CG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.4xlarge",
			Arches:   arm64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5152),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     408,
		},

		// SKU: FWW3KU6PSYK6P76Y
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1048,
		},

		// SKU: FYPJZG6M2SUV3623
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     226,
		},

		// SKU: FZJS4Z4XFXRVPFQT
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "z1d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     372,
		},

		// SKU: G5E5GHR5CPPQ68UZ
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
		{
			Name:     "x1e.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      1998848,
			VirtType: &hvm,
			Cost:     13344,
		},

		// SKU: G6AP64VM66JC8YN6
		// Instance family: Storage optimized
		// Storage: 4 x 2000 HDD
		{
			Name:     "h1.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1872,
		},

		// SKU: G7MRDHQJ9V4K22BJ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2752,
		},

		// SKU: G8QPBUA7JFN8S9Q2
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     113,
		},

		// SKU: GH8WPQ59336X6S8E
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     75,
		},

		// SKU: GJ4BPQDYNAF6YTH7
		// Instance family: Memory optimized
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "r5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1152,
		},

		// SKU: GMTVCDVQWVYEWFNN
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     1808,
		},

		// SKU: GMTWE5CTY4FEUYDN
		// Instance family: Memory optimized
		// Storage: 1 x 160 SSD
		{
			Name:       "r3.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2800),
			Mem:        62464,
			VirtType:   &hvm,
			Cost:       665,
			Deprecated: true,
		},

		// SKU: GSDQ269UBE7A7CBM
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2472,
		},

		// SKU: GT3FB45EUGGQBFCB
		// Instance family: General purpose
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "m5ad.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     206,
		},

		// SKU: GWRZR7YT89PF7BKK
		// Instance family: Memory optimized
		// Storage: 1 x 960 SSD
		{
			Name:     "x1e.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      999424,
			VirtType: &hvm,
			Cost:     6672,
		},

		// SKU: J9GRJ3EMVAA899AY
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.large",
			Arches:   arm64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     51,
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
			Name:       "m3.medium",
			Arches:     amd64,
			CpuCores:   1,
			CpuPower:   instances.CpuPower(350),
			Mem:        3840,
			VirtType:   &hvm,
			Cost:       67,
			Deprecated: true,
		},

		// SKU: JNZ6ESS4AS6RFUAF
		// Instance family: Memory optimized
		// Storage: 1 x 320 SSD
		{
			Name:       "r3.4xlarge",
			Arches:     amd64,
			CpuCores:   16,
			CpuPower:   instances.CpuPower(5600),
			Mem:        124928,
			VirtType:   &hvm,
			Cost:       1330,
			Deprecated: true,
		},

		// SKU: JW3ZQ3H2DQQMV2CG
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: JWYS2EA6V4FCHWAC
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3072,
		},

		// SKU: JX2CQH9B6M7D8GM4
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1376,
		},

		// SKU: JZMQ5VQSSPBWHNHH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     83,
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

		// SKU: K2QS6REWTXJ3533K
		// Instance family: Memory optimized
		// Storage: 2 x 1,920 SSD
		{
			Name:     "x1e.32xlarge",
			Arches:   amd64,
			CpuCores: 128,
			CpuPower: instances.CpuPower(41216),
			Mem:      3997696,
			VirtType: &hvm,
			Cost:     26688,
		},

		// SKU: K58CDEEMP6X7SHP3
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.medium",
			Arches:   arm64,
			CpuCores: 1,
			CpuPower: instances.CpuPower(322),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     25,
		},

		// SKU: K7FNMEK8WF6ZAS8V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.medium",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      4096,
			VirtType: &hvm,
			Cost:     37,
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

		// SKU: KQ7QPESHANBU3WAH
		// Instance family: General purpose
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "m5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1808,
		},

		// SKU: MBANS55WTSZ5HYS8
		// Instance family: Memory optimized
		// Storage: 1 x 80 SSD
		{
			Name:       "r3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        31232,
			VirtType:   &hvm,
			Cost:       333,
			Deprecated: true,
		},

		// SKU: MBRCHM79KBVURRUY
		// Instance family: Storage optimized
		// Storage: 2 x 2500 NVMe SSD
		{
			Name:     "i3en.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3472),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: MDBYMKCRTSYYGQQW
		// Instance family: Compute optimized
		// Storage: 1 x 400 NVMe SSD
		{
			Name:     "c5d.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: MJK9CJ3F2VX6S7YX
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     5424,
		},

		// SKU: MNG2Y3YRJK7GPKQR
		// Instance family: Compute optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "c3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(12543),
			Mem:        61440,
			VirtType:   &hvm,
			Cost:       1680,
			Deprecated: true,
		},

		// SKU: MQ37E3QFKSA6FUE5
		// Instance family: Memory optimized
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "r5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     524,
		},

		// SKU: MUQHV9B4DJTH3VEW
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     172,
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
			Cost:     11,
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

		// SKU: N794DC2AB36SPTS3
		// Instance family: Compute optimized
		// Storage: 1 x 100 NVMe SSD
		{
			Name:     "c5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: NA6BZ2FSPKCZGWTA
		// Instance family: Memory optimized
		// Storage: 2 x 320 SSD
		{
			Name:       "r3.8xlarge",
			Arches:     amd64,
			CpuCores:   32,
			CpuPower:   instances.CpuPower(11200),
			Mem:        249856,
			VirtType:   &hvm,
			Cost:       2660,
			Deprecated: true,
		},

		// SKU: NK72MQAHKR4TXF5P
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "r5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     131,
		},

		// SKU: NVDB8R922EEYY7MN
		// Instance family: General purpose
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "m5ad.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     103,
		},

		// SKU: NWH5GFN4NXG2UJUN
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.metal",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(7200),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: P4P583X6BJDDEQX3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2016,
		},

		// SKU: P6RKAKGY8EGB66UX
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     252,
		},

		// SKU: P8H7AGQFY8PUFPWE
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(20159),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     2040,
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

		// SKU: PFVGFQEV3RKWC2MV
		// Instance family: FPGA Instances
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

		// SKU: PKYFKE23C9DJNMGM
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1679),
			Mem:      10752,
			VirtType: &hvm,
			Cost:     216,
		},

		// SKU: PS6J5UCPGRFJTMMR
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     384,
		},

		// SKU: PWCUVRQBX67NDRMJ
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
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
			Name:       "g2.2xlarge",
			Arches:     amd64,
			CpuCores:   8,
			CpuPower:   instances.CpuPower(2911),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       650,
			Deprecated: true,
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

		// SKU: QC9ZVHP4HYUKM2JQ
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     9,
		},

		// SKU: QPQXK5JH8DB8BHCQ
		// Instance family: Compute optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "c5d.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3456,
		},

		// SKU: QRRR442X3JDA8KED
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: QS3PAVP8YYR6DTG5
		// Instance family: General purpose
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "m5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3616,
		},

		// SKU: R3GQ2N4B24GTY3EP
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(6719),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     680,
		},

		// SKU: RM9T78WC38RPDZUZ
		// Instance family: FPGA Instances
		// Storage: 1 x 940 GB
		{
			Name:     "f1.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(1600),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     3300,
		},

		// SKU: RPV38M7F4AHYVV74
		// Instance family: Storage optimized
		// Storage: 4 x 1900 NVMe SSD
		{
			Name:     "i3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     2496,
		},

		// SKU: RTGNGZVHF4N93H5K
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
		},

		// SKU: RZ9FPRZHKGMYG5N2
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     768,
		},

		// SKU: S5PT49TFFKAHWJPW
		// Instance family: Memory optimized
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "r5d.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6912,
		},

		// SKU: SNVTAJX5XNM27CDF
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(11200),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     1536,
		},

		// SKU: SNZHJD4DAJURWJUD
		// Instance family: Memory optimized
		// Storage: 1 x 450 NVMe SSD
		{
			Name:     "z1d.3xlarge",
			Arches:   amd64,
			CpuCores: 12,
			CpuPower: instances.CpuPower(1200),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1116,
		},

		// SKU: SP737G42VWYF8NZP
		// Instance family: Memory optimized
		// Storage: 1 x 120 SSD
		{
			Name:     "x1e.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      124928,
			VirtType: &hvm,
			Cost:     834,
		},

		// SKU: STZEHNY6ZN8EZPY7
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(800),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     504,
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

		// SKU: T4GZYF2JVPASTYU9
		// Instance family: GPU instance
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "p3dn.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     31212,
		},

		// SKU: T5CRJD6MS45TWRHG
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     192,
		},

		// SKU: TBZPVZATPD4UN2Z6
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4032,
		},

		// SKU: THDUZ287XBUHJ46X
		// Instance family: Storage optimized
		// Storage: 1 x 2500 NVMe SSD
		{
			Name:     "i3en.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1736),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     452,
		},

		// SKU: TKXS4QQS37X8H3UH
		// Instance family: Memory optimized
		// Storage: 2 x 600 NVMe SSD
		{
			Name:     "r5d.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(3200),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     2304,
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

		// SKU: TUN3WMC6J3UEQZDJ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(839),
			Mem:      5376,
			VirtType: &hvm,
			Cost:     108,
		},

		// SKU: U84DKE9BVHZNGXZ9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.18xlarge",
			Arches:   amd64,
			CpuCores: 72,
			CpuPower: instances.CpuPower(30239),
			Mem:      147456,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: U8764UF39AJFC7WP
		// Instance family: General purpose
		// Storage: 2 x 300 NVMe SSD
		{
			Name:     "m5ad.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      65536,
			VirtType: &hvm,
			Cost:     824,
		},

		// SKU: UBAMBGAXGXA57QZZ
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(3359),
			Mem:      21504,
			VirtType: &hvm,
			Cost:     432,
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
			Name:       "m3.xlarge",
			Arches:     amd64,
			CpuCores:   4,
			CpuPower:   instances.CpuPower(1400),
			Mem:        15360,
			VirtType:   &hvm,
			Cost:       266,
			Deprecated: true,
		},

		// SKU: VCDUFK62SH3APF2V
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     344,
		},

		// SKU: VCN9BB3M49B23Y77
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      8192,
			VirtType: &hvm,
			Cost:     96,
		},

		// SKU: VDFCZCR5DU63TGT5
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.4xlarge",
			Arches:   amd64,
			CpuCores: 16,
			CpuPower: instances.CpuPower(5600),
			Mem:      131072,
			VirtType: &hvm,
			Cost:     904,
		},

		// SKU: VGES4BGTHYXWPS9W
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      73728,
			VirtType: &hvm,
			Cost:     1530,
		},

		// SKU: VJNS2RY9CFHRVCMH
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3.micro",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(200),
			Mem:      1024,
			VirtType: &hvm,
			Cost:     10,
		},

		// SKU: VKY6VUTPQMWU8Y7T
		// Instance family: General purpose
		// Storage: 1 x 300 NVMe SSD
		{
			Name:     "m5ad.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     412,
		},

		// SKU: VRS2R5SD4VZWNGU3
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5a.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(22400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     3616,
		},

		// SKU: VZTNVQSBAXPKRHEH
		// Instance family: Memory optimized
		// Storage: 4 x 600 NVMe SSD
		{
			Name:     "r5d.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(6400),
			Mem:      524288,
			VirtType: &hvm,
			Cost:     4608,
		},

		// SKU: W4WYWEGHXCH4WAQ7
		// Instance family: Storage optimized
		// Storage: 1 x 475 NVMe SSD
		{
			Name:     "i3.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(644),
			Mem:      15616,
			VirtType: &hvm,
			Cost:     156,
		},

		// SKU: W6JYXGAFAS6XJ4RM
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2800),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     300,
		},

		// SKU: W7BV4NSVS5CF3H46
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "a1.2xlarge",
			Arches:   arm64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     204,
		},

		// SKU: W7ZAHM22QUWG5UJ8
		// Instance family: Memory optimized
		// Storage: 1 x 75 NVMe SSD
		{
			Name:     "z1d.large",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(1120),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     186,
		},

		// SKU: W8ZNTJWD5WJ8H7Y5
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     24480,
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
			Cost:     92,
		},

		// SKU: WNMKFPF6ZP35B7R9
		// Instance family: FPGA Instances
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

		// SKU: WYK3PZV2GCCGKW9W
		// Instance family: Memory optimized
		// Storage: EBS only
		{
			Name:     "r5.metal",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(9600),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     6048,
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

		// SKU: XDFNQQ7DHVVZ65TS
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "z1d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(4800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4464,
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

		// SKU: XSETYH5H5TNQUMHD
		// Instance family: Memory optimized
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "r5ad.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     3144,
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

		// SKU: XX2924KGDDYJG8AK
		// Instance family: General purpose
		// Storage: 4 x 900 NVMe SSD
		{
			Name:     "m5ad.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4944,
		},

		// SKU: Y675W6N6KNZ9J8HC
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(40319),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     4080,
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

		// SKU: YFSTE8F858SQQS3H
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "g3s.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     750,
		},

		// SKU: YKQ7DN6CCEDVB8Q2
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     900,
		},

		// SKU: YMM296RVWEZ6VXXA
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.small",
			Arches:   amd64,
			CpuCores: 2,
			CpuPower: instances.CpuPower(700),
			Mem:      2048,
			VirtType: &hvm,
			Cost:     18,
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

		// SKU: YRSM3RSFVAS4TERE
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "t3a.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1400),
			Mem:      16384,
			VirtType: &hvm,
			Cost:     150,
		},

		// SKU: YT7P3Q75RMN2RX4J
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p2.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      786432,
			VirtType: &hvm,
			Cost:     14400,
		},

		// SKU: YUQP4WPPCTB3CCAZ
		// Instance family: General purpose
		// Storage: 2 x 900 NVMe SSD
		{
			Name:     "m5d.12xlarge",
			Arches:   amd64,
			CpuCores: 48,
			CpuPower: instances.CpuPower(16800),
			Mem:      196608,
			VirtType: &hvm,
			Cost:     2712,
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

		// SKU: YZDG4WDMMU2N5FFC
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.8xlarge",
			Arches:   amd64,
			CpuCores: 32,
			CpuPower: instances.CpuPower(10304),
			Mem:      249856,
			VirtType: &hvm,
			Cost:     12240,
		},

		// SKU: YZN6FRZW8JHKE3HV
		// Instance family: Memory optimized
		// Storage: 1 x 1920 SSD
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
			Cost:     185,
		},

		// SKU: Z4GEHCCXPCNYSMP9
		// Instance family: Compute optimized
		// Storage: EBS only
		{
			Name:     "c5n.9xlarge",
			Arches:   amd64,
			CpuCores: 36,
			CpuPower: instances.CpuPower(15119),
			Mem:      98304,
			VirtType: &hvm,
			Cost:     1944,
		},

		// SKU: Z5V5GD3UGX5Q3YME
		// Instance family: Storage optimized
		// Storage: 8 x 1900 NVMe SSD
		{
			Name:     "i3.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      499712,
			VirtType: &hvm,
			Cost:     4992,
		},

		// SKU: ZACBBPKVJ95ZZFY8
		// Instance family: GPU instance
		// Storage: EBS only
		{
			Name:     "p3.2xlarge",
			Arches:   amd64,
			CpuCores: 8,
			CpuPower: instances.CpuPower(2576),
			Mem:      62464,
			VirtType: &hvm,
			Cost:     3060,
		},

		// SKU: ZAE2E4VBG3J85K5W
		// Instance family: Storage optimized
		// Storage: 8 x 2000 HDD
		{
			Name:     "h1.16xlarge",
			Arches:   amd64,
			CpuCores: 64,
			CpuPower: instances.CpuPower(20608),
			Mem:      262144,
			VirtType: &hvm,
			Cost:     3744,
		},

		// SKU: ZEV4AYQ3MP2J7KQH
		// Instance family: Memory optimized
		// Storage: 1 x 150 NVMe SSD
		{
			Name:     "r5d.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(400),
			Mem:      32768,
			VirtType: &hvm,
			Cost:     288,
		},

		// SKU: ZJZ9FZDG4RFMC5DU
		// Instance family: Storage optimized
		// Storage: 1 x 950 NVMe SSD
		{
			Name:     "i3.xlarge",
			Arches:   amd64,
			CpuCores: 4,
			CpuPower: instances.CpuPower(1288),
			Mem:      31232,
			VirtType: &hvm,
			Cost:     312,
		},

		// SKU: ZKT3KYSUA2YG86U9
		// Instance family: General purpose
		// Storage: EBS only
		{
			Name:     "m5a.24xlarge",
			Arches:   amd64,
			CpuCores: 96,
			CpuPower: instances.CpuPower(33600),
			Mem:      393216,
			VirtType: &hvm,
			Cost:     4128,
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
