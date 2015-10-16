// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/workload"
)

// API2FullID converts the API struct to a full ID string.
func API2FullID(fullID FullID) string {
	return workload.BuildID(fullID.Class, fullID.ID)
}

// FullID2api converts a full ID string to the API struct.
func FullID2api(fullID string) FullID {
	class, id := workload.ParseID(fullID)
	return FullID{
		Class: class,
		ID:    id,
	}
}

// API2Definition converts an API workload definition struct into
// a charm.PayloadClass struct.
func API2Definition(d WorkloadDefinition) charm.PayloadClass {
	return charm.PayloadClass{
		Name: d.Name,
		Type: d.Type,
	}
}

// Definition2api converts a charm.PayloadClass struct into an
// api.WorkloadDefinition struct.
func Definition2api(d charm.PayloadClass) WorkloadDefinition {
	return WorkloadDefinition{
		Name: d.Name,
		Type: d.Type,
	}
}

// API2Workload converts an API Workload info struct into a workload.Info struct.
func API2Workload(p Workload) workload.Info {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return workload.Info{
		PayloadClass: API2Definition(p.Definition),
		Status:       APIStatus2Status(p.Status),
		Tags:         tags,
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
		Definition: Definition2api(p.PayloadClass),
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
