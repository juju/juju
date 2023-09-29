// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/rpc/params"
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
	ImageDetails resources.DockerImageDetails
	Version      version.Number
}

// ModelOperatorProvisioningInfo returns the information needed for a given model
// when provisioning into a caas env
func (c *Client) ModelOperatorProvisioningInfo() (ModelOperatorProvisioningInfo, error) {
	var result params.ModelOperatorInfo
	if err := c.facade.FacadeCall("ModelOperatorProvisioningInfo", nil, &result); err != nil {
		return ModelOperatorProvisioningInfo{}, err
	}
	d := result.ImageDetails
	imageRepo := resources.DockerImageDetails{
		RegistryPath: d.RegistryPath,
		ImageRepoDetails: docker.ImageRepoDetails{
			Repository:    d.Repository,
			ServerAddress: d.ServerAddress,
			BasicAuthConfig: docker.BasicAuthConfig{
				Username: d.Username,
				Password: d.Password,
				Auth:     docker.NewToken(d.Auth),
			},
			TokenAuthConfig: docker.TokenAuthConfig{
				IdentityToken: docker.NewToken(d.IdentityToken),
				RegistryToken: docker.NewToken(d.RegistryToken),
				Email:         d.Email,
			},
		},
	}

	return ModelOperatorProvisioningInfo{
		APIAddresses: result.APIAddresses,
		ImageDetails: imageRepo,
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

// WatchModelOperatorProvisioningInfo provides a watcher for changes that affect the
// information returned by ModelOperatorProvisioningInfo.
func (c *Client) WatchModelOperatorProvisioningInfo() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	if err := c.facade.FacadeCall("WatchModelOperatorProvisioningInfo", nil, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
