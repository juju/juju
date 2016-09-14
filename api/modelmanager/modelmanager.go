// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/permission"
)

var logger = loggo.GetLogger("juju.api.modelmanager")

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ModelManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Close closes the api connection.
func (c *Client) Close() error {
	return c.ClientFacade.Close()
}

// CreateModel creates a new model using the model config,
// cloud region and credential specified in the args.
func (c *Client) CreateModel(
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
	err := c.facade.FacadeCall("CreateModel", createArgs, &modelInfo)
	if err != nil {
		return result, errors.Trace(err)
	}
	return convertParamsModelInfo(modelInfo)
}

func convertParamsModelInfo(modelInfo params.ModelInfo) (base.ModelInfo, error) {
	cloud, err := names.ParseCloudTag(modelInfo.CloudTag)
	if err != nil {
		return base.ModelInfo{}, err
	}
	var credential string
	if modelInfo.CloudCredentialTag != "" {
		credTag, err := names.ParseCloudCredentialTag(modelInfo.CloudCredentialTag)
		if err != nil {
			return base.ModelInfo{}, err
		}
		credential = credTag.Id()
	}
	ownerTag, err := names.ParseUserTag(modelInfo.OwnerTag)
	if err != nil {
		return base.ModelInfo{}, err
	}
	result := base.ModelInfo{
		Name:            modelInfo.Name,
		UUID:            modelInfo.UUID,
		ControllerUUID:  modelInfo.ControllerUUID,
		ProviderType:    modelInfo.ProviderType,
		DefaultSeries:   modelInfo.DefaultSeries,
		Cloud:           cloud.Id(),
		CloudRegion:     modelInfo.CloudRegion,
		CloudCredential: credential,
		Owner:           ownerTag.Id(),
		Life:            string(modelInfo.Life),
	}
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
			Id:         m.Id,
			InstanceId: m.InstanceId,
			HasVote:    m.HasVote,
			WantsVote:  m.WantsVote,
			Status:     m.Status,
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
func (c *Client) ListModels(user string) ([]base.UserModel, error) {
	var models params.UserModelList
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
	entity := params.Entity{names.NewUserTag(user).String()}
	err := c.facade.FacadeCall("ListModels", entity, &models)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]base.UserModel, len(models.UserModels))
	for i, model := range models.UserModels {
		owner, err := names.ParseUserTag(model.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", model.OwnerTag, i)
		}
		result[i] = base.UserModel{
			Name:           model.Name,
			UUID:           model.UUID,
			Owner:          owner.Canonical(),
			LastConnection: model.LastConnection,
		}
	}
	return result, nil
}

func (c *Client) ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error) {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	var results params.ModelInfoResults
	err := c.facade.FacadeCall("ModelInfo", entities, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// DumpModel returns the serialized database agnostic model representation.
func (c *Client) DumpModel(model names.ModelTag) (map[string]interface{}, error) {
	var results params.MapResults
	entities := params.Entities{
		Entities: []params.Entity{{Tag: model.String()}},
	}

	err := c.facade.FacadeCall("DumpModels", entities, &results)
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

// DumpModelDB returns all relevant mongo documents for the model.
func (c *Client) DumpModelDB(model names.ModelTag) (map[string]interface{}, error) {
	var results params.MapResults
	entities := params.Entities{
		Entities: []params.Entity{{Tag: model.String()}},
	}

	err := c.facade.FacadeCall("DumpModelsDB", entities, &results)
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
func (c *Client) DestroyModel(tag names.ModelTag) error {
	var results params.ErrorResults
	entities := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	if err := c.facade.FacadeCall("DestroyModels", entities, &results); err != nil {
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
func (c *Client) GrantModel(user, access string, modelUUIDs ...string) error {
	return c.modifyModelUser(params.GrantModelAccess, user, access, modelUUIDs)
}

// RevokeModel revokes a user's access to the specified models.
func (c *Client) RevokeModel(user, access string, modelUUIDs ...string) error {
	return c.modifyModelUser(params.RevokeModelAccess, user, access, modelUUIDs)
}

func (c *Client) modifyModelUser(action params.ModelAction, user, access string, modelUUIDs []string) error {
	var args params.ModifyModelAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	modelAccess := permission.Access(access)
	if err := permission.ValidateModelAccess(modelAccess); err != nil {
		return errors.Trace(err)
	}
	for _, model := range modelUUIDs {
		if !names.IsValidModel(model) {
			return errors.Errorf("invalid model: %q", model)
		}
		modelTag := names.NewModelTag(model)
		args.Changes = append(args.Changes, params.ModifyModelAccess{
			UserTag:  userTag.String(),
			Action:   action,
			Access:   params.UserAccessPermission(modelAccess),
			ModelTag: modelTag.String(),
		})
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ModifyModelAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf("model %q is already shared with %q", modelUUIDs[i], userTag.Canonical())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}

// ModelDefaults returns the default values for various sources used when
// creating a new model.
func (c *Client) ModelDefaults() (config.ModelDefaultAttributes, error) {
	result := params.ModelDefaultsResult{}
	err := c.facade.FacadeCall("ModelDefaults", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
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
func (c *Client) SetModelDefaults(cloud, region string, config map[string]interface{}) error {
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
	err := c.facade.FacadeCall("SetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}

// UnsetModelDefaults removes the specified default model config values.
func (c *Client) UnsetModelDefaults(cloud, region string, keys ...string) error {
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
	err := c.facade.FacadeCall("UnsetModelDefaults", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
