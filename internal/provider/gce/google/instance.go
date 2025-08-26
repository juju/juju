// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"fmt"
	"path"

	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/network"
)

// InstanceSpec holds all the information needed to create a new GCE instance.
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

	// AvailabilityZone holds the name of the availability zone in which
	// to create the instance.
	AvailabilityZone string

	// AllocatePublicIP is true if the instance should be assigned a public IP
	// address, exposing it to access from outside the internal network.
	AllocatePublicIP bool
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

// Summary builds an InstanceSummary based on the spec and returns it.
func (is InstanceSpec) Summary() InstanceSummary {
	raw := is.raw()
	return newInstanceSummary(raw)
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
		result = append(result, is.Network.newInterface(name, is.AllocatePublicIP))
	}
	return result
}

// RootDisk identifies the root disk for a given instance (or instance
// spec) and returns it. If the root disk could not be determined then
// nil is returned.
// TODO(ericsnow) Return an error?
func (is InstanceSpec) RootDisk() *compute.AttachedDisk {
	return is.Disks[0].newAttached()
}

// InstanceSummary captures all the data needed by Instance.
type InstanceSummary struct {
	// ID is the "name" of the instance.
	ID string
	// ZoneName is the unqualified name of the zone in which the
	// instance was provisioned.
	ZoneName string
	// Status holds the status of the instance at a certain point in time.
	Status string
	// Metadata is the instance metadata.
	Metadata map[string]string
	// Addresses are the IP Addresses associated with the instance.
	Addresses network.ProviderAddresses
	// NetworkInterfaces are the network connections associated with
	// the instance.
	NetworkInterfaces []*compute.NetworkInterface
}

func newInstanceSummary(raw *compute.Instance) InstanceSummary {
	return InstanceSummary{
		ID:                raw.Name,
		ZoneName:          path.Base(raw.Zone),
		Status:            raw.Status,
		Metadata:          unpackMetadata(raw.Metadata),
		Addresses:         extractAddresses(raw.NetworkInterfaces...),
		NetworkInterfaces: raw.NetworkInterfaces,
	}
}

// Instance represents a single realized GCE compute instance.
type Instance struct {
	InstanceSummary

	// spec is the InstanceSpec used to create this instance.
	spec *InstanceSpec
}

func newInstance(raw *compute.Instance, spec *InstanceSpec) *Instance {
	summary := newInstanceSummary(raw)
	return NewInstance(summary, spec)
}

// NewInstance builds an instance from the provided summary and spec
// and returns it.
func NewInstance(summary InstanceSummary, spec *InstanceSpec) *Instance {
	if spec != nil {
		// Make a copy.
		val := *spec
		spec = &val
	}
	return &Instance{
		InstanceSummary: summary,
		spec:            spec,
	}
}

// RootDisk returns an AttachedDisk
func (gi Instance) RootDisk() *compute.AttachedDisk {
	if gi.spec == nil {
		return nil
	}
	return gi.spec.RootDisk()
}

// RootDiskGB returns the size of the instance's root disk. If it
// cannot be determined then 0 is returned.
func (gi Instance) RootDiskGB() uint64 {
	if gi.spec == nil {
		return 0
	}
	attached := gi.RootDisk()
	return uint64(attached.InitializeParams.DiskSizeGb)
}

// Status returns a string identifying the status of the instance. The
// value will match one of the Status* constants in the package.
func (gi Instance) Status() string {
	return gi.InstanceSummary.Status
}

// Addresses identifies information about the network addresses
// associated with the instance and returns it.
func (gi Instance) Addresses() network.ProviderAddresses {
	// TODO*ericsnow) return a copy?
	return gi.InstanceSummary.Addresses
}

// Metadata returns the user-specified metadata for the instance.
func (gi Instance) Metadata() map[string]string {
	// TODO*ericsnow) return a copy?
	return gi.InstanceSummary.Metadata
}

// NetworkInterfaces returns the details of the network connection for
// this instance.
func (gi Instance) NetworkInterfaces() []compute.NetworkInterface {
	var results []compute.NetworkInterface
	// Copy to prevent callers from mutating the source data.
	for _, iface := range gi.InstanceSummary.NetworkInterfaces {
		results = append(results, *iface)
	}
	return results
}

// packMetadata composes the provided data into the format required
// by the GCE API.
func packMetadata(data map[string]string) *compute.Metadata {
	var items []*compute.MetadataItems
	for key, value := range data {
		// Needs to be a new variable so that &localValue is different
		// each time round the loop.
		localValue := value
		item := compute.MetadataItems{
			Key:   key,
			Value: &localValue,
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
		if item == nil {
			continue
		}
		value := ""
		if item.Value != nil {
			value = *item.Value
		}
		result[item.Key] = value
	}
	return result
}

func formatMachineType(zone, name string) string {
	return fmt.Sprintf("zones/%s/machineTypes/%s", zone, name)
}
