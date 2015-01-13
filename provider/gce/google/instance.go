// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"strings"

	"code.google.com/p/google-api-go-client/compute/v1"
	"github.com/juju/errors"

	"github.com/juju/juju/network"
)

const (
	partialMachineType = "zones/%s/machineTypes/%s"
)

// InstanceSpec holds all the information needed to create a new GCE
// instance within some zone.
// TODO(ericsnow) Validate the invariants?
type InstanceSpec struct {
	// ID is the "name" of the instance.
	ID string
	// Type is the name of the GCE instance type. The value is resolved
	// relative to an availability zone when the API request is sent.
	// The type must match one of the GCE-recognized types.
	Type string
	// Disks holds the information needed to request each of the disks
	// that should be attached to a new instance. This must include a
	// single root disk.
	Disks []DiskSpec
	// Network identifies the information for the network that a new
	// instance should use. If the network does not exist then it will
	// be added when the instance is. At least the network's name must
	// be set.
	Network NetworkSpec
	// NetworkInterfaces is the names of the network interfaces to
	// associate with the instance. They will be connected to the the
	// network identified by the instance spec. At least one name must
	// be provided.
	NetworkInterfaces []string
	// Metadata is the GCE instance "user-specified" metadata that will
	// be initialized on the new instance.
	Metadata map[string]string
	// Tags are the labels to associate with the instance. This is
	// useful when making bulk calls or in relation to some API methods
	// (e.g. related to firewalls access rules).
	Tags []string
}

// Create creates a new instance based on the spec's data and returns it.
// The instance will be created using the provided connection and in one
// of the provided zones.
func (is InstanceSpec) Create(conn *Connection, zones []string) (*Instance, error) {
	raw := is.raw()
	if err := addInstance(conn, raw, is.Type, zones); err != nil {
		return nil, errors.Trace(err)
	}

	inst := newInstance(raw)
	copied := is
	inst.spec = &copied
	return inst, nil
}

func (is InstanceSpec) raw() *compute.Instance {
	return &compute.Instance{
		Name:              is.ID,
		Disks:             is.disks(),
		NetworkInterfaces: is.networkInterfaces(),
		Metadata:          packMetadata(is.Metadata),
		Tags:              &compute.Tags{Items: is.Tags},
		// MachineType is set in the addInstance call.
	}
}

var addInstance = func(conn *Connection, raw *compute.Instance, typ string, zones []string) error {
	return conn.addInstance(raw, typ, zones)
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

// Instance represents a single realized GCE compute instance.
type Instance struct {
	raw  compute.Instance
	spec *InstanceSpec

	// ID is the "name" of the instance.
	ID string
	// Zone is the unqualified name of the zone in which the instance
	// was provisioned.
	Zone string
}

func newInstance(raw *compute.Instance) *Instance {
	return &Instance{
		ID:   raw.Name,
		Zone: zoneName(raw),
		raw:  *raw,
	}
}

// Spec returns the spec that was used to create this instance.
func (gi Instance) Spec() *InstanceSpec {
	return gi.spec
}

// RootDiskGB returns the size of the instance's root disk. If it
// cannot be determined then 0 is returned.
func (gi Instance) RootDiskGB() int64 {
	if gi.spec == nil {
		return 0
	}
	attached := rootDisk(gi.spec)
	// The root disk from a spec will not fail.
	size, _ := diskSizeGB(attached)
	return size
}

// Status returns a string identifying the status of the instance. The
// value will match one of the Status* constants in the package.
func (gi Instance) Status() string {
	return gi.raw.Status
}

// Refresh updates the instance with its current data, utilizing the
// provided connection to request it.
func (gi *Instance) Refresh(conn *Connection) error {
	raw, err := conn.instance(gi.Zone, gi.ID)
	if err != nil {
		return errors.Trace(err)
	}

	gi.raw = *raw
	return nil
}

// Addresses identifies information about the network addresses
// associated with the instance and returns it.
func (gi Instance) Addresses() []network.Address {
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

	return addresses
}

// Metadata returns the user-specified metadata for the instance.
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

// FormatAuthorizedKeys returns our authorizedKeys with
// the username prepended to it. This is the format that
// GCE uses for its sshKeys metadata.
func FormatAuthorizedKeys(raw, user string) (string, error) {
	if raw == "" {
		return "", errors.New("empty raw")
	}
	if user == "" {
		return "", errors.New("empty user")
	}

	var userKeys string
	keys := strings.Split(raw, "\n")
	for _, key := range keys {
		userKeys += user + ":" + key + "\n"
	}
	return userKeys, nil
}

// packMetadata composes the provided data into the format required
// by the GCE API.
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

// unpackMetadata decomposes the provided data from the format used
// in the GCE API.
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
