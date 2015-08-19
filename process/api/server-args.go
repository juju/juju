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

// RegisterProcessArgs are the arguments for the RegisterProcesses endpoint.
type RegisterProcessesArgs struct {
	// Processes is the list of Processes to register
	Processes []Process
}

// ProcessResults is the result for a call that makes one or more requests
// about processes.
type ProcessResults struct {
	// Results is the list of results.
	Results []ProcessResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// ProcessResult contains the result for a single call.
type ProcessResult struct {
	// ID is the id of the process referenced in the call..
	ID string
	// Error is the error (if any) for the call referring to ID.
	Error *params.Error
}

// ListProcessesesArgs are the arguments for the ListProcesses endpoint.
type ListProcessesArgs struct {
	// IDs is the list of IDs of the processes you want information on.
	IDs []string
}

// ListProcessesResults contains the results for a call to ListProcesses.
type ListProcessesResults struct {
	// Results is the list of process results.
	Results []ListProcessResult
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// ListProcessResult contains the results for a single call to ListProcess.
type ListProcessResult struct {
	// ID is the id of the process this result applies to.
	ID string
	// Info holds the details of the process.
	Info Process
	// NotFound indicates that the process was not found in state.
	NotFound bool
	// Error holds the error retrieving this information (if any).
	Error *params.Error
}

// ListDefinitionsResults contains the results for a call to ListDefinitions.
type ListDefinitionsResults struct {
	// Results is the list of definition results.
	Results []ProcessDefinition
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// SetProcessesStatusArgs are the arguments for the SetProcessesStatus endpoint.
type SetProcessesStatusArgs struct {
	// Args is the list of arguments to pass to this function.
	Args []SetProcessStatusArg
}

// SetProcessStatusArg are the arguments for a single call to the
// SetProcessStatus endpoint.
type SetProcessStatusArg struct {
	// ID is the ID of the process.
	ID string
	// Status is the status of the process.
	Status ProcessStatus
	// PluginStatus is the plugin-provided status of the process.
	PluginStatus PluginStatus
}

// UntrackArgs are the arguments for the Untrack endpoint.
type UntrackArgs struct {
	// IDs is a list of IDs of processes.
	IDs []string
}

// Process contains information about a workload process.
type Process struct {
	// Process is information about the process itself.
	Definition ProcessDefinition
	// Status is the Juju-level status for the process.
	Status ProcessStatus
	// Details are the information returned from starting the process.
	Details ProcessDetails
}

// ProcessDefinition is the static definition of a workload process in a charm.
type ProcessDefinition struct {
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
	// Ports is a list of ProcessPort.
	Ports []ProcessPort
	// Volumes is a list of ProcessVolume.
	Volumes []ProcessVolume
	// EnvVars is map of environment variables used by the process.
	EnvVars map[string]string
}

// ProcessPort is network port information for a workload process.
type ProcessPort struct {
	// External is the port on the host.
	External int
	// Internal is the port on the process.
	Internal int
	// Endpoint is the unit-relation endpoint matching the external
	// port, if any.
	Endpoint string
}

// ProcessVolume is storage volume information for a workload process.
type ProcessVolume struct {
	// ExternalMount is the path on the host.
	ExternalMount string
	// InternalMount is the path on the process.
	InternalMount string
	// Mode is the "ro" OR "rw"
	Mode string
	// Name is the name of the storage metadata entry, if any.
	Name string
}

// ProcessStatus represents the Juju-level status of the process.
type ProcessStatus struct {
	// State is the Juju-defined state the process is in.
	State string
	// Blocker identifies the kind of blocker preventing interaction
	// with the process.
	Blocker string
	// Message is the human-readable information about the status of
	// the process.
	Message string
}

// ProcessDetails represents information about a process launched by a plugin.
type ProcessDetails struct {
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
