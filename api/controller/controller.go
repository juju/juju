// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"encoding/json"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/permission"
)

// Client provides methods that the Juju client command uses to interact
// with the Juju controller.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	*common.ControllerConfigAPI
	*common.ModelStatusAPI
	*cloudspec.CloudSpecAPI
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Controller")
	return &Client{
		ClientFacade:        frontend,
		facade:              backend,
		ControllerConfigAPI: common.NewControllerConfig(backend),
		ModelStatusAPI:      common.NewModelStatusAPI(backend),
	}
}

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c *Client) AllModels() ([]base.UserModel, error) {
	var models params.UserModelList
	err := c.facade.FacadeCall("AllModels", nil, &models)
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

// CloudSpec returns a CloudSpec for the specified model.
func (c *Client) CloudSpec(modelTag names.ModelTag) (environs.CloudSpec, error) {
	api := cloudspec.NewCloudSpecAPI(c.facade, modelTag)
	return api.CloudSpec()
}

// ModelConfig returns all model settings for the
// controller model.
func (c *Client) ModelConfig() (map[string]interface{}, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall("ModelConfig", nil, &result)
	values := make(map[string]interface{})
	for name, val := range result.Config {
		values[name] = val.Value
	}
	return values, err
}

// HostedConfig contains the model config and the cloud spec for that
// model such that direct access to the provider can be used.
type HostedConfig struct {
	Name      string
	Owner     names.UserTag
	Config    map[string]interface{}
	CloudSpec environs.CloudSpec
	Error     error
}

// HostedModelsConfig returns all model settings for the
// controller model.
func (c *Client) HostedModelConfigs() ([]HostedConfig, error) {
	result := params.HostedModelConfigsResults{}
	err := c.facade.FacadeCall("HostedModelConfigs", nil, &result)
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
		hostedConfigs[i].Owner = tag
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

// DestroyControllerParams controls the behaviour of destroying the controller.
type DestroyControllerParams struct {
	// DestroyModels controls whether or not all hosted models should be
	// destroyed. If this is false, and there are non-empty hosted models,
	// an error with the code params.CodeHasHostedModels will be returned.
	DestroyModels bool

	// DestroyStorage controls whether or not storage in the model (and
	// hosted models, if DestroyModels is true) should be destroyed.
	//
	// This is ternary: nil, false, or true. If nil and there is persistent
	// storage in the model (or hosted models), an error with the code
	// params.CodeHasPersistentStorage will be returned.
	DestroyStorage *bool
}

// DestroyController puts the controller model into a "dying" state,
// and removes all non-manager machine instances.
func (c *Client) DestroyController(args DestroyControllerParams) error {
	if c.BestAPIVersion() < 4 {
		if args.DestroyStorage == nil || !*args.DestroyStorage {
			return errors.New("this Juju controller requires DestroyStorage to be true")
		}
		args.DestroyStorage = nil
	}
	return c.facade.FacadeCall("DestroyController", params.DestroyControllerArgs{
		DestroyModels:  args.DestroyModels,
		DestroyStorage: args.DestroyStorage,
	}, nil)
}

// ListBlockedModels returns a list of all models within the controller
// which have at least one block in place.
func (c *Client) ListBlockedModels() ([]params.ModelBlockInfo, error) {
	result := params.ModelBlockInfoList{}
	err := c.facade.FacadeCall("ListBlockedModels", nil, &result)
	return result.Models, err
}

// RemoveBlocks removes all the blocks in the controller.
func (c *Client) RemoveBlocks() error {
	args := params.RemoveBlocksArgs{All: true}
	return c.facade.FacadeCall("RemoveBlocks", args, nil)
}

// WatchAllModels returns an AllWatcher, from which you can request
// the Next collection of Deltas (for all models).
func (c *Client) WatchAllModels() (*api.AllWatcher, error) {
	var info params.AllWatcherId
	if err := c.facade.FacadeCall("WatchAllModels", nil, &info); err != nil {
		return nil, err
	}
	return api.NewAllModelWatcher(c.facade.RawAPICaller(), &info.AllWatcherId), nil
}

// GrantController grants a user access to the controller.
func (c *Client) GrantController(user, access string) error {
	return c.modifyControllerUser(params.GrantControllerAccess, user, access)
}

// RevokeController revokes a user's access to the controller.
func (c *Client) RevokeController(user, access string) error {
	return c.modifyControllerUser(params.RevokeControllerAccess, user, access)
}

func (c *Client) modifyControllerUser(action params.ControllerAction, user, access string) error {
	var args params.ModifyControllerAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	args.Changes = []params.ModifyControllerAccess{{
		UserTag: userTag.String(),
		Action:  action,
		Access:  access,
	}}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ModifyControllerAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	return result.Combine()
}

// GetControllerAccess returns the access level the user has on the controller.
func (c *Client) GetControllerAccess(user string) (permission.Access, error) {
	if !names.IsValidUser(user) {
		return "", errors.Errorf("invalid username: %q", user)
	}
	entities := params.Entities{Entities: []params.Entity{{names.NewUserTag(user).String()}}}
	var results params.UserAccessResults
	err := c.facade.FacadeCall("GetControllerAccess", entities, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return "", errors.Trace(err)
	}
	return permission.Access(results.Results[0].Result.Access), nil
}

// ConfigSet updates the passed controller configuration values. Any
// settings that aren't passed will be left with their previous
// values.
func (c *Client) ConfigSet(values map[string]interface{}) error {
	if c.BestAPIVersion() < 5 {
		return errors.Errorf("this controller version doesn't support updating controller config")
	}
	return errors.Trace(
		c.facade.FacadeCall("ConfigSet", params.ControllerConfigSet{Config: values}, nil),
	)
}

// MigrationSpec holds the details required to start the migration of
// a single model.
type MigrationSpec struct {
	ModelUUID            string
	TargetControllerUUID string
	TargetAddrs          []string
	TargetCACert         string
	TargetUser           string
	TargetPassword       string
	TargetMacaroons      []macaroon.Slice
}

// Validate performs sanity checks on the migration configuration it
// holds.
func (s *MigrationSpec) Validate() error {
	if !names.IsValidModel(s.ModelUUID) {
		return errors.NotValidf("model UUID")
	}
	if !names.IsValidModel(s.TargetControllerUUID) {
		return errors.NotValidf("controller UUID")
	}
	if len(s.TargetAddrs) < 1 {
		return errors.NotValidf("empty target API addresses")
	}
	if !names.IsValidUser(s.TargetUser) {
		return errors.NotValidf("target user")
	}
	if s.TargetPassword == "" && len(s.TargetMacaroons) == 0 {
		return errors.NotValidf("missing authentication secrets")
	}
	return nil
}

// InitiateMigration attempts to start a migration for the specified
// model, returning the migration's ID.
//
// The API server supports starting multiple migrations in one request
// but we don't need that at the client side yet (and may never) so
// this call just supports starting one migration at a time.
func (c *Client) InitiateMigration(spec MigrationSpec) (string, error) {
	if err := spec.Validate(); err != nil {
		return "", errors.Annotatef(err, "client-side validation failed")
	}

	macsJSON, err := macaroonsToJSON(spec.TargetMacaroons)
	if err != nil {
		return "", errors.Annotatef(err, "client-side validation failed")
	}

	args := params.InitiateMigrationArgs{
		Specs: []params.MigrationSpec{{
			ModelTag: names.NewModelTag(spec.ModelUUID).String(),
			TargetInfo: params.MigrationTargetInfo{
				ControllerTag: names.NewControllerTag(spec.TargetControllerUUID).String(),
				Addrs:         spec.TargetAddrs,
				CACert:        spec.TargetCACert,
				AuthTag:       names.NewUserTag(spec.TargetUser).String(),
				Password:      spec.TargetPassword,
				Macaroons:     string(macsJSON),
			},
		}},
	}
	response := params.InitiateMigrationResults{}
	if err := c.facade.FacadeCall("InitiateMigration", args, &response); err != nil {
		return "", errors.Trace(err)
	}
	if len(response.Results) != 1 {
		return "", errors.New("unexpected number of results returned")
	}
	result := response.Results[0]
	if result.Error != nil {
		return "", errors.Trace(result.Error)
	}
	return result.MigrationId, nil
}

func macaroonsToJSON(macs []macaroon.Slice) (string, error) {
	if len(macs) == 0 {
		return "", nil
	}
	out, err := json.Marshal(macs)
	if err != nil {
		return "", errors.Annotate(err, "marshalling macaroons")
	}
	return string(out), nil
}
