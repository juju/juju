// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/config"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

var logger = internallogger.GetLogger("juju.api.modelmanager")

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	*common.ModelStatusAPI
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelManager", options...)
	return &Client{
		ClientFacade:   frontend,
		facade:         backend,
		ModelStatusAPI: common.NewModelStatusAPI(backend),
	}
}

// CreateModel creates a new model using the model config,
// cloud region and credential specified in the args.
func (c *Client) CreateModel(
	ctx context.Context,
	name, owner, cloud, cloudRegion string,
	cloudCredential names.CloudCredentialTag,
	config map[string]interface{},
) (base.ModelInfo, error) {
	var result base.ModelInfo
	if !names.IsValidUser(owner) {
		return result, errors.Errorf("invalid owner name %q", owner)
	}
	var cloudTag string
	if cloud != "" {
		if !names.IsValidCloud(cloud) {
			return result, errors.Errorf("invalid cloud name %q", cloud)
		}
		cloudTag = names.NewCloudTag(cloud).String()
	}
	var cloudCredentialTag string
	if cloudCredential != (names.CloudCredentialTag{}) {
		cloudCredentialTag = cloudCredential.String()
	}
	createArgs := params.ModelCreateArgs{
		Name:               name,
		OwnerTag:           names.NewUserTag(owner).String(),
		Config:             config,
		CloudTag:           cloudTag,
		CloudRegion:        cloudRegion,
		CloudCredentialTag: cloudCredentialTag,
	}
	var modelInfo params.ModelInfo
	err := c.facade.FacadeCall(ctx, "CreateModel", createArgs, &modelInfo)
	if err != nil {
		return result, errors.Trace(err)
	}
	return convertParamsModelInfo(modelInfo)
}

func convertParamsModelInfo(modelInfo params.ModelInfo) (base.ModelInfo, error) {
	cloud, err := names.ParseCloudTag(modelInfo.CloudTag)
	if err != nil {
		return base.ModelInfo{}, errors.Trace(err)
	}
	var credential string
	if modelInfo.CloudCredentialTag != "" {
		credTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
		if err != nil {
			return base.ModelInfo{}, errors.Trace(err)
		}
		credential = credTag.Id()
	}
	ownerTag, err := names.ParseUserTag(modelInfo.OwnerTag)
	if err != nil {
		return base.ModelInfo{}, errors.Trace(err)
	}
	result := base.ModelInfo{
		Name:            modelInfo.Name,
		UUID:            modelInfo.UUID,
		ControllerUUID:  modelInfo.ControllerUUID,
		IsController:    modelInfo.IsController,
		ProviderType:    modelInfo.ProviderType,
		Cloud:           cloud.Id(),
		CloudRegion:     modelInfo.CloudRegion,
		CloudCredential: credential,
		Owner:           ownerTag.Id(),
		Life:            modelInfo.Life,
		AgentVersion:    modelInfo.AgentVersion,
	}
	modelType := modelInfo.Type
	if modelType == "" {
		modelType = model.IAAS.String()
	}
	result.Type = model.ModelType(modelType)
	result.Status = base.Status{
		Status: modelInfo.Status.Status,
		Info:   modelInfo.Status.Info,
		Data:   make(map[string]interface{}),
		Since:  modelInfo.Status.Since,
	}
	for k, v := range modelInfo.Status.Data {
		result.Status.Data[k] = v
	}
	result.Users = make([]base.UserInfo, len(modelInfo.Users))
	for i, u := range modelInfo.Users {
		result.Users[i] = base.UserInfo{
			UserName:       u.UserName,
			DisplayName:    u.DisplayName,
			Access:         string(u.Access),
			LastConnection: u.LastConnection,
		}
	}
	result.Machines = make([]base.Machine, len(modelInfo.Machines))
	for i, m := range modelInfo.Machines {
		machine := base.Machine{
			Id:          m.Id,
			InstanceId:  m.InstanceId,
			DisplayName: m.DisplayName,
			HasVote:     m.HasVote,
			WantsVote:   m.WantsVote,
			Status:      m.Status,
			HAPrimary:   m.HAPrimary,
		}
		if m.Hardware != nil {
			machine.Hardware = &instance.HardwareCharacteristics{
				Arch:             m.Hardware.Arch,
				Mem:              m.Hardware.Mem,
				RootDisk:         m.Hardware.RootDisk,
				CpuCores:         m.Hardware.Cores,
				CpuPower:         m.Hardware.CpuPower,
				Tags:             m.Hardware.Tags,
				AvailabilityZone: m.Hardware.AvailabilityZone,
			}
		}
		result.Machines[i] = machine
	}
	return result, nil
}

// ListModels returns the models that the specified user
// has access to in the current server.  Only that controller owner
// can list models for any user (at this stage).  Other users
// can only ask about their own models.
func (c *Client) ListModels(ctx context.Context, user string) ([]base.UserModel, error) {
	var models params.UserModelList
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
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
			UUID:           usermodel.UUID,
			Type:           modelType,
			Owner:          owner.Id(),
			LastConnection: usermodel.LastConnection,
		}
	}
	return result, nil
}

func (c *Client) ListModelSummaries(ctx context.Context, user string, all bool) ([]base.UserModelSummary, error) {
	var out params.ModelSummaryResults
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
	in := params.ModelSummariesRequest{UserTag: names.NewUserTag(user).String(), All: all}
	err := c.facade.FacadeCall(ctx, "ListModelSummaries", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	summaries := make([]base.UserModelSummary, len(out.Results))
	for i, r := range out.Results {
		if r.Error != nil {
			// cope with typed error
			summaries[i] = base.UserModelSummary{Error: errors.Trace(r.Error)}
			continue
		}
		summary := r.Result
		modelType := model.ModelType(summary.Type)
		if modelType == "" {
			modelType = model.IAAS
		}
		summaries[i] = base.UserModelSummary{
			Name:               summary.Name,
			UUID:               summary.UUID,
			Type:               modelType,
			ControllerUUID:     summary.ControllerUUID,
			IsController:       summary.IsController,
			ProviderType:       summary.ProviderType,
			CloudRegion:        summary.CloudRegion,
			Life:               summary.Life,
			ModelUserAccess:    string(summary.UserAccess),
			UserLastConnection: summary.UserLastConnection,
			Counts:             make([]base.EntityCount, len(summary.Counts)),
			AgentVersion:       summary.AgentVersion,
		}
		for pos, count := range summary.Counts {
			summaries[i].Counts[pos] = base.EntityCount{string(count.Entity), count.Count}
		}
		summaries[i].Status = base.Status{
			Status: summary.Status.Status,
			Info:   summary.Status.Info,
			Data:   make(map[string]interface{}),
			Since:  summary.Status.Since,
		}
		for k, v := range summary.Status.Data {
			summaries[i].Status.Data[k] = v
		}
		if owner, err := names.ParseUserTag(summary.OwnerTag); err != nil {
			summaries[i].Error = errors.Annotatef(err, "while parsing model owner tag")
			continue
		} else {
			summaries[i].Owner = owner.Id()
		}
		if cloud, err := names.ParseCloudTag(summary.CloudTag); err != nil {
			summaries[i].Error = errors.Annotatef(err, "while parsing model cloud tag")
			continue
		} else {
			summaries[i].Cloud = cloud.Id()
		}
		if summary.CloudCredentialTag != "" {
			if credTag, err := names.ParseCloudCredentialTag(summary.CloudCredentialTag); err != nil {
				summaries[i].Error = errors.Annotatef(err, "while parsing model cloud credential tag")
				continue
			} else {
				summaries[i].CloudCredential = credTag.Id()
			}
		}
		if summary.Migration != nil {
			summaries[i].Migration = &base.MigrationSummary{
				Status:    summary.Migration.Status,
				StartTime: summary.Migration.Start,
				EndTime:   summary.Migration.End,
			}
		}
	}
	return summaries, nil
}

func (c *Client) ModelInfo(ctx context.Context, tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	var results params.ModelInfoResults
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
	return results.Results, nil
}

// DumpModel returns the serialized database agnostic model representation.
func (c *Client) DumpModel(ctx context.Context, model names.ModelTag) (map[string]interface{}, error) {
	var results params.StringResults
	entities := params.DumpModelRequest{
		Entities: []params.Entity{{Tag: model.String()}},
	}

	err := c.facade.FacadeCall(ctx, "DumpModels", entities, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("unexpected result count: %d", count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	// Parse back into a map.
	var asMap map[string]interface{}
	err = yaml.Unmarshal([]byte(result.Result), &asMap)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return asMap, nil
}

// DumpModelDB returns all relevant mongo documents for the model.
func (c *Client) DumpModelDB(ctx context.Context, model names.ModelTag) (map[string]interface{}, error) {
	var results params.MapResults
	entities := params.Entities{
		Entities: []params.Entity{{Tag: model.String()}},
	}

	err := c.facade.FacadeCall(ctx, "DumpModelsDB", entities, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if count := len(results.Results); count != 1 {
		return nil, errors.Errorf("unexpected result count: %d", count)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return result.Result, nil
}

// DestroyModel puts the specified model into a "dying" state, which will
// cause the model's resources to be cleaned up, after which the model will
// be removed.
func (c *Client) DestroyModel(ctx context.Context, tag names.ModelTag, destroyStorage, force *bool, maxWait *time.Duration, timeout *time.Duration) error {
	arg := params.DestroyModelParams{
		ModelTag:       tag.String(),
		DestroyStorage: destroyStorage,
		Force:          force,
		MaxWait:        maxWait,
		Timeout:        timeout,
	}
	args := params.DestroyModelsParams{Models: []params.DestroyModelParams{arg}}
	var results params.ErrorResults
	if err := c.facade.FacadeCall(ctx, "DestroyModels", args, &results); err != nil {
		return errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return errors.Trace(err)
	}
	return nil
}

// GrantModel grants a user access to the specified models.
func (c *Client) GrantModel(ctx context.Context, user, access string, modelUUIDs ...string) error {
	return c.modifyModelUser(ctx, params.GrantModelAccess, user, access, modelUUIDs)
}

// RevokeModel revokes a user's access to the specified models.
func (c *Client) RevokeModel(ctx context.Context, user, access string, modelUUIDs ...string) error {
	return c.modifyModelUser(ctx, params.RevokeModelAccess, user, access, modelUUIDs)
}

func (c *Client) modifyModelUser(ctx context.Context, action params.ModelAction, user, access string, modelUUIDs []string) error {
	var args params.ModifyModelAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	modelAccess := permission.Access(access)
	if err := permission.ValidateModelAccess(modelAccess); err != nil {
		return errors.Trace(err)
	}
	for _, m := range modelUUIDs {
		if !names.IsValidModel(m) {
			return errors.Errorf("invalid model: %q", m)
		}
		modelTag := names.NewModelTag(m)
		args.Changes = append(args.Changes, params.ModifyModelAccess{
			UserTag:  userTag.String(),
			Action:   action,
			Access:   params.UserAccessPermission(modelAccess),
			ModelTag: modelTag.String(),
		})
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall(ctx, "ModifyModelAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf(context.TODO(), "model %q is already shared with %q", modelUUIDs[i], userTag.Id())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// ModelDefaults returns the default values for various sources used when
// creating a new model on the specified cloud.
func (c *Client) ModelDefaults(ctx context.Context, cloud string) (config.ModelDefaultAttributes, error) {
	results := params.ModelDefaultsResults{}
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag(cloud).String()}},
	}
	err := c.facade.FacadeCall(ctx, "ModelDefaultsForClouds", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	values := make(config.ModelDefaultAttributes)
	for name, val := range result.Config {
		setting := config.AttributeDefaultValues{
			Default:    val.Default,
			Controller: val.Controller,
		}
		for _, region := range val.Regions {
			setting.Regions = append(setting.Regions, config.RegionDefaultValue{
				Name:  region.RegionName,
				Value: region.Value})
		}
		values[name] = setting
	}
	return values, nil
}

// SetModelDefaults updates the specified default model config values.
func (c *Client) SetModelDefaults(ctx context.Context, cloud, region string, config map[string]interface{}) error {
	var cloudTag string
	if cloud != "" {
		cloudTag = names.NewCloudTag(cloud).String()
	}
	args := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config:      config,
			CloudTag:    cloudTag,
			CloudRegion: region,
		}},
	}
	var result params.ErrorResults
	err := c.facade.FacadeCall(ctx, "SetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// UnsetModelDefaults removes the specified default model config values.
func (c *Client) UnsetModelDefaults(ctx context.Context, cloud, region string, keys ...string) error {
	var cloudTag string
	if cloud != "" {
		cloudTag = names.NewCloudTag(cloud).String()
	}
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys:        keys,
			CloudTag:    cloudTag,
			CloudRegion: region,
		}},
	}
	var result params.ErrorResults
	err := c.facade.FacadeCall(ctx, "UnsetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// ChangeModelCredential replaces cloud credential for a given model with the provided one.
func (c *Client) ChangeModelCredential(ctx context.Context, model names.ModelTag, credential names.CloudCredentialTag) error {
	var out params.ErrorResults
	in := params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: model.String(), CloudCredentialTag: credential.String()},
		},
	}

	err := c.facade.FacadeCall(ctx, "ChangeModelCredential", in, &out)
	if err != nil {
		return errors.Trace(err)
	}
	return out.OneError()
}

// ValidateModelUpgrade checks to see if it's possible to upgrade a model,
// before actually attempting to do the real environ-upgrade.
func (c *Client) ValidateModelUpgrade(ctx context.Context, model names.ModelTag, force bool) error {
	args := params.ValidateModelUpgradeParams{
		Models: []params.ModelParam{{
			ModelTag: model.String(),
		}},
		Force: force,
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall(ctx, "ValidateModelUpgrades", args, &results); err != nil {
		return errors.Trace(err)
	}
	if num := len(results.Results); num != 1 {
		return errors.Errorf("expected one result, got %d", num)
	}
	return apiservererrors.RestoreError(results.OneError())
}
