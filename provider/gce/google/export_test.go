// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/utils"
)

var (
	NewRawConnection   = &newRawConnection
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

func ExposeRawService(conn *Connection) *compute.Service {
	return conn.raw
}

func SetRawService(conn *Connection, service *compute.Service) {
	conn.raw = service
}

func CheckOperation(conn *Connection, op *compute.Operation) (*compute.Operation, error) {
	return conn.checkOperation(op)
}

func WaitOperation(conn *Connection, op *compute.Operation, attempts utils.AttemptStrategy) error {
	return conn.waitOperation(op, attempts)
}
