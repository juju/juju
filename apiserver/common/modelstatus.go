// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelStatusAPI implements the ModelStatus() API.
type ModelStatusAPI struct {
	authorizer facade.Authorizer
	apiUser    names.UserTag
	backend    ModelManagerBackend
}

// ModelApplicationInfo returns information about applications.
func ModelApplicationInfo(applications []Application) ([]params.ModelApplicationInfo, error) {
	applicationInfo := transform.Slice(applications, func(app Application) params.ModelApplicationInfo {
		return params.ModelApplicationInfo{Name: app.Name()}
	})
	return applicationInfo, nil
}

// NewModelStatusAPI creates an implementation providing the ModelStatus() API.
func NewModelStatusAPI(backend ModelManagerBackend, authorizer facade.Authorizer, apiUser names.UserTag) *ModelStatusAPI {
	return &ModelStatusAPI{
		authorizer: authorizer,
		apiUser:    apiUser,
		backend:    backend,
	}
}

// ModelStatus returns a summary of the model.
func (c *ModelStatusAPI) ModelStatus(ctx context.Context, req params.Entities) (params.ModelStatusResults, error) {
	models := req.Entities
	status := make([]params.ModelStatus, len(models))
	for i, model := range models {
		modelStatus, err := c.modelStatus(model.Tag)
		if err != nil {
			status[i].Error = apiservererrors.ServerError(err)
			continue
		}
		status[i] = modelStatus
	}
	return params.ModelStatusResults{Results: status}, nil
}

func (c *ModelStatusAPI) modelStatus(tag string) (params.ModelStatus, error) {
	var status params.ModelStatus
	modelTag, err := names.ParseModelTag(tag)
	if err != nil {
		return status, errors.Trace(err)
	}
	st := c.backend
	if modelTag != c.backend.ModelTag() {
		otherSt, releaser, err := c.backend.GetBackend(modelTag.Id())
		if err != nil {
			return status, errors.Trace(err)
		}
		defer releaser()
		st = otherSt
	}

	model, err := st.Model()
	if err != nil {
		return status, errors.Trace(err)
	}
	isAdmin, err := HasModelAdmin(c.authorizer, c.backend.ControllerTag(), model.ModelTag())
	if err != nil {
		return status, errors.Trace(err)
	}
	if !isAdmin {
		return status, apiservererrors.ErrPerm
	}

	machines, err := st.AllMachines()
	if err != nil {
		return status, errors.Trace(err)
	}

	var hostedMachines []Machine
	for _, m := range machines {
		if !m.IsManager() {
			hostedMachines = append(hostedMachines, m)
		}
	}

	applications, err := st.AllApplications()
	if err != nil {
		return status, errors.Trace(err)
	}
	var unitCount int
	for _, app := range applications {
		unitCount += app.UnitCount()
	}

	modelMachines, err := ModelMachineInfo(st)
	if err != nil {
		return status, errors.Trace(err)
	}

	// TODO (Anvial): we need to think about common parameter list (maybe "st") to all these functions:
	// ModelMachineInfo, ModelApplicationInfo, ModelVolumeInfo, ModelFilesystemInfo. Looks like better to do in
	// ModelMachineInfo style and optimize st.*() calls.

	modelApplications, err := ModelApplicationInfo(applications)
	if err != nil {
		return status, errors.Trace(err)
	}

	volumes, err := st.AllVolumes()
	if err != nil {
		return status, errors.Trace(err)
	}
	modelVolumes := ModelVolumeInfo(volumes)

	filesystems, err := st.AllFilesystems()
	if err != nil {
		return status, errors.Trace(err)
	}
	modelFilesystems := ModelFilesystemInfo(filesystems)

	result := params.ModelStatus{
		ModelTag:           tag,
		OwnerTag:           model.Owner().String(),
		Life:               life.Value(model.Life().String()),
		Type:               string(model.Type()),
		HostedMachineCount: len(hostedMachines),
		ApplicationCount:   len(modelApplications),
		UnitCount:          unitCount,
		Applications:       modelApplications,
		Machines:           modelMachines,
		Volumes:            modelVolumes,
		Filesystems:        modelFilesystems,
	}

	return result, nil
}

// ModelFilesystemInfo returns information about filesystems in the model.
func ModelFilesystemInfo(in []state.Filesystem) []params.ModelFilesystemInfo {
	out := make([]params.ModelFilesystemInfo, len(in))
	for i, in := range in {
		var statusString string
		status, err := in.Status()
		if err != nil {
			statusString = err.Error()
		} else {
			statusString = string(status.Status)
		}
		var providerId string
		if info, err := in.Info(); err == nil {
			providerId = info.FilesystemId
		}
		out[i] = params.ModelFilesystemInfo{
			Id:         in.Tag().Id(),
			ProviderId: providerId,
			Status:     statusString,
			Message:    status.Message,
			Detachable: in.Detachable(),
		}
	}
	return out
}

// ModelVolumeInfo returns information about volumes in the model.
func ModelVolumeInfo(in []state.Volume) []params.ModelVolumeInfo {
	out := make([]params.ModelVolumeInfo, len(in))
	for i, in := range in {
		var statusString string
		status, err := in.Status()
		if err != nil {
			statusString = err.Error()
		} else {
			statusString = string(status.Status)
		}
		var providerId string
		if info, err := in.Info(); err == nil {
			providerId = info.VolumeId
		}
		out[i] = params.ModelVolumeInfo{
			Id:         in.Tag().Id(),
			ProviderId: providerId,
			Status:     statusString,
			Message:    status.Message,
			Detachable: in.Detachable(),
		}
	}
	return out
}
