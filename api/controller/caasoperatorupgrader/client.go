// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client allows access to the CAAS operator upgrader API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator Upgrader API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASOperatorUpgrader")
	return &Client{
		facade: facadeCaller,
	}
}

// Upgrade upgrades the operator for the specified agent tag to v.
func (c *Client) Upgrade(agentTag string, v version.Number) error {
	var result params.ErrorResult
	arg := params.KubernetesUpgradeArg{
		AgentTag: agentTag,
		Version:  v,
	}
	if err := c.facade.FacadeCall(context.TODO(), "UpgradeOperator", arg, &result); err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}
