// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package systemmanager

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

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
	logger.Debugf("%#v", frontend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// EnvironmentGet returns all environment settings for the
// system environment.
func (c *Client) EnvironmentGet() (map[string]interface{}, error) {
	result := params.EnvironmentConfigResults{}
	err := c.facade.FacadeCall("EnvironmentGet", nil, &result)
	return result.Config, err
}

// DestroySystem puts the system environment into a "dying" state,
// and removes all non-manager machine instances. Underlying DestroyEnvironment
// calls will fail if there are any manually-provisioned non-manager machines
// in state.
func (c *Client) DestroySystem(envUUID names.EnvironTag, destroyEnvs bool, ignoreBlocks bool) error {
	args := params.DestroySystemArgs{
		EnvTag:       envUUID.String(),
		DestroyEnvs:  destroyEnvs,
		IgnoreBlocks: ignoreBlocks,
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
