// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/process"
	"gopkg.in/juju/charm.v5"
)

// API2Proc converts an API Process info struct into a process.Info struct.
func API2Proc(p Process) process.Info {
	return process.Info{
		Process: charm.Process{
			Name:        p.Definition.Name,
			Description: p.Definition.Description,
			Type:        p.Definition.Type,
			TypeOptions: p.Definition.TypeOptions,
			Command:     p.Definition.Command,
			Image:       p.Definition.Image,
			Ports:       API2charmPorts(p.Definition.Ports),
			Volumes:     API2charmVolumes(p.Definition.Volumes),
			EnvVars:     p.Definition.EnvVars,
		},
		Details: process.Details{
			ID:     p.Details.ID,
			Status: APIStatus2Status(p.Details.Status),
		},
	}
}

// Proc2api converts a process.Info struct into an api.Process struct.
func Proc2api(p process.Info) Process {
	return Process{
		Definition: ProcessDefinition{
			Name:        p.Process.Name,
			Description: p.Process.Description,
			Type:        p.Process.Type,
			TypeOptions: p.Process.TypeOptions,
			Command:     p.Process.Command,
			Image:       p.Process.Image,
			Ports:       Charm2apiPorts(p.Process.Ports),
			Volumes:     Charm2apiVolumes(p.Process.Volumes),
			EnvVars:     p.Process.EnvVars,
		},
		Details: ProcessDetails{
			ID:     p.Details.ID,
			Status: Status2apiStatus(p.Details.Status),
		},
	}
}

// APIStatus2Status converts an API ProcessStatus struct into a
// process.Status struct.
func APIStatus2Status(status ProcessStatus) process.Status {
	return process.Status{
		Label: status.Label,
	}
}

// Status2APIStatus converts a process.Status struct into an
// API ProcessStatus struct.
func Status2apiStatus(status process.Status) ProcessStatus {
	return ProcessStatus{
		Label: status.Label,
	}
}

// API2charmPorts converts a slice of api.ProcessPorts into a slice of
// charm.ProcessPorts.
func API2charmPorts(ports []ProcessPort) []charm.ProcessPort {
	ret := make([]charm.ProcessPort, len(ports))
	for i, p := range ports {
		ret[i] = charm.ProcessPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

// API2charmVolumes converts a slice of api.ProcessVolume into a slice of charm.ProcessVolume.
func API2charmVolumes(vols []ProcessVolume) []charm.ProcessVolume {
	ret := make([]charm.ProcessVolume, len(vols))
	for i, v := range vols {
		ret[i] = charm.ProcessVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}

// Charm2apiPorts converts a slice of charm.ProcessPorts into a slice of api.ProcessPort.
func Charm2apiPorts(ports []charm.ProcessPort) []ProcessPort {
	ret := make([]ProcessPort, len(ports))
	for i, p := range ports {
		ret[i] = ProcessPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

// Charm2apiVolumes converts a slice of charm.ProcessVolume into a slice of api.ProcessVolume.
func Charm2apiVolumes(vols []charm.ProcessVolume) []ProcessVolume {
	ret := make([]ProcessVolume, len(vols))
	for i, v := range vols {
		ret[i] = ProcessVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}
