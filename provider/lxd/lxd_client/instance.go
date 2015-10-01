// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/lxc/lxd/shared"
)

// AliveStatuses are the LXD statuses that indicate a container is "alive".
var AliveStatuses = []string{
	//StatusOK,
	//StatusPending,
	StatusStarting,
	StatusStarted,
	StatusRunning,
	//StatusThawed,
}

// InstanceSpec holds all the information needed to create a new LXD
// container.
type InstanceSpec struct {
	// ID is the "name" of the instance.
	ID string

	// Metadata is the instance metadata.
	Metadata map[string]string

	// Disks
	// Networks
	// Metadata
	// Tags
}

func (spec InstanceSpec) info(namespace string) *shared.ContainerState {
	name := spec.ID
	if namespace != "" {
		name = namespace + "-" + name
	}

	return &shared.ContainerState{
		Architecture:    0,
		Config:          map[string]string{},
		Devices:         shared.Devices{},
		Ephemeral:       false,
		ExpandedConfig:  map[string]string{},
		ExpandedDevices: shared.Devices{},
		Name:            name,
		Profiles:        []string{},
		//Status:          ContainerStatus{},
		Userdata: []byte{}, // from spec.Metadata
	}
}

// Summary builds an InstanceSummary based on the spec and returns it.
func (spec InstanceSpec) Summary(namespace string) InstanceSummary {
	info := spec.info(namespace)
	return newInstanceSummary(info)
}

// InstanceSummary captures all the data needed by Instance.
type InstanceSummary struct {
	// ID is the "name" of the instance.
	ID string

	// Status holds the status of the instance at a certain point in time.
	Status string

	// Metadata is the instance metadata.
	Metadata map[string]string

	// Addresses
}

func newInstanceSummary(info *shared.ContainerState) InstanceSummary {
	return InstanceSummary{
		ID:       info.Name,
		Status:   info.Status.State,
		Metadata: unpackMetadata(info.Userdata),
	}
}

// Instance represents a single realized LXD container.
type Instance struct {
	InstanceSummary

	// spec is the InstanceSpec used to create this instance.
	spec *InstanceSpec
}

func newInstance(info *shared.ContainerState, spec *InstanceSpec) *Instance {
	summary := newInstanceSummary(info)
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

// Status returns a string identifying the status of the instance.
func (gi Instance) Status() string {
	return gi.InstanceSummary.Status
}

// Metadata returns the user-specified metadata for the instance.
func (gi Instance) Metadata() map[string]string {
	// TODO*ericsnow) return a copy?
	return gi.InstanceSummary.Metadata
}

// packMetadata composes the provided data into the format required
// by the API.
func packMetadata(data map[string]string) []byte {
	// TODO(ericsnow) finish!
	panic("not finished")
	return nil
}

// unpackMetadata decomposes the provided data from the format used
// in the API.
func unpackMetadata(data []byte) map[string]string {
	if data == nil {
		return nil
	}

	// TODO(ericsnow) finish!
	panic("not finished")
	return nil
}
