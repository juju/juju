// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

// MachineType represents a gce Machine Type.
// this is basically a copy of compute.MachineType put here to
// satisfy an extra layer of abstraction.
type MachineType struct {
	CreationTimestamp            string
	Deprecated                   bool
	Description                  string
	GuestCpus                    int64
	Id                           uint64
	ImageSpaceGb                 int64
	Kind                         string
	MaximumPersistentDisks       int64
	MaximumPersistentDisksSizeGb int64
	MemoryMb                     int64
	Name                         string
}
