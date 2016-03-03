// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
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
	logger.Debugf("%#v", frontend)
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
	logger.Infof("created model %s (%s)", result.Name, result.UUID)
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
