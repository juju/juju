// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client is a caas model operator facade client
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS Operator Provisioner API.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CAASModelOperator")
	return &Client{
		facade: facadeCaller,
	}
}

// ModelOperatorProvisioningInfo represents return api information for
// provisioning a caas model operator
type ModelOperatorProvisioningInfo struct {
	APIAddresses []string
	ImagePath    string
	Version      version.Number
}

// ModelOperatorProvisioningInfo returns the information needed for a given model
// when provisioning into a caas env
func (c *Client) ModelOperatorProvisioningInfo() (ModelOperatorProvisioningInfo, error) {
	var result params.ModelOperatorInfo
	if err := c.facade.FacadeCall("ModelOperatorProvisioningInfo", nil, &result); err != nil {
		return ModelOperatorProvisioningInfo{}, err
	}

	return ModelOperatorProvisioningInfo{
		APIAddresses: result.APIAddresses,
		ImagePath:    result.ImagePath,
		Version:      result.Version,
	}, nil
}

// SetPasswords sets the supplied passwords on their corresponding models
func (c *Client) SetPassword(password string) error {
	var result params.ErrorResults
	modelTag, modelCon := c.facade.RawAPICaller().ModelTag()
	if !modelCon {
		return errors.New("not a model connection")
	}

	args := params.EntityPasswords{
		Changes: []params.EntityPassword{{
			Tag:      modelTag.String(),
			Password: password,
		}},
	}
	err := c.facade.FacadeCall("SetPasswords", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}
