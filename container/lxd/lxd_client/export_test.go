// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"google.golang.org/api/compute/v1"
)

var (
	NewRawConnection = &newRawConnection

	NewInstanceRaw    = newInstance
	PackMetadata      = packMetadata
	UnpackMetadata    = unpackMetadata
	FormatMachineType = formatMachineType
	FirewallSpec      = firewallSpec
	ExtractAddresses  = extractAddresses
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

func NewDetached(spec DiskSpec) (*compute.Disk, error) {
	return spec.newDetached()
}

func NewAvailabilityZone(zone *compute.Zone) AvailabilityZone {
	return AvailabilityZone{zone: zone}
}

func GetInstanceSpec(inst *Instance) *InstanceSpec {
	return inst.spec
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
