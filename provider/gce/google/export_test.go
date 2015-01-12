// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
)

var (
	DoCall             = &doCall
	AddInstance        = &addInstance
	NewInstance        = newInstance
	FilterInstances    = filterInstances
	CheckInstStatus    = checkInstStatus
	PackMetadata       = packMetadata
	UnpackMetadata     = unpackMetadata
	ResolveMachineType = resolveMachineType
	RootDisk           = rootDisk
	DiskSizeGB         = diskSizeGB
	ZoneName           = zoneName
	FirewallSpec       = firewallSpec
)

func NewAttached(spec DiskSpec) *compute.AttachedDisk {
	return spec.newAttached()
}

func NewAvailabilityZone(zone *compute.Zone) AvailabilityZone {
	return AvailabilityZone{zone: zone}
}

func ExposeRawInstance(inst *Instance) *compute.Instance {
	return &inst.raw
}

func NewNetInterface(spec NetworkSpec, name string) *compute.NetworkInterface {
	return spec.newInterface(name)
}
