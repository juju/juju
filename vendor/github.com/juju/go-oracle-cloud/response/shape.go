// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package response

// A shape is a resource profile that specifies the number of OCPUs
// and the amount of memory to be allocated to an instance in Oracle
// Compute Cloud Service. The shape determines the type of disk drive
// that your instance uses. If you select a general purpose or
// high-memory shape, a hard-disk drive is used. If you select a
// high-I/O shape, an NVM Express solid-state drive is used.
// High-I/O shapes also determine the size of the root disk.
type Shape struct {
	// Limits the rate of IO for NDS storage volumes.
	Nds_iops_limit uint64 `json:"nds_iops_limit"`

	// Number of megabytes of memory allocated to instances of this shape.
	Ram uint64 `json:"ram"`

	// Store the root disk image on SSD storage.
	Is_root_ssd bool `json:"is_root_ssd,omitempty"`

	// Size of the local SSD data disk in bytes.
	Ssd_data_size uint64 `json:"ssd_data_size,omitmepty"`

	// Number of gpu devices allocated to instances of this shape
	Gpus uint64 `json:"gpus,omitempty"`

	// Number of CPUs or partial CPUs allocated to instances of this shape.
	Cpus float64 `json:"cpus"`

	// Uniform Resource Identifier of the shape
	Uri string `json:"uri"`

	// Size of the root disk in bytes.
	Root_disk_size uint64 `json:"root_disk_size"`

	// IO share allocated to instances of this shape.
	Io uint64 `json:"io"`

	// Name of this shape.
	Name string `json:"name"`
}

// AllShapes has a slice of all declared shapes in the oracle api
type AllShapes struct {
	Result []Shape `json:"result"`
}
