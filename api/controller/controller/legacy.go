// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc/params"
)

// allModelsCompat gets model info but adapts the
// result to convert a model owner into a model qualifier.
func (c *Client) allModelsCompat(ctx context.Context) ([]base.UserModel, error) {
	var models params.UserModelListLegacy
	err := c.facade.FacadeCall(ctx, "AllModels", nil, &models)
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

// hostedModelConfigsCompat gets model config info but adapts the
// result to convert a model owner into a model qualifier.
func (c *Client) hostedModelConfigsCompat(ctx context.Context) ([]HostedConfig, error) {
	result := params.HostedModelConfigsResultsLegacy{}
	err := c.facade.FacadeCall(ctx, "HostedModelConfigs", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If we get to here, we have some values. Each value may or
	// may not have an error, but it should at least have a name
	// and owner.
	hostedConfigs := make([]HostedConfig, len(result.Models))
	for i, modelConfig := range result.Models {
		hostedConfigs[i].Name = modelConfig.Name
		tag, err := names.ParseUserTag(modelConfig.OwnerTag)
		if err != nil {
			hostedConfigs[i].Error = errors.Trace(err)
			continue
		}
		hostedConfigs[i].Qualifier = tag.Id()
		if modelConfig.Error != nil {
			hostedConfigs[i].Error = errors.Trace(modelConfig.Error)
			continue
		}
		hostedConfigs[i].Config = modelConfig.Config
		spec, err := c.MakeCloudSpec(modelConfig.CloudSpec)
		if err != nil {
			hostedConfigs[i].Error = errors.Trace(err)
			continue
		}
		hostedConfigs[i].CloudSpec = spec
	}
	return hostedConfigs, err
}
