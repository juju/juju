// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
)

// BulkFailure indicates that at least one arg failed.
var BulkFailure = errors.Errorf("at least one bulk arg has an error")

// TrackArgs are the arguments for the Track endpoint.
type TrackArgs struct {
	// Workloads is the list of Workloads to track
	Workloads []Workload
}

// WorkloadResults is the result for a call that makes one or more requests
// about workloads.
type WorkloadResults struct {
	// Results is the list of results.
	Results []WorkloadResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// WorkloadResult contains the result for a single call.
type WorkloadResult struct {
	// ID is the id of the workload referenced in the call..
	ID string
	// Error is the error (if any) for the call referring to ID.
	Error *params.Error
}

// ListArgs are the arguments for the List endpoint.
type ListArgs struct {
	// IDs is the list of IDs of the workloads you want information on.
	IDs []string
}

// ListResults contains the results for a call to List.
type ListResults struct {
	// Results is the list of workload results.
	Results []ListResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// ListResult contains the results for a single call to List.
type ListResult struct {
	// ID is the id of the workload this result applies to.
	ID string
	// Info holds the details of the workload.
	Info Workload
	// NotFound indicates that the workload was not found in state.
	NotFound bool
	// Error holds the error retrieving this information (if any).
	Error *params.Error
}

// SetStatusArgs are the arguments for the SetStatus endpoint.
type SetStatusArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []SetStatusArg
}

// SetStatusArg are the arguments for a single call to the
// SetStatus endpoint.
type SetStatusArg struct {
	// ID is the ID of the workload.
	ID string
	// Status is the status of the workload.
	Status string
}

// UntrackArgs are the arguments for the Untrack endpoint.
type UntrackArgs struct {
	// IDs is a list of IDs of workloads.
	IDs []string
}

// Workload contains information about a workload.
type Workload struct {
	// Workload is information about the workload itself.
	Definition WorkloadDefinition
	// Status is the Juju-level status for the workload.
	Status WorkloadStatus
	// Details are the information returned from starting the workload.
	Details WorkloadDetails
}

// WorkloadDefinition is the static definition of a workload in a charm.
type WorkloadDefinition struct {
	// Name is the name of the workload.
	Name string
	// Description is a brief description of the workload.
	Description string
	// Type is the name of the workload type.
	Type string
	// TypeOptions is a map of arguments for the workload type.
	TypeOptions map[string]string
	// Command is use command executed used by the workload, if any.
	Command string
	// Image is the image used by the workload, if any.
	Image string
	// Ports is a list of WorkloadPort.
	Ports []WorkloadPort
	// Volumes is a list of WorkloadVolume.
	Volumes []WorkloadVolume
	// EnvVars is map of environment variables used by the workload.
	EnvVars map[string]string
}

// WorkloadPort is network port information for a workload.
type WorkloadPort struct {
	// External is the port on the host.
	External int
	// Internal is the port on the workload.
	Internal int
	// Endpoint is the unit-relation endpoint matching the external
	// port, if any.
	Endpoint string
}

// WorkloadVolume is storage volume information for a workload.
type WorkloadVolume struct {
	// ExternalMount is the path on the host.
	ExternalMount string
	// InternalMount is the path on the workload.
	InternalMount string
	// Mode is the "ro" OR "rw"
	Mode string
	// Name is the name of the storage metadata entry, if any.
	Name string
}

// WorkloadStatus represents the Juju-level status of the workload.
type WorkloadStatus struct {
	// State is the Juju-defined state the workload is in.
	State string
	// Blocker identifies the kind of blocker preventing interaction
	// with the workload.
	Blocker string
	// Message is the human-readable information about the status of
	// the workload.
	Message string
}

// WorkloadDetails represents information about a workload launched by a plugin.
type WorkloadDetails struct {
	// ID is a unique string identifying the workload to the plugin.
	ID string
	// Status is the status of the workload after launch.
	Status PluginStatus
}

// PluginStatus represents the plugin-defined status for the workload.
type PluginStatus struct {
	// State represents the human-readable label returned by the plugin for
	// the workload that represents the status of the workload.
	State string
}
