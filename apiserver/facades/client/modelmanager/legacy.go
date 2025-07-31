// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// ModelStatus returns a summary of the model.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (c *ModelManagerAPIV10) ModelStatus(ctx context.Context, req params.Entities) (params.ModelStatusResultsLegacy, error) {
	status, err := c.ModelManagerAPI.ModelStatus(ctx, req)
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
		owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(r.Qualifier))
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

// CreateModel creates a new model using the account and
// model config specified in the args.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (m *ModelManagerAPIV10) CreateModel(ctx context.Context, args params.ModelCreateArgsLegacy) (params.ModelInfoLegacy, error) {
	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return params.ModelInfoLegacy{}, errors.Trace(err)
	}
	createArgs := params.ModelCreateArgs{
		Name:               args.Name,
		Qualifier:          coremodel.QualifierFromUserTag(ownerTag).String(),
		Config:             args.Config,
		CloudTag:           args.CloudTag,
		CloudRegion:        args.CloudRegion,
		CloudCredentialTag: args.CloudCredentialTag,
	}

	info, err := m.ModelManagerAPI.CreateModel(ctx, createArgs)
	if err != nil {
		return params.ModelInfoLegacy{}, errors.Trace(err)
	}
	owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(info.Qualifier))
	if err != nil {
		return params.ModelInfoLegacy{}, errors.Trace(err)
	}
	result := params.ModelInfoLegacy{
		Name:                    info.Name,
		Type:                    info.Type,
		UUID:                    info.UUID,
		ControllerUUID:          info.ControllerUUID,
		IsController:            info.IsController,
		ProviderType:            info.ProviderType,
		CloudTag:                info.CloudTag,
		CloudRegion:             info.CloudRegion,
		CloudCredentialTag:      info.CloudCredentialTag,
		CloudCredentialValidity: info.CloudCredentialValidity,
		OwnerTag:                owner.String(),
		Life:                    info.Life,
		Status:                  info.Status,
		Users:                   info.Users,
		Machines:                info.Machines,
		SecretBackends:          info.SecretBackends,
		Migration:               info.Migration,
		AgentVersion:            info.AgentVersion,
		SupportedFeatures:       info.SupportedFeatures,
	}
	return result, nil
}

// ListModelSummaries returns models that the specified user
// has access to in the current server.  Controller admins (superuser)
// can list models for any user.  Other users
// can only ask about their own models.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (m *ModelManagerAPIV10) ListModelSummaries(ctx context.Context, req params.ModelSummariesRequest) (params.ModelSummaryResultsLegacy, error) {
	summary, err := m.ModelManagerAPI.ListModelSummaries(ctx, req)
	if err != nil {
		return params.ModelSummaryResultsLegacy{}, errors.Trace(err)
	}
	result := params.ModelSummaryResultsLegacy{
		Results: make([]params.ModelSummaryResultLegacy, len(summary.Results)),
	}
	for i, r := range summary.Results {
		if r.Error != nil {
			result.Results[i].Error = r.Error
			continue
		}
		owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(r.Result.Qualifier))
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		result.Results[i].Result = &params.ModelSummaryLegacy{
			Name:               r.Result.Name,
			UUID:               r.Result.UUID,
			Type:               r.Result.Type,
			ControllerUUID:     r.Result.ControllerUUID,
			IsController:       r.Result.IsController,
			ProviderType:       r.Result.ProviderType,
			CloudTag:           r.Result.CloudTag,
			CloudRegion:        r.Result.CloudRegion,
			CloudCredentialTag: r.Result.CloudCredentialTag,
			OwnerTag:           owner.String(),
			Life:               r.Result.Life,
			Status:             r.Result.Status,
			UserAccess:         r.Result.UserAccess,
			UserLastConnection: r.Result.UserLastConnection,
			Counts:             r.Result.Counts,
			Migration:          r.Result.Migration,
			AgentVersion:       r.Result.AgentVersion,
		}
	}
	return result, nil
}

// ModelInfo returns information about the specified models.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (m *ModelManagerAPIV10) ModelInfo(ctx context.Context, args params.Entities) (params.ModelInfoResultsLegacy, error) {
	info, err := m.ModelManagerAPI.ModelInfo(ctx, args)
	if err != nil {
		return params.ModelInfoResultsLegacy{}, errors.Trace(err)
	}
	results := params.ModelInfoResultsLegacy{
		Results: make([]params.ModelInfoResultLegacy, len(info.Results)),
	}
	for i, r := range info.Results {
		if r.Error != nil {
			results.Results[i].Error = r.Error
			continue
		}
		result := r.Result
		owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(result.Qualifier))
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &params.ModelInfoLegacy{
			Name:                    result.Name,
			Type:                    result.Type,
			UUID:                    result.UUID,
			ControllerUUID:          result.ControllerUUID,
			IsController:            result.IsController,
			ProviderType:            result.ProviderType,
			CloudTag:                result.CloudTag,
			CloudRegion:             result.CloudRegion,
			CloudCredentialTag:      result.CloudCredentialTag,
			CloudCredentialValidity: result.CloudCredentialValidity,
			OwnerTag:                owner.String(),
			Life:                    result.Life,
			Status:                  result.Status,
			Users:                   result.Users,
			Machines:                result.Machines,
			SecretBackends:          result.SecretBackends,
			Migration:               result.Migration,
			AgentVersion:            result.AgentVersion,
			SupportedFeatures:       result.SupportedFeatures,
		}
	}
	return results, nil
}

// ListModels returns the models that the specified user
// has access to in the current server.  Controller admins (superuser)
// can list models for any user.  Other users
// can only ask about their own models.
// It converts results which have a model qualifier to instead use
// an owner tag.
func (m *ModelManagerAPIV10) ListModels(ctx context.Context, userEntity params.Entity) (params.UserModelListLegacy, error) {
	models, err := m.ModelManagerAPI.ListModels(ctx, userEntity)
	if err != nil {
		return params.UserModelListLegacy{}, errors.Trace(err)
	}

	result := params.UserModelListLegacy{
		UserModels: make([]params.UserModelLegacy, len(models.UserModels)),
	}
	for i, model := range models.UserModels {
		owner, err := params.ApproximateUserTagFromQualifier(coremodel.Qualifier(model.Qualifier))
		if err != nil {
			return params.UserModelListLegacy{}, apiservererrors.ServerError(err)
		}
		result.UserModels[i] = params.UserModelLegacy{
			ModelLegacy: params.ModelLegacy{
				Name:     model.Name,
				UUID:     model.UUID,
				Type:     model.Type,
				OwnerTag: owner.String(),
			},
			LastConnection: model.LastConnection,
		}
	}
	return result, nil
}
