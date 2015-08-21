// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.systemmanager")

// Client provides methods that the Juju client command uses to interact
// with systems stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "SystemManager")
	logger.Tracef("%#v", frontend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// AllEnvironments allows system administrators to get the list of all the
// environments in the system.
func (c *Client) AllEnvironments() ([]base.UserEnvironment, error) {
	var environments params.UserEnvironmentList
	err := c.facade.FacadeCall("AllEnvironments", nil, &environments)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]base.UserEnvironment, len(environments.UserEnvironments))
	for i, env := range environments.UserEnvironments {
		owner, err := names.ParseUserTag(env.OwnerTag)
		if err != nil {
			return nil, errors.Annotatef(err, "OwnerTag %q at position %d", env.OwnerTag, i)
		}
		result[i] = base.UserEnvironment{
			Name:           env.Name,
			UUID:           env.UUID,
			Owner:          owner.Username(),
			LastConnection: env.LastConnection,
		}
	}
	return result, nil
}

// EnvironmentConfig returns all environment settings for the
// system environment.
func (c *Client) EnvironmentConfig() (map[string]interface{}, error) {
	result := params.EnvironmentConfigResults{}
	err := c.facade.FacadeCall("EnvironmentConfig", nil, &result)
	return result.Config, err
}

// DestroySystem puts the system environment into a "dying" state,
// and removes all non-manager machine instances. Underlying DestroyEnvironment
// calls will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroySystem(destroyEnvs bool, ignoreBlocks bool) error {
	args := params.DestroySystemArgs{
		DestroyEnvironments: destroyEnvs,
		IgnoreBlocks:        ignoreBlocks,
	}
	return c.facade.FacadeCall("DestroySystem", args, nil)
}

// ListBlockedEnvironments returns a list of all environments within the system
// which have at least one block in place.
func (c *Client) ListBlockedEnvironments() ([]params.EnvironmentBlockInfo, error) {
	result := params.EnvironmentBlockInfoList{}
	err := c.facade.FacadeCall("ListBlockedEnvironments", nil, &result)
	return result.Environments, err
}

// RemoveBlocks removes all the blocks in the system.
func (c *Client) RemoveBlocks() error {
	args := params.RemoveBlocksArgs{All: true}
	return c.facade.FacadeCall("RemoveBlocks", args, nil)
}

// WatchAllEnv returns an AllEnvWatcher, from which you can request
// the Next collection of Deltas (for all environments).
func (c *Client) WatchAllEnvs() (*api.AllWatcher, error) {
	info := new(api.WatchAll)
	if err := c.facade.FacadeCall("WatchAllEnvs", nil, info); err != nil {
		return nil, err
	}
	return api.NewAllEnvWatcher(c.facade.RawAPICaller(), &info.AllWatcherId), nil
}
