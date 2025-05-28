// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/rpc/params"
)

// ModelManagerAPIV10 implements the model manager V10.
type ModelManagerAPIV10 struct {
	*ModelManagerAPI
}

// ModelStatus returns a summary of the model.
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

// CreateModel creates a new model using the account and
// model config specified in the args.
func (m *ModelManagerAPIV10) CreateModel(ctx context.Context, args params.ModelCreateArgsLegacy) (params.ModelInfoLegacy, error) {
	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return params.ModelInfoLegacy{}, errors.Trace(err)
	}
	createArgs := params.ModelCreateArgs{
		Name:               args.Name,
		Qualifier:          ownerTag.Id(),
		Config:             args.Config,
		CloudTag:           args.CloudTag,
		CloudRegion:        args.CloudRegion,
		CloudCredentialTag: args.CloudCredentialTag,
	}

	info, err := m.ModelManagerAPI.CreateModel(ctx, createArgs)
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
		OwnerTag:                names.NewUserTag(info.Qualifier).String(),
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
			OwnerTag:           names.NewUserTag(r.Result.Qualifier).String(),
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
			OwnerTag:                names.NewUserTag(result.Qualifier).String(),
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
func (m *ModelManagerAPIV10) ListModels(ctx context.Context, userEntity params.Entity) (params.UserModelListLegacy, error) {
	models, err := m.ModelManagerAPI.ListModels(ctx, userEntity)
	if err != nil {
		return params.UserModelListLegacy{}, errors.Trace(err)
	}

	result := params.UserModelListLegacy{
		UserModels: make([]params.UserModelLegacy, len(models.UserModels)),
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
