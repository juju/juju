// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
)

// Constraints represents the application constraints.
// All fields of this struct are taken from core/constraints except for the
// spaces, which are modeled with SpaceConstraint to take into account the
// negative constraint (excluded field in the db).
type Constraints struct {
	// Arch, if not nil or empty, indicates that a machine must run the named
	// architecture.
	Arch *string

	// Container, if not nil, indicates that a machine must be the specified container type.
	Container *instance.ContainerType

	// CpuCores, if not nil, indicates that a machine must have at least that
	// number of effective cores available.
	CpuCores *uint64

	// CpuPower, if not nil, indicates that a machine must have at least that
	// amount of CPU power available, where 100 CpuPower is considered to be
	// equivalent to 1 Amazon ECU (or, roughly, a single 2007-era Xeon).
	CpuPower *uint64

	// Mem, if not nil, indicates that a machine must have at least that many
	// megabytes of RAM.
	Mem *uint64

	// RootDisk, if not nil, indicates that a machine must have at least
	// that many megabytes of disk space available in the root disk. In
	// providers where the root disk is configurable at instance startup
	// time, an instance with the specified amount of disk space in the OS
	// disk might be requested.
	RootDisk *uint64

	// RootDiskSource, if specified, determines what storage the root
	// disk should be allocated from. This will be provider specific -
	// in the case of vSphere it identifies the datastore the root
	// disk file should be created in.
	RootDiskSource *string

	// Tags, if not nil, indicates tags that the machine must have applied to it.
	// An empty list is treated the same as a nil (unspecified) list, except an
	// empty list will override any default tags, where a nil list will not.
	Tags *[]string

	// InstanceRole, if not nil, indicates that the specified role/profile for
	// the given cloud should be used. Only valid for clouds which support
	// instance roles. Currently only for AWS with instance-profiles
	InstanceRole *string

	// InstanceType, if not nil, indicates that the specified cloud instance type
	// be used. Only valid for clouds which support instance types.
	InstanceType *string

	// Spaces, if not nil, holds a list of juju network spaces that
	// should be available (or not) on the machine.
	Spaces *[]SpaceConstraint

	// VirtType, if not nil or empty, indicates that a machine must run the named
	// virtual type. Only valid for clouds with multi-hypervisor support.
	VirtType *string

	// Zones, if not nil, holds a list of availability zones limiting where
	// the machine can be located.
	Zones *[]string

	// AllocatePublicIP, if nil or true, signals that machines should be
	// created with a public IP address instead of a cloud local one.
	// The default behaviour if the value is not specified is to allocate
	// a public IP so that public cloud behaviour works out of the box.
	AllocatePublicIP *bool

	// ImageID, if not nil, indicates that a machine must use the specified
	// image. This is provider specific, and for the moment is only
	// implemented on MAAS clouds.
	ImageID *string
}

// SpaceConstraint represents a single space constraint for an application.
type SpaceConstraint struct {
	// Excluded indicates that this space should not be available to the
	// machine.
	Exclude bool

	// SpaceName is the name of the space.
	SpaceName string
}

// FromCoreConstraints is responsible for converting a [constraints.Value] to a
// [Constraints] object.
func FromCoreConstraints(coreCons constraints.Value) Constraints {
	rval := Constraints{
		Arch:             coreCons.Arch,
		Container:        coreCons.Container,
		CpuCores:         coreCons.CpuCores,
		CpuPower:         coreCons.CpuPower,
		Mem:              coreCons.Mem,
		RootDisk:         coreCons.RootDisk,
		RootDiskSource:   coreCons.RootDiskSource,
		Tags:             coreCons.Tags,
		InstanceRole:     coreCons.InstanceRole,
		InstanceType:     coreCons.InstanceType,
		VirtType:         coreCons.VirtType,
		Zones:            coreCons.Zones,
		AllocatePublicIP: coreCons.AllocatePublicIP,
		ImageID:          coreCons.ImageID,
	}

	if coreCons.Spaces == nil {
		return rval
	}

	spaces := make([]SpaceConstraint, 0, len(*coreCons.Spaces))
	// Set included spaces
	for _, incSpace := range coreCons.IncludeSpaces() {
		spaces = append(spaces, SpaceConstraint{
			SpaceName: incSpace,
			Exclude:   false,
		})
	}

	// Set excluded spaces
	for _, exSpace := range coreCons.ExcludeSpaces() {
		spaces = append(spaces, SpaceConstraint{
			SpaceName: exSpace,
			Exclude:   true,
		})
	}
	rval.Spaces = &spaces

	return rval
}

// ToCoreConstraints is responsible for converting a [Constraints] value to a
// [constraints.Value].
func ToCoreConstraints(cons Constraints) constraints.Value {
	rval := constraints.Value{
		Arch:             cons.Arch,
		Container:        cons.Container,
		CpuCores:         cons.CpuCores,
		CpuPower:         cons.CpuPower,
		Mem:              cons.Mem,
		RootDisk:         cons.RootDisk,
		RootDiskSource:   cons.RootDiskSource,
		Tags:             cons.Tags,
		InstanceRole:     cons.InstanceRole,
		InstanceType:     cons.InstanceType,
		VirtType:         cons.VirtType,
		Zones:            cons.Zones,
		AllocatePublicIP: cons.AllocatePublicIP,
		ImageID:          cons.ImageID,
	}

	if cons.Spaces == nil {
		return rval
	}

	for _, space := range *cons.Spaces {
		rval.AddSpace(space.SpaceName, space.Exclude)
	}

	return rval
}
