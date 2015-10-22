// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// TODO(ericsnow) Eliminate the apiserver/common import if possible.
// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/workload"
)

// NewWorkloadResult builds a new WorkloadResult from the provided tag
// and error. NotFound is also set based on the error.
func NewWorkloadResult(id string, err error) WorkloadResult {
	result := workload.Result{
		ID:       id,
		Workload: nil,
		NotFound: errors.IsNotFound(err),
		Error:    err,
	}
	return Result2api(result)
}

// API2Result converts the API result to a workload.Result.
func API2Result(r WorkloadResult) (workload.Result, error) {
	result := workload.Result{
		NotFound: r.NotFound,
	}

	id, err := API2ID(r.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	result.ID = id

	if r.Workload != nil {
		info := API2Workload(*r.Workload)
		result.Workload = &info
	}

	if r.Error != nil {
		result.Error, _ = common.RestoreError(r.Error)
	}

	return result, nil
}

// Result2api converts the workload.Result into a WorkloadResult.
func Result2api(result workload.Result) WorkloadResult {
	res := WorkloadResult{
		NotFound: result.NotFound,
	}

	if result.ID != "" {
		res.Tag = names.NewPayloadTag(result.ID).String()
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

// API2ID converts the given tag string into a payload ID.
func API2ID(tagStr string) (string, error) {
	if tagStr == "" {
		return tagStr, nil
	}
	tag, err := names.ParsePayloadTag(tagStr)
	if err != nil {
		return "", errors.Trace(err)
	}
	return tag.Id(), nil
}

// Infos2TrackArgs converts the provided workload info into arguments
// for the Track API endpoint.
func Infos2TrackArgs(workloads []workload.Info) TrackArgs {
	var args TrackArgs
	for _, wl := range workloads {
		arg := Workload2api(wl)
		args.Workloads = append(args.Workloads, arg)
	}
	return args
}

// IDs2ListArgs converts the provided workload IDs into arguments
// for the List API endpoint.
func IDs2ListArgs(ids []string) params.Entities {
	return ids2args(ids)
}

// FullIDs2LookUpArgs converts the provided workload "full" IDs into arguments
// for the LookUp API endpoint.
func FullIDs2LookUpArgs(fullIDs []string) LookUpArgs {
	var args LookUpArgs
	for _, fullID := range fullIDs {
		name, rawID := workload.ParseID(fullID)
		args.Args = append(args.Args, LookUpArg{
			Name: name,
			ID:   rawID,
		})
	}
	return args
}

// IDs2SetStatusArgs converts the provided workload IDs into arguments
// for the SetStatus API endpoint.
func IDs2SetStatusArgs(ids []string, status string) SetStatusArgs {
	var args SetStatusArgs
	for _, id := range ids {
		arg := SetStatusArg{
			Status: status,
		}
		arg.Tag = names.NewPayloadTag(id).String()
		args.Args = append(args.Args, arg)
	}
	return args
}

// IDs2UntrackArgs converts the provided workload IDs into arguments
// for the Untrack API endpoint.
func IDs2UntrackArgs(ids []string) params.Entities {
	return ids2args(ids)
}

func ids2args(ids []string) params.Entities {
	var args params.Entities
	for _, id := range ids {
		tag := names.NewPayloadTag(id).String()
		args.Entities = append(args.Entities, params.Entity{
			Tag: tag,
		})
	}
	return args
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
