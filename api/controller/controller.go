// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.controller")

// Client provides methods that the Juju client command uses to interact
// with the Juju controller.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Controller")
	logger.Tracef("%#v", frontend)
	return &Client{ClientFacade: frontend, facade: backend}
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

// ModelConfig returns all model settings for the
// controller model.
func (c *Client) ModelConfig() (map[string]interface{}, error) {
	result := params.ModelConfigResults{}
	err := c.facade.FacadeCall("ModelConfig", nil, &result)
	return result.Config, err
}

// DestroyController puts the controller model into a "dying" state,
// and removes all non-manager machine instances. Underlying DestroyModel
// calls will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroyController(destroyModels bool) error {
	args := params.DestroyControllerArgs{
		DestroyModels: destroyModels,
	}
	return c.facade.FacadeCall("DestroyController", args, nil)
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
	info := new(api.WatchAll)
	if err := c.facade.FacadeCall("WatchAllModels", nil, info); err != nil {
		return nil, err
	}
	return api.NewAllModelWatcher(c.facade.RawAPICaller(), &info.AllWatcherId), nil
}

// ModelStatus returns a status summary for each model tag passed in.
func (c *Client) ModelStatus(tags ...names.ModelTag) ([]base.ModelStatus, error) {
	result := params.ModelStatusResults{}
	models := make([]params.Entity, len(tags))
	for i, tag := range tags {
		models[i] = params.Entity{Tag: tag.String()}
	}
	req := params.Entities{
		Entities: models,
	}
	if err := c.facade.FacadeCall("ModelStatus", req, &result); err != nil {
		return nil, err
	}

	results := make([]base.ModelStatus, len(result.Results))
	for i, r := range result.Results {
		model, err := names.ParseModelTag(r.ModelTag)
		if err != nil {
			return nil, errors.Annotatef(err, "ModelTag %q at position %d", r.ModelTag, i)
		}
		owner, err := names.ParseUserTag(r.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", r.OwnerTag, i)
		}

		results[i] = base.ModelStatus{
			UUID:               model.Id(),
			Life:               r.Life,
			Owner:              owner.Canonical(),
			HostedMachineCount: r.HostedMachineCount,
			ServiceCount:       r.ServiceCount,
		}

	}
	return results, nil
}
