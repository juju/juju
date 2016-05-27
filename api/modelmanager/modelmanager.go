// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/permission"
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

// ConfigSkeleton returns config values to be used as a starting point for the
// API caller to construct a valid model specific config.  The provider
// and region params are there for future use, and current behaviour expects
// both of these to be empty.
func (c *Client) ConfigSkeleton(provider, region string) (params.ModelConfig, error) {
	var result params.ModelConfigResult
	args := params.ModelSkeletonConfigArgs{
		Provider: provider,
		Region:   region,
	}
	err := c.facade.FacadeCall("ConfigSkeleton", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.Config, nil
}

// CreateModel creates a new model using the account and
// model config specified in the args.
func (c *Client) CreateModel(owner string, account, config map[string]interface{}) (params.Model, error) {
	var result params.Model
	if !names.IsValidUser(owner) {
		return result, errors.Errorf("invalid owner name %q", owner)
	}
	createArgs := params.ModelCreateArgs{
		OwnerTag: names.NewUserTag(owner).String(),
		Account:  account,
		Config:   config,
	}
	err := c.facade.FacadeCall("CreateModel", createArgs, &result)
	if err != nil {
		return result, errors.Trace(err)
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

// ParseModelAccess parses an access permission argument into
// a type suitable for making an API facade call.
func ParseModelAccess(access string) (params.ModelAccessPermission, error) {
	var fail params.ModelAccessPermission

	modelAccess, err := permission.ParseModelAccess(access)
	if err != nil {
		return fail, errors.Trace(err)
	}
	var accessPermission params.ModelAccessPermission
	switch modelAccess {
	case permission.ModelReadAccess:
		accessPermission = params.ModelReadAccess
	case permission.ModelWriteAccess:
		accessPermission = params.ModelWriteAccess
	default:
		return fail, errors.Errorf("unsupported model access permission %v", modelAccess)
	}
	return accessPermission, nil
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

	accessPermission, err := ParseModelAccess(access)
	if err != nil {
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
			Access:   accessPermission,
			ModelTag: modelTag.String(),
		})
	}

	var result params.ErrorResults
	err = c.facade.FacadeCall("ModifyModelAccess", args, &result)
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
