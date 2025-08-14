// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// createModelCompat creates a model with params which use a model owner
// rather than a model qualifier. It also adapts the result to convert a
// model owner into a model qualifier.
func (c *Client) createModelCompat(
	ctx context.Context,
	createArgs params.ModelCreateArgs,
) (base.ModelInfo, error) {
	createArgsLegacy := params.ModelCreateArgsLegacy{
		Name:               createArgs.Name,
		OwnerTag:           names.NewUserTag(createArgs.Qualifier).String(),
		Config:             createArgs.Config,
		CloudTag:           createArgs.CloudTag,
		CloudRegion:        createArgs.CloudRegion,
		CloudCredentialTag: createArgs.CloudCredentialTag,
	}
	var result params.ModelInfoLegacy
	err := c.facade.FacadeCall(ctx, "CreateModel", createArgsLegacy, &result)
	if err != nil {
		return base.ModelInfo{}, errors.Trace(err)
	}
	ownerTag, err := names.ParseUserTag(result.OwnerTag)
	if err != nil {
		return base.ModelInfo{}, errors.Trace(err)
	}
	info := params.ModelInfo{
		Name:                    result.Name,
		Qualifier:               ownerTag.Id(),
		Type:                    result.Type,
		UUID:                    result.UUID,
		ControllerUUID:          result.ControllerUUID,
		IsController:            result.IsController,
		ProviderType:            result.ProviderType,
		CloudTag:                result.CloudTag,
		CloudRegion:             result.CloudRegion,
		CloudCredentialTag:      result.CloudCredentialTag,
		CloudCredentialValidity: result.CloudCredentialValidity,
		Life:                    result.Life,
		Status:                  result.Status,
		Users:                   result.Users,
		Machines:                result.Machines,
		SecretBackends:          result.SecretBackends,
		Migration:               result.Migration,
		AgentVersion:            result.AgentVersion,
		SupportedFeatures:       result.SupportedFeatures,
	}
	return convertParamsModelInfo(info)
}

// listModelsCompat lists models for a user but adapts the result to convert
// a model owner into a model qualifier.
func (c *Client) listModelsCompat(ctx context.Context, user string) ([]base.UserModel, error) {
	var models params.UserModelListLegacy
	entity := params.Entity{names.NewUserTag(user).String()}
	err := c.facade.FacadeCall(ctx, "ListModels", entity, &models)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]base.UserModel, len(models.UserModels))
	for i, usermodel := range models.UserModels {
		owner, err := names.ParseUserTag(usermodel.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", usermodel.OwnerTag, i)
		}
		modelType := model.ModelType(usermodel.Type)
		if modelType == "" {
			modelType = model.IAAS
		}
		result[i] = base.UserModel{
			Name:           usermodel.Name,
			Qualifier:      model.QualifierFromUserTag(owner),
			UUID:           usermodel.UUID,
			Type:           modelType,
			LastConnection: usermodel.LastConnection,
		}
	}
	return result, nil
}

// listModelSummariesCompat lists model summaries for a user but adapts the
// result to convert a model owner into a model qualifier.
func (c *Client) listModelSummariesCompat(ctx context.Context, user string, all bool) ([]base.UserModelSummary, error) {
	var out params.ModelSummaryResultsLegacy
	in := params.ModelSummariesRequest{UserTag: names.NewUserTag(user).String(), All: all}
	err := c.facade.FacadeCall(ctx, "ListModelSummaries", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results := make([]params.ModelSummaryResult, len(out.Results))
	for i, r := range out.Results {
		if r.Error != nil {
			results[i].Error = r.Error
			continue
		}
		result := r.Result
		var qualifier string
		if owner, err := names.ParseUserTag(result.OwnerTag); err != nil {
			results[i].Error = &params.Error{
				Message: fmt.Sprintf("parsing model owner tag: %v", err),
			}
			continue
		} else {
			qualifier = owner.Id()
		}
		results[i] = params.ModelSummaryResult{
			Result: &params.ModelSummary{
				Name:               result.Name,
				Qualifier:          qualifier,
				UUID:               result.UUID,
				Type:               result.Type,
				ControllerUUID:     result.ControllerUUID,
				IsController:       result.IsController,
				ProviderType:       result.ProviderType,
				CloudTag:           result.CloudTag,
				CloudRegion:        result.CloudRegion,
				CloudCredentialTag: result.CloudCredentialTag,
				Life:               result.Life,
				Status:             result.Status,
				UserAccess:         result.UserAccess,
				UserLastConnection: result.UserLastConnection,
				Counts:             result.Counts,
				Migration:          result.Migration,
				AgentVersion:       result.AgentVersion,
			},
		}
	}
	return c.composeModelSummaries(results)
}

// modelInfoCompat lists model summaries for a user but adapts the
// result to convert a model owner into a model qualifier.
func (c *Client) modelInfoCompat(ctx context.Context, tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	var results params.ModelInfoResultsLegacy
	err := c.facade.FacadeCall(ctx, "ModelInfo", entities, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	for i := range results.Results {
		if results.Results[i].Error != nil {
			continue
		}
		if results.Results[i].Result.Type == "" {
			results.Results[i].Result.Type = model.IAAS.String()
		}
	}
	info := make([]params.ModelInfoResult, len(results.Results))
	for i, r := range results.Results {
		result := r.Result
		ownerTag, err := names.ParseUserTag(result.OwnerTag)
		if err != nil {
			info[i].Error = &params.Error{
				Message: fmt.Sprintf("parsing model owner tag: %v", err),
			}
			continue
		}
		info[i] = params.ModelInfoResult{
			Result: &params.ModelInfo{
				Name:                    result.Name,
				Qualifier:               ownerTag.Id(),
				Type:                    result.Type,
				UUID:                    result.UUID,
				ControllerUUID:          result.ControllerUUID,
				IsController:            result.IsController,
				ProviderType:            result.ProviderType,
				CloudTag:                result.CloudTag,
				CloudRegion:             result.CloudRegion,
				CloudCredentialTag:      result.CloudCredentialTag,
				CloudCredentialValidity: result.CloudCredentialValidity,
				Life:                    result.Life,
				Status:                  result.Status,
				Users:                   result.Users,
				Machines:                result.Machines,
				SecretBackends:          result.SecretBackends,
				Migration:               result.Migration,
				AgentVersion:            result.AgentVersion,
				SupportedFeatures:       result.SupportedFeatures,
			},
		}
	}
	return info, nil
}
