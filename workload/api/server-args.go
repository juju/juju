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
// about processes.
type WorkloadResults struct {
	// Results is the list of results.
	Results []WorkloadResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// WorkloadResult contains the result for a single call.
type WorkloadResult struct {
	// ID is the id of the process referenced in the call..
	ID string
	// Error is the error (if any) for the call referring to ID.
	Error *params.Error
}

// ListArgs are the arguments for the List endpoint.
type ListArgs struct {
	// IDs is the list of IDs of the processes you want information on.
	IDs []string
}

// ListResults contains the results for a call to List.
type ListResults struct {
	// Results is the list of process results.
	Results []ListResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// ListResult contains the results for a single call to List.
type ListResult struct {
	// ID is the id of the process this result applies to.
	ID string
	// Info holds the details of the process.
	Info Workload
	// NotFound indicates that the process was not found in state.
	NotFound bool
	// Error holds the error retrieving this information (if any).
	Error *params.Error
}

// DefinitionsResults contains the results for a call to Definitions.
type DefinitionsResults struct {
	// Results is the list of definition results.
	Results []WorkloadDefinition
	// Error is the error (if any) for the call as a whole.
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
	// ID is the ID of the process.
	ID string
	// Status is the status of the process.
	Status WorkloadStatus
	// PluginStatus is the plugin-provided status of the process.
	PluginStatus PluginStatus
}

// UntrackArgs are the arguments for the Untrack endpoint.
type UntrackArgs struct {
	// IDs is a list of IDs of processes.
	IDs []string
}

// Workload contains information about a workload process.
type Workload struct {
	// Workload is information about the process itself.
	Definition WorkloadDefinition
	// Status is the Juju-level status for the process.
	Status WorkloadStatus
	// Details are the information returned from starting the process.
	Details WorkloadDetails
}

// WorkloadDefinition is the static definition of a workload process in a charm.
type WorkloadDefinition struct {
	// Name is the name of the process.
	Name string
	// Description is a brief description of the process.
	Description string
	// Type is the name of the process type.
	Type string
	// TypeOptions is a map of arguments for the process type.
	TypeOptions map[string]string
	// Command is use command executed used by the process, if any.
	Command string
	// Image is the image used by the process, if any.
	Image string
	// Ports is a list of WorkloadPort.
	Ports []WorkloadPort
	// Volumes is a list of WorkloadVolume.
	Volumes []WorkloadVolume
	// EnvVars is map of environment variables used by the process.
	EnvVars map[string]string
}

// WorkloadPort is network port information for a workload process.
type WorkloadPort struct {
	// External is the port on the host.
	External int
	// Internal is the port on the process.
	Internal int
	// Endpoint is the unit-relation endpoint matching the external
	// port, if any.
	Endpoint string
}

// WorkloadVolume is storage volume information for a workload process.
type WorkloadVolume struct {
	// ExternalMount is the path on the host.
	ExternalMount string
	// InternalMount is the path on the process.
	InternalMount string
	// Mode is the "ro" OR "rw"
	Mode string
	// Name is the name of the storage metadata entry, if any.
	Name string
}

// WorkloadStatus represents the Juju-level status of the process.
type WorkloadStatus struct {
	// State is the Juju-defined state the process is in.
	State string
	// Blocker identifies the kind of blocker preventing interaction
	// with the process.
	Blocker string
	// Message is the human-readable information about the status of
	// the process.
	Message string
}

// WorkloadDetails represents information about a process launched by a plugin.
type WorkloadDetails struct {
	// ID is a unique string identifying the process to the plugin.
	ID string
	// Status is the status of the process after launch.
	Status PluginStatus
}

// PluginStatus represents the plugin-defined status for the process.
type PluginStatus struct {
	// State represents the human-readable label returned by the plugin for
	// the process that represents the status of the workload process.
	State string
}
