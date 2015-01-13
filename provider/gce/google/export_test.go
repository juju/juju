// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/utils"
)

var (
	NewRawConnection    = &newRawConnection
	DoCall              = &doCall
	AddInstance         = &addInstance
	RawInstance         = &rawInstance
	InstsNextPage       = &instsNextPage
	ConnRemoveFirewall  = &connRemoveFirewall
	PConnRemoveInstance = &connRemoveInstance

	ConnRemoveInstance = connRemoveInstance
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

func SetInstanceSpec(inst *Instance, spec *InstanceSpec) {
	inst.spec = spec
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

func ConnInstance(conn *Connection, zone, id string) (*compute.Instance, error) {
	return conn.instance(zone, id)
}

func ConnAddInstance(conn *Connection, inst *compute.Instance, typ string, zones []string) error {
	return conn.addInstance(inst, typ, zones)
}

func InstanceSpecRaw(spec InstanceSpec) *compute.Instance {
	return spec.raw()
}

func SetQuickAttemptStrategy(s *BaseSuite) {
	s.PatchValue(&attemptsLong, utils.AttemptStrategy{})
	s.PatchValue(&attemptsShort, utils.AttemptStrategy{})
}
