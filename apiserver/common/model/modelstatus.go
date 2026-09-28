// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// MachineServiceGetter is a function that returns a MachineService for the
// given model UUID.
type MachineServiceGetter = func(context.Context, coremodel.UUID) (MachineService, error)

// StatusServiceGetter is a function that returns a StatusService for the
// given model UUID.
type StatusServiceGetter = func(context.Context, coremodel.UUID) (StatusService, error)

// ModelInfoService defines domain service methods for managing a model.
type ModelInfoService interface {
	// IsControllerModel returns true if the model is the controller model.
	// The following errors may be returned:
	// - [modelerrors.NotFound] when the model does not exist.
	IsControllerModel(context.Context) (bool, error)

	// HasValidCredential returns true if the model has a valid credential.
	// The following errors may be returned:
	// - [modelerrors.NotFound] when the model no longer exists.
	HasValidCredential(context.Context) (bool, error)
}

// ModelService provides access to information about the models within the controller.
type ModelService interface {
	// GetModelUUIDs returns a list of all model UUIDs in the controller.
	GetModelUUIDs(context.Context) ([]coremodel.UUID, error)

	// ModelRedirection returns redirection information for the current model. If it
	// is not redirected, [modelmigrationerrors.ModelNotRedirected] is returned.
	ModelRedirection(ctx context.Context, modelUUID coremodel.UUID) (model.ModelRedirection, error)

	// Model returns the model associated with the provided uuid.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
}

// ModelStatusAPI implements the ModelStatus() API.
type ModelStatusAPI struct {
	authorizer        facade.Authorizer
	apiUser           names.UserTag
	modelTag          names.ModelTag
	controllerTag     names.ControllerTag
	modelService      ModelService
	getMachineService MachineServiceGetter
	getStatusService  StatusServiceGetter
}

// NewModelStatusAPI creates an implementation providing the ModelStatus() API.
func NewModelStatusAPI(
	controllerUUID string,
	modelUUID string,
	modelService ModelService,
	getMachineService MachineServiceGetter,
	getStatusService StatusServiceGetter,
	authorizer facade.Authorizer,
	apiUser names.UserTag,
) *ModelStatusAPI {
	controllerTag := names.NewControllerTag(controllerUUID)
	modelTag := names.NewModelTag(modelUUID)
	return &ModelStatusAPI{
		authorizer:        authorizer,
		apiUser:           apiUser,
		controllerTag:     controllerTag,
		modelTag:          modelTag,
		modelService:      modelService,
		getMachineService: getMachineService,
		getStatusService:  getStatusService,
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
	modelTag, err := names.ParseModelTag(tag)
	if err != nil {
		return status, errors.Trace(err)
	}
	isAdmin, err := HasModelAdmin(ctx, c.authorizer, c.controllerTag, modelTag)
	if err != nil {
		return status, errors.Trace(err)
	}

	if !isAdmin {
		return status, apiservererrors.ErrPerm
	}

	modelUUID := coremodel.UUID(modelTag.Id())

	if modelTag != c.modelTag {
		modelRedirection, err := c.modelService.ModelRedirection(ctx, modelUUID)
		if err == nil {
			hps, mErr := network.ParseProviderHostPorts(modelRedirection.Addresses...)
			if mErr != nil {
				return status, errors.Trace(mErr)
			}
			return status, &apiservererrors.RedirectError{
				Servers:         []network.ProviderHostPorts{hps},
				CACert:          modelRedirection.CACert,
				ControllerTag:   names.NewControllerTag(modelRedirection.ControllerUUID),
				ControllerAlias: modelRedirection.ControllerAlias,
			}
		} else if !errors.Is(err, modelerrors.ModelNotRedirected) {
			return status, errors.Trace(err)
		}
	}

	// TODO: update model DB drop detection logic. Currently,
	// statusService.GetModelStatusInfo does not return NotFound because model
	// data is read from the cache within the same DB connection.
	statusService, err := c.getStatusService(ctx, modelUUID)
	if err != nil {
		return status, errors.Trace(err)
	}
	modelInfo, err := statusService.GetModelStatusInfo(ctx)
	if errors.Is(err, modelerrors.NotFound) ||
		errors.Is(err, database.ErrDBDead) ||
		errors.Is(err, database.ErrDBNotFound) {
		return status, internalerrors.Errorf(
			"model %q does not exist", modelTag,
		).Add(errors.NotFound)
	} else if err != nil {
		return status, internalerrors.Errorf(
			"getting model info for tag %q: %w", modelTag, err,
		)
	}

	applications, err := statusService.GetApplicationAndUnitModelStatuses(ctx)
	if err != nil {
		return status, errors.Trace(err)
	}

	modelApplications := make([]params.ModelApplicationInfo, 0, len(applications))
	var unitCount int
	for name, units := range applications {
		modelApplications = append(modelApplications, params.ModelApplicationInfo{
			Name: name,
		})
		unitCount += units
	}

	machineService, err := c.getMachineService(ctx, modelUUID)
	if err != nil {
		return status, errors.Trace(err)
	}
	modelMachines, err := ModelMachineInfo(ctx, machineService, statusService)
	if err != nil {
		return status, errors.Trace(err)
	}

	storageStatuses, err := statusService.GetModelStorageStatuses(ctx)
	if err != nil {
		return status, errors.Trace(err)
	}
	modelFilesystems := make([]params.ModelFilesystemInfo, 0, len(storageStatuses.Filesystems))
	modelVolumes := make([]params.ModelVolumeInfo, 0, len(storageStatuses.Volumes))
	for _, fs := range storageStatuses.Filesystems {
		modelFilesystems = append(modelFilesystems, params.ModelFilesystemInfo{
			Id:         fs.ID,
			ProviderId: fs.ProviderID,
			Status:     fs.Status,
			Detachable: fs.Detachable,
		})
	}
	for _, vol := range storageStatuses.Volumes {
		modelVolumes = append(modelVolumes, params.ModelVolumeInfo{
			Id:         vol.ID,
			ProviderId: vol.ProviderID,
			Status:     vol.Status,
			Detachable: vol.Detachable,
		})
	}

	m, err := c.modelService.Model(ctx, modelUUID)
	if err != nil {
		return status, errors.Trace(err)
	}

	result := params.ModelStatus{
		ModelTag:           tag,
		Qualifier:          m.Qualifier.String(),
		Life:               m.Life,
		Type:               modelInfo.Type.String(),
		HostedMachineCount: len(modelMachines),
		ApplicationCount:   len(applications),
		UnitCount:          unitCount,
		Applications:       modelApplications,
		Machines:           modelMachines,
		Volumes:            modelVolumes,
		Filesystems:        modelFilesystems,
	}

	return result, nil
}
