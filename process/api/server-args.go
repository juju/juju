// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// RegisterProcessArgs are the arguments for the RegisterProcess endpoint.
type RegisterProcessArgs struct {
	// UnitTag is the tag of the unit on which the process resides.
	UnitTag string
	// ProcessInfo contains information about the process being registered.
	ProcessInfo
}

// ListProcessesArgs are the arguments for the ListProcesses endpoint.
type ListProcessesArgs struct {
	// UnitTag is the tag of the unit on which the process resides.
	UnitTag string
	// IDs is the list of IDs of the processes you want information on.
	IDs []string
}

// SetProcessStatusArgs are the arguments for the SetProcessStatus endpoint.
type SetProcessStatusArgs struct {
	// UnitTag is the tag of the unit on which the process resides.
	UnitTag string
	// ID is the ID of the process.
	ID     string
	Status ProcStatus
}

// UnregisterProcessArgs are the arguments for the UnregisterProcess endpoint.
type UnregisterProcessArgs struct {
	// UnitTag is the tag of the unit on which the process resides.
	UnitTag string
	// ID is the ID of the process.
	ID string
}

// ProcessInfo contains information about a workload process.
type ProcessInfo struct {
	// Process is information about the process itself.
	Process Process
	// Status is the status of the process.
	Status int
	// Details are the information returned from starting the process.
	Details ProcDetails
}

// Process is the static definition of a workload process in a charm.
type Process struct {
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

// ProcDetails represents information about a process launched by a plugin.
type ProcDetails struct {
	// ID is a unique string identifying the process to the plugin.
	ID string
	// ProcStatus is the status of the process after launch.
	ProcStatus
}

// ProcStatus represents the data returned from the Status call.
type ProcStatus struct {
	// Status represents the human-readable string returned by the plugin for
	// the process.
	Status string
}
