// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/workload"
	"gopkg.in/juju/charm.v5"
)

// API2Definition converts an API workload definition struct into
// a charm.Workload struct.
func API2Definition(d WorkloadDefinition) charm.Workload {
	return charm.Workload{
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

// Definition2api converts a charm.Workload struct into an
// api.WorkloadDefinition struct.
func Definition2api(d charm.Workload) WorkloadDefinition {
	return WorkloadDefinition{
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

// API2Workload converts an API Workload info struct into a workload.Info struct.
func API2Workload(p Workload) workload.Info {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return workload.Info{
		Workload: API2Definition(p.Definition),
		Status:   APIStatus2Status(p.Status),
		Tags:     tags,
		Details: workload.Details{
			ID:     p.Details.ID,
			Status: APIPluginStatus2PluginStatus(p.Details.Status),
		},
	}
}

// Workload2api converts a workload.Info struct into an api.Workload struct.
func Workload2api(p workload.Info) Workload {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return Workload{
		Definition: Definition2api(p.Workload),
		Status:     Status2apiStatus(p.Status),
		Tags:       tags,
		Details: WorkloadDetails{
			ID:     p.Details.ID,
			Status: PluginStatus2apiPluginStatus(p.Details.Status),
		},
	}
}

// APIStatus2Status converts an API WorkloadStatus struct into a
// workload.Status struct.
func APIStatus2Status(status WorkloadStatus) workload.Status {
	return workload.Status{
		State:   status.State,
		Blocker: status.Blocker,
		Message: status.Message,
	}
}

// Status2apiStatus converts a workload.Status struct into an
// API WorkloadStatus struct.
func Status2apiStatus(status workload.Status) WorkloadStatus {
	return WorkloadStatus{
		State:   status.State,
		Blocker: status.Blocker,
		Message: status.Message,
	}
}

// APIPluginStatus2PluginStatus converts an API PluginStatus struct into
// a workload.PluginStatus struct.
func APIPluginStatus2PluginStatus(status PluginStatus) workload.PluginStatus {
	return workload.PluginStatus{
		State: status.State,
	}
}

// PluginStatus2apiPluginStatus converts a workload.PluginStatus struct
// into an API PluginStatus struct.
func PluginStatus2apiPluginStatus(status workload.PluginStatus) PluginStatus {
	return PluginStatus{
		State: status.State,
	}
}

// API2charmPorts converts a slice of api.WorkloadPorts into a slice of
// charm.WorkloadPorts.
func API2charmPorts(ports []WorkloadPort) []charm.WorkloadPort {
	ret := make([]charm.WorkloadPort, len(ports))
	for i, p := range ports {
		ret[i] = charm.WorkloadPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

// API2charmVolumes converts a slice of api.WorkloadVolume into a slice of charm.WorkloadVolume.
func API2charmVolumes(vols []WorkloadVolume) []charm.WorkloadVolume {
	ret := make([]charm.WorkloadVolume, len(vols))
	for i, v := range vols {
		ret[i] = charm.WorkloadVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}

// Charm2apiPorts converts a slice of charm.WorkloadPorts into a slice of api.WorkloadPort.
func Charm2apiPorts(ports []charm.WorkloadPort) []WorkloadPort {
	ret := make([]WorkloadPort, len(ports))
	for i, p := range ports {
		ret[i] = WorkloadPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

// Charm2apiVolumes converts a slice of charm.WorkloadVolume into a slice of api.WorkloadVolume.
func Charm2apiVolumes(vols []charm.WorkloadVolume) []WorkloadVolume {
	ret := make([]WorkloadVolume, len(vols))
	for i, v := range vols {
		ret[i] = WorkloadVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}
