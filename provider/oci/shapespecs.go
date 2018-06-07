// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oci

// ShapeSpec holds information about a shapes resource allocation
type ShapeSpec struct {
	// Cpus is the number of CPU cores available to the instance
	Cpus int
	// Gpus is the number of GPUs available to this instance
	Gpus int
	// Memory is the amount of RAM available to the instance
	Memory int
	Type   InstanceType
	Tags   []string
}

// shapeSpecs is a map containing resource information
// about each shape. Unfortunately the API simply returns
// the name of the shape and nothing else. For details see:
// https://cloud.oracle.com/infrastructure/pricing
// https://cloud.oracle.com/infrastructure/compute/pricing
var shapeSpecs = map[string]ShapeSpec{
	"VM.Standard1.1": {
		Cpus:   1,
		Memory: 7168,
		Type:   VirtualMachine,
	},
	"VM.Standard2.1": {
		Cpus:   1,
		Memory: 15360,
		Type:   VirtualMachine,
	},
	"VM.Standard1.2": {
		Cpus:   2,
		Memory: 14336,
		Type:   VirtualMachine,
	},
	"VM.Standard2.2": {
		Cpus:   2,
		Memory: 30720,
		Type:   VirtualMachine,
	},
	"VM.Standard1.4": {
		Cpus:   4,
		Memory: 28672,
		Type:   VirtualMachine,
	},
	"VM.Standard2.4": {
		Cpus:   4,
		Memory: 61440,
		Type:   VirtualMachine,
	},
	"VM.Standard1.8": {
		Cpus:   8,
		Memory: 57344,
		Type:   VirtualMachine,
	},
	"VM.Standard2.8": {
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
	},
	"VM.Standard1.16": {
		Cpus:   16,
		Memory: 114688,
		Type:   VirtualMachine,
	},
	"VM.Standard2.16": {
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
	},
	"VM.Standard2.24": {
		Cpus:   24,
		Memory: 327680,
		Type:   VirtualMachine,
	},
	"VM.DenseIO1.4": {
		Cpus:   4,
		Memory: 61440,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO1.8": {
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO2.8": {
		Cpus:   8,
		Memory: 122880,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO1.16": {
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},

	"VM.DenseIO2.16": {
		Cpus:   16,
		Memory: 245760,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"VM.DenseIO2.24": {
		Cpus:   24,
		Memory: 327680,
		Type:   VirtualMachine,
		Tags: []string{
			"denseio",
		},
	},
	"BM.Standard1.36": {
		Cpus:   36,
		Memory: 262144,
		Type:   BareMetal,
	},
	"BM.Standard2.52": {
		Cpus:   52,
		Memory: 786432,
		Type:   BareMetal,
	},
	"BM.HighIO1.36": {
		Cpus:   36,
		Memory: 524288,
		Type:   BareMetal,
		Tags: []string{
			"highio",
		},
	},
	"BM.DenseIO1.36": {
		Cpus:   36,
		Memory: 7168,
		Type:   BareMetal,
		Tags: []string{
			"denseio",
		},
	},
	"BM.DenseIO2.52": {
		Cpus:   52,
		Memory: 786432,
		Type:   BareMetal,
		Tags: []string{
			"denseio",
		},
	},
	"BM.GPU2.2": {
		Cpus:   28,
		Gpus:   2,
		Memory: 196608,
		Type:   GPUMachine,
		Tags: []string{
			"denseio",
		},
	},
}
