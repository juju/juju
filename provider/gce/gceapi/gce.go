// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"fmt"
	"path"
	"strings"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/network"
)

const (
	partialMachineType = "zones/%s/machineTypes/%s"

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

type InstanceSpec struct {
	ID                string
	Type              string
	Disks             []DiskSpec
	Network           NetworkSpec
	NetworkInterfaces []string
	Metadata          map[string]string
	Tags              []string
}

func (is InstanceSpec) Create(conn *Connection, zones []string) (*Instance, error) {
	raw, err := is.create(conn, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	inst := newInstance(raw)
	copied := is
	inst.spec = &copied
	return inst, nil
}

func (is InstanceSpec) create(conn *Connection, zones []string) (*compute.Instance, error) {
	raw := &compute.Instance{
		Name:              is.ID,
		Disks:             is.disks(),
		NetworkInterfaces: is.networkInterfaces(),
		Metadata:          packMetadata(is.Metadata),
		Tags:              &compute.Tags{Items: is.Tags},
		// MachineType is set in the addInstance call.
	}
	err := conn.addInstance(raw, is.Type, zones)
	return raw, errors.Trace(err)
}

func (is InstanceSpec) disks() []*compute.AttachedDisk {
	var result []*compute.AttachedDisk
	for _, spec := range is.Disks {
		result = append(result, spec.newAttached())
	}
	return result
}

func (is InstanceSpec) networkInterfaces() []*compute.NetworkInterface {
	var result []*compute.NetworkInterface
	for _, name := range is.NetworkInterfaces {
		result = append(result, is.Network.newInterface(name))
	}
	return result
}

type Instance struct {
	ID   string
	Zone string
	raw  compute.Instance
	spec *InstanceSpec
}

func newInstance(raw *compute.Instance) *Instance {
	return &Instance{
		ID:   raw.Name,
		Zone: zoneName(raw),
		raw:  *raw,
	}
}

func (gi Instance) RootDiskGB() int64 {
	if gi.spec == nil {
		return 0
	}
	attached := rootDisk(gi.spec)
	// The root disk from a spec will not fail.
	size, _ := diskSizeGB(attached)
	return size
}

func (gi Instance) Status() string {
	return gi.raw.Status
}

func (gi *Instance) Refresh(conn *Connection) error {
	raw, err := conn.instance(gi.Zone, gi.ID)
	if err != nil {
		return errors.Trace(err)
	}

	gi.raw = *raw
	return nil
}

func (gi Instance) Addresses() ([]network.Address, error) {
	var addresses []network.Address

	for _, netif := range gi.raw.NetworkInterfaces {
		// Add public addresses.
		for _, accessConfig := range netif.AccessConfigs {
			if accessConfig.NatIP == "" {
				continue
			}
			address := network.Address{
				Value: accessConfig.NatIP,
				Type:  network.IPv4Address,
				Scope: network.ScopePublic,
			}
			addresses = append(addresses, address)

		}

		// Add private address.
		if netif.NetworkIP == "" {
			continue
		}
		address := network.Address{
			Value: netif.NetworkIP,
			Type:  network.IPv4Address,
			Scope: network.ScopeCloudLocal,
		}
		addresses = append(addresses, address)
	}

	return addresses, nil
}

func (gi Instance) Metadata() map[string]string {
	return unpackMetadata(gi.raw.Metadata)
}

func filterInstances(instances []Instance, statuses ...string) []Instance {
	var results []Instance
	for _, inst := range instances {
		if !checkInstStatus(inst, statuses...) {
			continue
		}
		results = append(results, inst)
	}
	return results
}

func checkInstStatus(inst Instance, statuses ...string) bool {
	for _, status := range statuses {
		if inst.Status() == status {
			return true
		}
	}
	return false
}

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

// FormatAuthorizedKeys returns our authorizedKeys with
// the username prepended to it. This is the format that
// GCE uses for its sshKeys metadata.
func FormatAuthorizedKeys(raw, user string) (string, error) {
	var userKeys string
	users := []string{user}
	keys := strings.Split(raw, "\n")
	for _, key := range keys {
		for _, user := range users {
			userKeys += user + ":" + key + "\n"
		}
	}
	return userKeys, nil
}

func packMetadata(data map[string]string) *compute.Metadata {
	var items []*compute.MetadataItems
	for key, value := range data {
		item := compute.MetadataItems{
			Key:   key,
			Value: value,
		}
		items = append(items, &item)
	}
	return &compute.Metadata{Items: items}
}

func unpackMetadata(data *compute.Metadata) map[string]string {
	if data == nil {
		return nil
	}

	result := make(map[string]string)
	for _, item := range data.Items {
		result[item.Key] = item.Value
	}
	return result
}

func resolveMachineType(zone, name string) string {
	return fmt.Sprintf(partialMachineType, zone, name)
}
