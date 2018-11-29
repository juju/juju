// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

// ShapeSpec holds information about a shapes resource allocation
type ShapeSpec struct {
	// Cpus is the number of CPU cores available to the instance
	Cpus int
	// Gpus is the number of GPUs available to this instance
	Gpus int
	// Memory is the amount of RAM available to the instance in MB
	Memory int
	// Bandwidth is the network bandwidth in Gbps. Where there are multiple physical NICs, the speed of the fastest is used.
	Bandwidth float32
	Type      InstanceType
	Tags      []string
}

// shapeSpecs is a map containing resource information
// about each shape. Unfortunately the API simply returns
// the name of the shape and nothing else. For details see:
// https://docs.cloud.oracle.com/iaas/Content/Compute/References/computeshapes.htm
// https://cloud.oracle.com/infrastructure/pricing
// https://cloud.oracle.com/infrastructure/compute/pricing
var shapeSpecs = map[string]ShapeSpec{
	"VM.Standard1.1": {
		Cpus:      1,
		Memory:    7 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 0.599, // "Up to 600 Mbps"
	},
	"VM.Standard1.2": {
		Cpus:      2,
		Memory:    14 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 1.199, // "Up to 1.2 Gbps"
	},
	"VM.Standard1.4": {
		Cpus:      4,
		Memory:    28 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 1.2,
	},
	"VM.Standard1.8": {
		Cpus:      8,
		Memory:    56 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 2.4,
	},
	"VM.Standard1.16": {
		Cpus:      16,
		Memory:    112 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 4.8,
	},
	"VM.Standard2.1": {
		Cpus:      1,
		Memory:    15 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 1,
	},
	"VM.Standard2.2": {
		Cpus:      2,
		Memory:    30 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 2,
	},
	"VM.Standard2.4": {
		Cpus:      4,
		Memory:    60 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 4.1,
	},
	"VM.Standard2.8": {
		Cpus:      8,
		Memory:    120 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 8.2,
	},
	"VM.Standard2.16": {
		Cpus:      16,
		Memory:    240 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 16.4,
	},
	"VM.Standard2.24": {
		Cpus:      24,
		Memory:    320 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 24.6,
	},
	"VM.DenseIO1.4": {
		Cpus:   4,
		Memory: 60 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 1.2,
	},
	"VM.DenseIO1.8": {
		Cpus:   8,
		Memory: 120 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 2.4,
	},
	"VM.DenseIO1.16": {
		Cpus:   16,
		Memory: 240 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 4.8,
	},
	"VM.DenseIO2.8": {
		Cpus:   8,
		Memory: 120 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 8.2,
	},
	"VM.DenseIO2.16": {
		Cpus:   16,
		Memory: 240 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 16.4,
	},
	"VM.DenseIO2.24": {
		Cpus:   24,
		Memory: 320 * 1024,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 24.6,
	},
	"VM.Standard.E2.1": {
		Cpus:      1,
		Memory:    8 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 0.7,
	},
	"VM.Standard.E2.2": {
		Cpus:      2,
		Memory:    16 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 1.4,
	},
	"VM.Standard.E2.4": {
		Cpus:      4,
		Memory:    32 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 2.8,
	},
	"VM.Standard.E2.8": {
		Cpus:      8,
		Memory:    64 * 1024,
		Type:      VirtualMachine,
		Bandwidth: 5.6,
	},
	"VM.GPU2.1": {
		Cpus:      12,
		Gpus:      1,
		Memory:    72 * 1024,
		Type:      GPUMachine,
		Bandwidth: 8.0,
		Tags: []string{
			"nvidia_p100",
		},
	},
	"VM.GPU3.1": {
		Cpus:      6,
		Gpus:      1,
		Memory:    90 * 1024,
		Type:      GPUMachine,
		Bandwidth: 6.0,
		Tags: []string{
			"nvidia_v100",
		},
	},
	"VM.GPU3.2": {
		Cpus:      12,
		Gpus:      2,
		Memory:    180 * 1024,
		Type:      GPUMachine,
		Bandwidth: 8.0,
		Tags: []string{
			"nvidia_v100",
		},
	},
	"VM.GPU3.4": {
		Cpus:      24,
		Gpus:      4,
		Memory:    360 * 1024,
		Type:      GPUMachine,
		Bandwidth: 8.0,
		Tags: []string{
			"nvidia_v100",
		},
	},
	"BM.Standard1.36": {
		Cpus:      36,
		Memory:    256 * 1024,
		Type:      BareMetal,
		Bandwidth: 10.0,
	},
	"BM.Standard2.52": {
		Cpus:      52,
		Memory:    768 * 1024,
		Type:      BareMetal,
		Bandwidth: 25.0,
	},
	"BM.DenseIO1.36": {
		Cpus:      36,
		Memory:    512 * 1024,
		Type:      BareMetal,
		Bandwidth: 10.0,
		Tags: []string{
			"denseio",
		},
	},
	"BM.DenseIO2.52": {
		Cpus:   52,
		Memory: 768 * 1024,
		Type:   BareMetal,
		Tags: []string{
			"denseio",
		},
		Bandwidth: 25,
	},
	"BM.GPU2.2": {
		Cpus:      28,
		Gpus:      2,
		Memory:    192 * 1024,
		Type:      GPUMachine,
		Bandwidth: 25,
		Tags: []string{
			"nvidia_p100",
		},
	},
	"BM.GPU3.8": {
		Cpus:      52,
		Gpus:      8,
		Memory:    768 * 1024,
		Type:      GPUMachine,
		Bandwidth: 25,
		Tags: []string{
			"nvidia_v100",
		},
	},
	"BM.Standard.E2.64": {
		Cpus:      64,
		Memory:    512 * 1024,
		Type:      BareMetal,
		Bandwidth: 25.0,
	},
	"BM.HPC2.36": {
		Cpus:      36,
		Memory:    384 * 1024,
		Type:      BareMetal,
		Bandwidth: 25.0,
		Tags: []string{
			"rdma",
		},
	},
}
