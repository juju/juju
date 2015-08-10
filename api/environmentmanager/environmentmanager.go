// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environmentmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.environmentmanager")

// Client provides methods that the Juju client command uses to interact
// with environments stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "EnvironmentManager")
	logger.Debugf("%#v", frontend)
	return &Client{ClientFacade: frontend, facade: backend}
}

// ConfigSkeleton returns config values to be used as a starting point for the
// API caller to construct a valid environment specific config.  The provider
// and region params are there for future use, and current behaviour expects
// both of these to be empty.
func (c *Client) ConfigSkeleton(provider, region string) (params.EnvironConfig, error) {
	var result params.EnvironConfigResult
	args := params.EnvironmentSkeletonConfigArgs{
		Provider: provider,
		Region:   region,
	}
	err := c.facade.FacadeCall("ConfigSkeleton", args, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.Config, nil
}

// CreateEnvironment creates a new environment using the account and
// environment config specified in the args.
func (c *Client) CreateEnvironment(owner string, account, config map[string]interface{}) (params.Environment, error) {
	var result params.Environment
	if !names.IsValidUser(owner) {
		return result, errors.Errorf("invalid owner name %q", owner)
	}
	createArgs := params.EnvironmentCreateArgs{
		OwnerTag: names.NewUserTag(owner).String(),
		Account:  account,
		Config:   config,
	}
	err := c.facade.FacadeCall("CreateEnvironment", createArgs, &result)
	if err != nil {
		return result, errors.Trace(err)
	}
	logger.Infof("created environment %s (%s)", result.Name, result.UUID)
	return result, nil
}

// ListEnvironments returns the environments that the specified user
// has access to in the current server.  Only that state server owner
// can list environments for any user (at this stage).  Other users
// can only ask about their own environments.
func (c *Client) ListEnvironments(user string) ([]base.UserEnvironment, error) {
	var environments params.UserEnvironmentList
	if !names.IsValidUser(user) {
		return nil, errors.Errorf("invalid user name %q", user)
	}
	entity := params.Entity{names.NewUserTag(user).String()}
	err := c.facade.FacadeCall("ListEnvironments", entity, &environments)
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
