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
// environments in the controller.
func (c *Client) AllModels() ([]base.UserModel, error) {
	var environments params.UserModelList
	err := c.facade.FacadeCall("AllModels", nil, &environments)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]base.UserModel, len(environments.UserModels))
	for i, env := range environments.UserModels {
		owner, err := names.ParseUserTag(env.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", env.OwnerTag, i)
		}
		result[i] = base.UserModel{
			Name:           env.Name,
			UUID:           env.UUID,
			Owner:          owner.Canonical(),
			LastConnection: env.LastConnection,
		}
	}
	return result, nil
}

// EnvironmentConfig returns all environment settings for the
// controller environment.
func (c *Client) EnvironmentConfig() (map[string]interface{}, error) {
	result := params.EnvironmentConfigResults{}
	err := c.facade.FacadeCall("EnvironmentConfig", nil, &result)
	return result.Config, err
}

// DestroyController puts the controller environment into a "dying" state,
// and removes all non-manager machine instances. Underlying DestroyEnvironment
// calls will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroyController(destroyEnvs bool) error {
	args := params.DestroyControllerArgs{
		DestroyModels: destroyEnvs,
	}
	return c.facade.FacadeCall("DestroyController", args, nil)
}

// ListBlockedModels returns a list of all environments within the controller
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

// WatchAllEnvs returns an AllEnvWatcher, from which you can request
// the Next collection of Deltas (for all environments).
func (c *Client) WatchAllEnvs() (*api.AllWatcher, error) {
	info := new(api.WatchAll)
	if err := c.facade.FacadeCall("WatchAllEnvs", nil, info); err != nil {
		return nil, err
	}
	return api.NewAllEnvWatcher(c.facade.RawAPICaller(), &info.AllWatcherId), nil
}

// ModelStatus returns a status summary for each environment tag passed in.
func (c *Client) ModelStatus(tags ...names.EnvironTag) ([]base.ModelStatus, error) {
	result := params.ModelStatusResults{}
	envs := make([]params.Entity, len(tags))
	for i, tag := range tags {
		envs[i] = params.Entity{Tag: tag.String()}
	}
	req := params.Entities{
		Entities: envs,
	}
	if err := c.facade.FacadeCall("ModelStatus", req, &result); err != nil {
		return nil, err
	}

	results := make([]base.ModelStatus, len(result.Results))
	for i, r := range result.Results {
		env, err := names.ParseEnvironTag(r.ModelTag)
		if err != nil {
			return nil, errors.Annotatef(err, "EnvironTag %q at position %d", r.ModelTag, i)
		}
		owner, err := names.ParseUserTag(r.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", r.OwnerTag, i)
		}

		results[i] = base.ModelStatus{
			UUID:               env.Id(),
			Life:               r.Life,
			Owner:              owner.Canonical(),
			HostedMachineCount: r.HostedMachineCount,
			ServiceCount:       r.ServiceCount,
		}

	}
	return results, nil
}
