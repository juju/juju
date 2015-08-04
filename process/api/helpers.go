// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/process"
	"gopkg.in/juju/charm.v5"
)

// API2Definition converts an API process definition struct into
// a charm.Process struct.
func API2Definition(d ProcessDefinition) charm.Process {
	return charm.Process{
		Name:        d.Name,
		Description: d.Description,
		Type:        d.Type,
		TypeOptions: d.TypeOptions,
		Command:     d.Command,
		Image:       d.Image,
		Ports:       API2charmPorts(d.Ports),
		Volumes:     API2charmVolumes(d.Volumes),
		EnvVars:     d.EnvVars,
	}
}

// Definition2api converts a charm.Process struct into an
// api.ProcessDefinition struct.
func Definition2api(d charm.Process) ProcessDefinition {
	return ProcessDefinition{
		Name:        d.Name,
		Description: d.Description,
		Type:        d.Type,
		TypeOptions: d.TypeOptions,
		Command:     d.Command,
		Image:       d.Image,
		Ports:       Charm2apiPorts(d.Ports),
		Volumes:     Charm2apiVolumes(d.Volumes),
		EnvVars:     d.EnvVars,
	}
}

// API2Proc converts an API Process info struct into a process.Info struct.
func API2Proc(p Process) process.Info {
	return process.Info{
		Process: API2Definition(p.Definition),
		Status:  APIStatus2Status(p.Status),
		Details: process.Details{
			ID:     p.Details.ID,
			Status: APIPluginStatus2PluginStatus(p.Details.Status),
		},
	}
}

// Proc2api converts a process.Info struct into an api.Process struct.
func Proc2api(p process.Info) Process {
	return Process{
		Definition: Definition2api(p.Process),
		Status:     Status2apiStatus(p.Status),
		Details: ProcessDetails{
			ID:     p.Details.ID,
			Status: PluginStatus2apiPluginStatus(p.Details.Status),
		},
	}
}

// APIStatus2Status converts an API ProcessStatus struct into a
// process.Status struct.
func APIStatus2Status(status ProcessStatus) process.Status {
	return process.Status{
		State:   status.State,
		Blocker: status.Blocker,
		Message: status.Message,
	}
}

// Status2APIStatus converts a process.Status struct into an
// API ProcessStatus struct.
func Status2apiStatus(status process.Status) ProcessStatus {
	return ProcessStatus{
		State:   status.State,
		Blocker: status.Blocker,
		Message: status.Message,
	}
}

// APIPluginStatus2PluginStatus converts an API PluginStatus struct into
// a process.PluginStatus struct.
func APIPluginStatus2PluginStatus(status PluginStatus) process.PluginStatus {
	return process.PluginStatus{
		State: status.State,
	}
}

// PluginStatus2APIPluginStatus converts a process.PluginStatus struct
// into an API PluginStatus struct.
func PluginStatus2apiPluginStatus(status process.PluginStatus) PluginStatus {
	return PluginStatus{
		State: status.State,
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
