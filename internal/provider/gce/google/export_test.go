// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/collections/set"
	"google.golang.org/api/compute/v1"
)

var (
	NewService = &newService

	NewInstanceRaw      = newInstance
	PackMetadata        = packMetadata
	UnpackMetadata      = unpackMetadata
	FormatMachineType   = formatMachineType
	FirewallSpec        = firewallSpec
	ExtractAddresses    = extractAddresses
	NewRuleSetFromRules = newRuleSetFromRules
	MatchesPrefix       = matchesPrefix
)

func SetRawConn(conn *Connection, svc service) {
	conn.service = svc
}

func ExposeRawService(conn *Connection) *compute.Service {
	return conn.service.(*rawConn).Service
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

func NewNetInterface(spec NetworkSpec, name string, allocatePublicIP bool) *compute.NetworkInterface {
	return spec.newInterface(name, allocatePublicIP)
}

func ConnAddInstance(conn *Connection, inst *compute.Instance, mtype string, zone string) error {
	return conn.addInstance(inst, mtype, zone)
}

func ConnRemoveInstance(conn *Connection, id, zone string) error {
	return conn.removeInstance(id, zone)
}

func HashSuffixNamer(fw *firewall, prefix string, _ set.Strings) (string, error) {
	if len(fw.SourceCIDRs) == 0 || len(fw.SourceCIDRs) == 1 && fw.SourceCIDRs[0] == "0.0.0.0/0" {
		return prefix, nil
	}
	return prefix + "-" + sourcecidrs(fw.SourceCIDRs).key(), nil
}
