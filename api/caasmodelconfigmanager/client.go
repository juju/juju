// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodelconfigmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/controller"
)

// Client allows access to the CAAS model config manager API endpoint.
type Client struct {
	facade              base.FacadeCaller
	controllerConfigAPI *common.ControllerConfigAPI
}

// NewClient returns a client used to access the CAAS Application Provisioner API.
func NewClient(caller base.APICaller) (*Client, error) {
	_, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "CAASModelConfigManager")
	return &Client{
		facade:              facadeCaller,
		controllerConfigAPI: common.NewControllerConfig(facadeCaller),
	}, nil
}

func (c *Client) ControllerConfig() (controller.Config, error) {
	return c.controllerConfigAPI.ControllerConfig()
}
