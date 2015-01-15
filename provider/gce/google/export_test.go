// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
)

var (
	NewRawConnection = &newRawConnection

	NewInstance        = newInstance
	PackMetadata       = packMetadata
	UnpackMetadata     = unpackMetadata
	ResolveMachineType = resolveMachineType
	RootDisk           = rootDisk
	DiskSizeGB         = diskSizeGB
	ZoneName           = zoneName
	FirewallSpec       = firewallSpec
	ExtractAddresses   = extractAddresses
)

func SetRawConn(conn *Connection, raw rawConnectionWrapper) {
	conn.raw = raw
}

func ExposeRawService(conn *Connection) *compute.Service {
	return conn.raw.(*rawConn).Service
}

func NewAttached(spec DiskSpec) *compute.AttachedDisk {
	return spec.newAttached()
}

func NewAvailabilityZone(zone *compute.Zone) AvailabilityZone {
	return AvailabilityZone{zone: zone}
}

// TODO(ericsnow) Elimiinate this.
func SetInstanceSpec(inst *Instance, spec *InstanceSpec) {
	inst.spec = spec
}

func NewNetInterface(spec NetworkSpec, name string) *compute.NetworkInterface {
	return spec.newInterface(name)
}

func InstanceSpecRaw(spec InstanceSpec) *compute.Instance {
	return spec.raw()
}

func ConnAddInstance(conn *Connection, inst *compute.Instance, mtype string, zones []string) error {
	return conn.addInstance(inst, mtype, zones)
}

func ConnRemoveInstance(conn *Connection, id, zone string) error {
	return conn.removeInstance(id, zone)
}
