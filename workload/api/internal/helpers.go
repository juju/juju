// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Eliminate the apiserver/common import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/workload"
)

// NewWorkloadResult builds a new WorkloadResult from the provided ID
// and error. NotFound is also set based on the error.
func NewWorkloadResult(id string, err error) WorkloadResult {
	result := workload.Result{
		ID:       id,
		NotFound: errors.IsNotFound(err),
		Error:    err,
	}
	return Result2api(result)
}

// TODO(ericsnow) Properly convert WorkloadResult.Error.

// API2Result converts the API result to a workload.Result.
func API2Result(r WorkloadResult) workload.Result {
	result := workload.Result{
		ID:       r.ID.Id(),
		NotFound: r.NotFound,
	}
	if r.Workload != nil {
		info := API2Workload(*r.Workload)
		result.Workload = &info
	}
	if r.Error != nil {
		result.Error = r.Error
	}
	return result
}

// Result2api converts the workload.Result into a WorkloadResult.
func Result2api(result workload.Result) WorkloadResult {
	res := WorkloadResult{
		NotFound: result.NotFound,
	}
	if result.ID != "" {
		res.ID = names.NewPayloadTag(result.ID)
	}
	if result.Workload != nil {
		wl := Workload2api(*result.Workload)
		res.Workload = &wl
	}
	if result.Error != nil {
		res.Error = common.ServerError(result.Error)
	}
	return res
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
