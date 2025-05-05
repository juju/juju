// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelInfoService defines domain service methods for managing a model.
type ModelInfoService interface {
	// GetStatus returns the current status of the model.
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/modelerrors.NotFound]: When the model does not exist.
	GetStatus(context.Context) (domainmodel.StatusInfo, error)
}

// ModelStatusAPI implements the ModelStatus() API.
type ModelStatusAPI struct {
	authorizer        facade.Authorizer
	apiUser           names.UserTag
	backend           ModelManagerBackend
	getMachineService func(context.Context, coremodel.UUID) (MachineService, error)
}

// ModelApplicationInfo returns information about applications.
func ModelApplicationInfo(applications []Application) ([]params.ModelApplicationInfo, error) {
	applicationInfo := transform.Slice(applications, func(app Application) params.ModelApplicationInfo {
		return params.ModelApplicationInfo{Name: app.Name()}
	})
	return applicationInfo, nil
}

// NewModelStatusAPI creates an implementation providing the ModelStatus() API.
func NewModelStatusAPI(backend ModelManagerBackend, getMachineService func(context.Context, coremodel.UUID) (MachineService, error),
	authorizer facade.Authorizer, apiUser names.UserTag) *ModelStatusAPI {
	return &ModelStatusAPI{
		authorizer:        authorizer,
		apiUser:           apiUser,
		backend:           backend,
		getMachineService: getMachineService,
	}
}

// ModelStatus returns a summary of the model.
func (c *ModelStatusAPI) ModelStatus(ctx context.Context, req params.Entities) (params.ModelStatusResults, error) {
	models := req.Entities
	status := make([]params.ModelStatus, len(models))
	for i, model := range models {
		modelStatus, err := c.modelStatus(ctx, model.Tag)
		if err != nil {
			status[i].Error = apiservererrors.ServerError(err)
			continue
		}
		status[i] = modelStatus
	}
	return params.ModelStatusResults{Results: status}, nil
}

func (c *ModelStatusAPI) modelStatus(ctx context.Context, tag string) (params.ModelStatus, error) {
	var status params.ModelStatus
	// Use Model(ctx context.Context, uuid coremodel.UUID) impl for all model related properties below
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

	// Use Model controllerUUID and modelUUID for tags
	model, err := st.Model()
	if err != nil {
		return status, errors.Trace(err)
	}
	isAdmin, err := HasModelAdmin(ctx, c.authorizer, c.backend.ControllerTag(), model.ModelTag())
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

	// check for how many machines with isController=true in machine table
	hostedMachineCount := 0
	for _, m := range machines {
		if !m.IsManager() {
			hostedMachineCount++
		}
	}

	// use table applications for application names
	// use dbModelMetrics (v_model_metrics) for unit count
	applications, err := st.AllApplications()
	if err != nil {
		return status, errors.Trace(err)
	}
	var unitCount int
	for _, app := range applications {
		unitCount += app.UnitCount()
	}

	svc, err := c.getMachineService(ctx, coremodel.UUID(modelTag.Id()))
	if err != nil {
		return status, errors.Trace(err)
	}

	// use func (s *Service) HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error) {
	// refer to func (st *State) SetMachineCloudInstance(...) to get all the info, but some properties like status, vote and message still missing
	modelMachines, err := ModelMachineInfo(ctx, st, svc)
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
	// Use info from TABLE storage_volume (
	modelVolumes := ModelVolumeInfo(volumes)

	filesystems, err := st.AllFilesystems()
	if err != nil {
		return status, errors.Trace(err)
	}
	modelFilesystems := ModelFilesystemInfo(filesystems)

	result := params.ModelStatus{
		ModelTag:           tag,                               // use domain Model()
		OwnerTag:           model.Owner().String(),            // use domain Model()
		Life:               life.Value(model.Life().String()), // use domain Model()
		Type:               string(model.Type()),              // use domain Model()
		HostedMachineCount: hostedMachineCount,                // Use machines table and check for how many machines with isController=true
		ApplicationCount:   len(modelApplications),            // Use dbModelMetrics (v_model_metrics)
		UnitCount:          unitCount,                         // Use dbModelMetrics (v_model_metrics)
		Applications:       modelApplications,                 // Use table applications for application names
		Machines:           modelMachines,                     // Refer to services in machine_cloud_instance.go
		Volumes:            modelVolumes,                      // use info from TABLE storage_volume to create state & service
		Filesystems:        modelFilesystems,                  // TBD - look at block_device?
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
