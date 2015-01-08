// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"path"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/network"
)

const (
	diskTypeScratch    = "SCRATCH"
	diskTypePersistent = "PERSISTENT"
	diskModeRW         = "READ_WRITE"
	diskModeRO         = "READ_ONLY"

	networkDefaultName       = "default"
	networkPathRoot          = "global/networks/"
	networkAccessOneToOneNAT = "ONE_TO_ONE_NAT"

	StatusDone         = "DONE"
	StatusDown         = "DOWN"
	StatusPending      = "PENDING"
	StatusProvisioning = "PROVISIONING"
	StatusRunning      = "RUNNING"
	StatusStaging      = "STAGING"
	StatusStopped      = "STOPPED"
	StatusStopping     = "STOPPING"
	StatusTerminated   = "TERMINATED"
	StatusUp           = "UP"

	// MinDiskSize is the minimum/default size (in megabytes) for GCE
	// disks. GCE does not currently have a minimum disk size.
	MinDiskSizeGB int64 = 0
)

var (
	logger = loggo.GetLogger("juju.provider.gce.gceapi")
)

type DiskSpec struct {
	// sizeHint is the requested disk size in Gigabytes.
	SizeHintGB int64
	ImageURL   string
	Boot       bool
	Scratch    bool
	Readonly   bool
	AutoDelete bool
}

func (ds *DiskSpec) TooSmall() bool {
	return ds.SizeHintGB < MinDiskSizeGB
}

func (ds *DiskSpec) SizeGB() int64 {
	size := ds.SizeHintGB
	if ds.TooSmall() {
		size = MinDiskSizeGB
	}
	return size
}

func (ds *DiskSpec) newAttached() *compute.AttachedDisk {
	diskType := diskTypePersistent // The default.
	if ds.Scratch {
		diskType = diskTypeScratch
	}
	mode := diskModeRW // The default.
	if ds.Readonly {
		mode = diskModeRO
	}

	disk := compute.AttachedDisk{
		Type:       diskType,
		Boot:       ds.Boot,
		Mode:       mode,
		AutoDelete: ds.AutoDelete,
		InitializeParams: &compute.AttachedDiskInitializeParams{
			// DiskName (defaults to instance name)
			DiskSizeGb: ds.SizeGB(),
			// DiskType (defaults to pd-standard, pd-ssd, local-ssd)
			SourceImage: ds.ImageURL,
		},
		// Interface (defaults to SCSI)
		// DeviceName (GCE sets this, persistent disk only)
	}
	return &disk
}

func rootDisk(inst interface{}) *compute.AttachedDisk {
	switch typed := inst.(type) {
	case *compute.Instance:
		return typed.Disks[0]
	case *Instance:
		if typed.spec == nil {
			return nil
		}
		return typed.spec.Disks[0].newAttached()
	case *InstanceSpec:
		return typed.Disks[0].newAttached()
	default:
		return nil
	}
}

func diskSizeGB(disk interface{}) (int64, error) {
	switch typed := disk.(type) {
	case *compute.Disk:
		return typed.SizeGb, nil
	case *compute.AttachedDisk:
		if typed.InitializeParams == nil {
			return 0, errors.Errorf("attached disk missing init params: %v", disk)
		}
		return typed.InitializeParams.DiskSizeGb, nil
	default:
		return 0, errors.Errorf("disk has unrecognized type: %v", disk)
	}
}

func zoneName(value interface{}) string {
	// We trust that path.Base will always give the right answer
	// when used.
	switch typed := value.(type) {
	case *compute.Instance:
		return path.Base(typed.Zone)
	case *compute.Operation:
		return path.Base(typed.Zone)
	default:
		// TODO(ericsnow) Fail?
		return ""
	}
}

type NetworkSpec struct {
	Name string
	// TODO(ericsnow) support a CIDR for internal IP addr range?
}

func (ns *NetworkSpec) path() string {
	name := ns.Name
	if name == "" {
		name = networkDefaultName
	}
	return networkPathRoot + name
}

func (ns *NetworkSpec) newInterface(name string) *compute.NetworkInterface {
	var access []*compute.AccessConfig
	if name != "" {
		// This interface has an internet connection.
		access = append(access, &compute.AccessConfig{
			Name: name,
			Type: networkAccessOneToOneNAT, // the default
			// NatIP (only set if using a reserved public IP)
		})
		// TODO(ericsnow) Will we need to support more access configs?
	}
	return &compute.NetworkInterface{
		Network:       ns.path(),
		AccessConfigs: access,
	}
}

// firewallSpec expands a port range set in to compute.FirewallAllowed
// and returns a compute.Firewall for the provided name.
func firewallSpec(name string, ps network.PortSet) *compute.Firewall {
	firewall := compute.Firewall{
		// Allowed is set below.
		// Description is not set.
		Name: name,
		// Network: (defaults to global)
		// SourceTags is not set.
		TargetTags:   []string{name},
		SourceRanges: []string{"0.0.0.0/0"},
	}

	for _, protocol := range ps.Protocols() {
		allowed := compute.FirewallAllowed{
			IPProtocol: protocol,
			Ports:      ps.PortStrings(protocol),
		}
		firewall.Allowed = append(firewall.Allowed, &allowed)
	}
	return &firewall
}
