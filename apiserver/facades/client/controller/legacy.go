// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// ModelStatus returns a summary of the model.
// It converts results which have a model qualifier to instead use
// an owner tag.
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
		owner, err := params.ApproximateUserTagFromQualifier(model.Qualifier(r.Qualifier))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i] = params.ModelStatusLegacy{
			ModelTag:           r.ModelTag,
			Life:               r.Life,
			Type:               r.Type,
			HostedMachineCount: r.HostedMachineCount,
			ApplicationCount:   r.ApplicationCount,
			UnitCount:          r.UnitCount,
			OwnerTag:           owner.String(),
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
// It converts results which have a model qualifier to instead use
// an owner tag.
func (c *ControllerAPIV12) AllModels(ctx context.Context) (params.UserModelListLegacy, error) {
	models, err := c.ControllerAPI.AllModels(ctx)
	if err != nil {
		return params.UserModelListLegacy{}, errors.Trace(err)
	}
	result := params.UserModelListLegacy{
		UserModels: make([]params.UserModelLegacy, 0, len(models.UserModels)),
	}
	for i, m := range models.UserModels {
		owner, err := params.ApproximateUserTagFromQualifier(model.Qualifier(m.Qualifier))
		if err != nil {
			return params.UserModelListLegacy{}, apiservererrors.ServerError(err)
		}
		result.UserModels[i] = params.UserModelLegacy{
			ModelLegacy: params.ModelLegacy{
				Name:     m.Name,
				UUID:     m.UUID,
				Type:     m.Type,
				OwnerTag: owner.String(),
			},
			LastConnection: m.LastConnection,
		}
	}
	return result, nil
}

// ListBlockedModels returns a list of all models on the controller
// which have a block in place.  The resulting slice is sorted by model
// name, then owner. Callers must be controller administrators to retrieve the
// list.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (c *ControllerAPIV12) ListBlockedModels(ctx context.Context) (params.ModelBlockInfoListLegacy, error) {
	models, err := c.ControllerAPI.ListBlockedModels(ctx)
	if err != nil {
		return params.ModelBlockInfoListLegacy{}, errors.Trace(err)
	}
	result := params.ModelBlockInfoListLegacy{
		Models: make([]params.ModelBlockInfoLegacy, 0, len(models.Models)),
	}
	for i, m := range models.Models {
		owner, err := params.ApproximateUserTagFromQualifier(model.Qualifier(m.Qualifier))
		if err != nil {
			return params.ModelBlockInfoListLegacy{}, apiservererrors.ServerError(err)
		}
		result.Models[i] = params.ModelBlockInfoLegacy{
			Name:     m.Name,
			UUID:     m.UUID,
			OwnerTag: owner.String(),
			Blocks:   m.Blocks,
		}
	}
	return result, nil
}

// HostedModelConfigs returns all the information that the client needs in
// order to connect directly with the host model's provider and destroy it
// directly.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (c *ControllerAPIV12) HostedModelConfigs(ctx context.Context) (params.HostedModelConfigsResultsLegacy, error) {
	results, err := c.ControllerAPI.HostedModelConfigs(ctx)
	if err != nil {
		return params.HostedModelConfigsResultsLegacy{}, errors.Trace(err)
	}
	result := params.HostedModelConfigsResultsLegacy{
		Models: make([]params.HostedModelConfigLegacy, 0, len(results.Models)),
	}
	for i, m := range results.Models {
		if m.Error != nil {
			result.Models[i] = params.HostedModelConfigLegacy{
				Error: m.Error,
			}
			continue
		}
		owner, err := params.ApproximateUserTagFromQualifier(model.Qualifier(m.Qualifier))
		if err != nil {
			result.Models[i] = params.HostedModelConfigLegacy{
				Error: apiservererrors.ServerError(err),
			}
			continue
		}
		result.Models[i] = params.HostedModelConfigLegacy{
			Name:      m.Name,
			OwnerTag:  owner.String(),
			Config:    m.Config,
			CloudSpec: m.CloudSpec,
		}
	}
	return result, nil
}
