// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/rpc/params"
)

// ControllerAPIV12 implements the controller APIV12.
type ControllerAPIV12 struct {
	*ControllerAPI
}

// ModelStatus returns a summary of the model.
func (c *ControllerAPIV12) ModelStatus(ctx context.Context, req params.Entities) (params.ModelStatusResultsLegacy, error) {
	status, err := c.ControllerAPI.ModelStatus(ctx, req)
	if err != nil {
		return params.ModelStatusResultsLegacy{}, errors.Trace(err)
	}
	result := params.ModelStatusResultsLegacy{
		Results: make([]params.ModelStatusLegacy, len(status.Results)),
	}
	for i, r := range status.Results {
		if r.Error != nil {
			result.Results[i].Error = r.Error
			continue
		}
		result.Results[i] = params.ModelStatusLegacy{
			ModelTag:           r.ModelTag,
			Life:               r.Life,
			Type:               r.Type,
			HostedMachineCount: r.HostedMachineCount,
			ApplicationCount:   r.ApplicationCount,
			UnitCount:          r.UnitCount,
			OwnerTag:           names.NewUserTag(r.Qualifier).String(),
			Applications:       r.Applications,
			Machines:           r.Machines,
			Volumes:            r.Volumes,
			Filesystems:        r.Filesystems,
		}
	}
	return result, nil
}

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c *ControllerAPIV12) AllModels(ctx context.Context) (params.UserModelListLegacy, error) {
	models, err := c.ControllerAPI.AllModels(ctx)
	if err != nil {
		return params.UserModelListLegacy{}, errors.Trace(err)
	}
	result := params.UserModelListLegacy{
		UserModels: make([]params.UserModelLegacy, 0, len(models.UserModels)),
	}
	for i, model := range models.UserModels {
		result.UserModels[i] = params.UserModelLegacy{
			ModelLegacy: params.ModelLegacy{
				Name:     model.Name,
				UUID:     model.UUID,
				Type:     model.Type,
				OwnerTag: names.NewUserTag(model.Qualifier).String(),
			},
			LastConnection: model.LastConnection,
		}
	}
	return result, nil
}

// ListBlockedModels returns a list of all models on the controller
// which have a block in place.  The resulting slice is sorted by model
// name, then owner. Callers must be controller administrators to retrieve the
// list.
func (c *ControllerAPIV12) ListBlockedModels(ctx context.Context) (params.ModelBlockInfoListLegacy, error) {
	models, err := c.ControllerAPI.ListBlockedModels(ctx)
	if err != nil {
		return params.ModelBlockInfoListLegacy{}, errors.Trace(err)
	}
	result := params.ModelBlockInfoListLegacy{
		Models: make([]params.ModelBlockInfoLegacy, 0, len(models.Models)),
	}
	for i, model := range models.Models {
		result.Models[i] = params.ModelBlockInfoLegacy{
			Name:     model.Name,
			UUID:     model.UUID,
			OwnerTag: names.NewUserTag(model.Qualifier).String(),
			Blocks:   model.Blocks,
		}
	}
	return result, nil
}

// HostedModelConfigs returns all the information that the client needs in
// order to connect directly with the host model's provider and destroy it
// directly.
func (c *ControllerAPIV12) HostedModelConfigs(ctx context.Context) (params.HostedModelConfigsResultsLegacy, error) {
	results, err := c.ControllerAPI.HostedModelConfigs(ctx)
	if err != nil {
		return params.HostedModelConfigsResultsLegacy{}, errors.Trace(err)
	}
	result := params.HostedModelConfigsResultsLegacy{
		Models: make([]params.HostedModelConfigLegacy, 0, len(results.Models)),
	}
	for i, model := range results.Models {
		if model.Error != nil {
			result.Models[i] = params.HostedModelConfigLegacy{
				Error: model.Error,
			}
			continue
		}
		result.Models[i] = params.HostedModelConfigLegacy{
			Name:      model.Name,
			OwnerTag:  names.NewUserTag(model.Qualifier).String(),
			Config:    model.Config,
			CloudSpec: model.CloudSpec,
		}
	}
	return result, nil
}
