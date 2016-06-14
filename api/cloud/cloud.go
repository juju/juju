// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
)

// Client provides methods that the Juju client command uses to interact
// with models stored in the Juju Server.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Cloud")
	return &Client{ClientFacade: frontend, facade: backend}
}

func (c *Client) Cloud() (cloud.Cloud, error) {
	var result params.Cloud
	if err := c.facade.FacadeCall("Cloud", nil, &result); err != nil {
		return cloud.Cloud{}, errors.Trace(err)
	}
	authTypes := make([]cloud.AuthType, len(result.AuthTypes))
	for i, authType := range result.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regions := make([]cloud.Region, len(result.Regions))
	for i, region := range result.Regions {
		regions[i] = cloud.Region{
			Name:            region.Name,
			Endpoint:        region.Endpoint,
			StorageEndpoint: region.StorageEndpoint,
		}
	}
	return cloud.Cloud{
		Type:            result.Type,
		AuthTypes:       authTypes,
		Endpoint:        result.Endpoint,
		StorageEndpoint: result.StorageEndpoint,
		Regions:         regions,
	}, nil
}

func (c *Client) Credentials(user names.UserTag) (map[string]cloud.Credential, error) {
	var results params.CloudCredentialsResults
	args := params.Entities{[]params.Entity{{user.String()}}}
	if err := c.facade.FacadeCall("Credentials", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return nil, results.Results[0].Error
	}
	credentials := make(map[string]cloud.Credential)
	for name, credential := range results.Results[0].Credentials {
		credentials[name] = cloud.NewCredential(
			cloud.AuthType(credential.AuthType),
			credential.Attributes,
		)
	}
	return credentials, nil
}

func (c *Client) UpdateCredentials(user names.UserTag, credentials map[string]cloud.Credential) error {
	var results params.ErrorResults
	paramsCredentials := make(map[string]params.CloudCredential)
	for name, credential := range credentials {
		paramsCredentials[name] = params.CloudCredential{
			AuthType:   string(credential.AuthType()),
			Attributes: credential.Attributes(),
		}
	}
	args := params.UsersCloudCredentials{[]params.UserCloudCredentials{{
		UserTag:     user.String(),
		Credentials: paramsCredentials,
	}}}
	if err := c.facade.FacadeCall("UpdateCredentials", args, &results); err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return results.Results[0].Error
	}
	return nil
}
