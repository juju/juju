// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

// DiskProvider provides an interface for creating, destroying and
// describing disks in the environment.
type DiskProvider interface {
	// CreateDisks creates disks with the specified size, in MiB.
	CreateDisks(spec []*DiskParams) ([]*BlockDevice, error)

	// DescribeDisks returns the properties of the disks with the
	// specified provider disk IDs.
	DescribeDisks(diskIds []string) ([]*BlockDevice, error)

	// DestroyDisks destroys the disks with the specified provider
	// disk IDs.
	DestroyDisks(diskIds []string) error

	// ListDisks returns provider disk IDs for each disk in the pool.
	ListDisks() ([]string, error)

	// ValidateDiskParams validates the provided disk parameters,
	// returning an error if they are invalid.
	ValidateDiskParams(params DiskParams) error

	//AttachDisks(diskIds []string, instId []instance.Id) error
	//DetachDisks(diskIds []string, instId []instance.Id) error
}

// DiskParams is a fully specified set of parameters for disk creation,
// derived from one or more of user-specified storage constraints, a
// storage pool definition, and charm storage metadata.
type DiskParams struct {
	// Size is the minimum size of the disk in MiB.
	Size uint64

	// Options is a set of provider-specific options for storage creation,
	// as defined in a storage pool.
	Options map[string]interface{}
}
