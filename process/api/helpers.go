// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/plugin"
	"gopkg.in/juju/charm.v5"
)

// API2Proc converts an API Process info struct into a process.Info struct.
func API2Proc(p ProcessInfo) process.Info {
	return process.Info{
		Process: charm.Process{
			Name:        p.Process.Name,
			Description: p.Process.Description,
			Type:        p.Process.Type,
			TypeOptions: p.Process.TypeOptions,
			Command:     p.Process.Command,
			Image:       p.Process.Image,
			Ports:       API2charmPorts(p.Process.Ports),
			Volumes:     API2charmVolumes(p.Process.Volumes),
			EnvVars:     p.Process.EnvVars,
		},
		Status: process.Status(p.Status),
		Details: plugin.ProcDetails{
			ID: p.Details.ID,
			ProcStatus: plugin.ProcStatus{
				Status: p.Details.Status,
			},
		},
	}
}

// Proc2api converts a process.Info struct into an api.ProcessInfo struct.
func Proc2api(p process.Info) ProcessInfo {
	return ProcessInfo{
		Process: Process{
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
		Status: int(p.Status),
		Details: ProcDetails{
			ID: p.Details.ID,
			ProcStatus: ProcStatus{
				Status: p.Details.Status,
			},
		},
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
