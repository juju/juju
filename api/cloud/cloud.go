// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
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

// Cloud returns the details of the cloud with the given tag.
func (c *Client) Cloud(tag names.CloudTag) (jujucloud.Cloud, error) {
	var results params.CloudResults
	args := params.Entities{[]params.Entity{{tag.String()}}}
	if err := c.facade.FacadeCall("Cloud", args, &results); err != nil {
		return jujucloud.Cloud{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return jujucloud.Cloud{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return jujucloud.Cloud{}, results.Results[0].Error
	}
	result := results.Results[0].Cloud
	authTypes := make([]jujucloud.AuthType, len(result.AuthTypes))
	for i, authType := range result.AuthTypes {
		authTypes[i] = jujucloud.AuthType(authType)
	}
	regions := make([]jujucloud.Region, len(result.Regions))
	for i, region := range result.Regions {
		regions[i] = jujucloud.Region{
			Name:            region.Name,
			Endpoint:        region.Endpoint,
			StorageEndpoint: region.StorageEndpoint,
		}
	}
	return jujucloud.Cloud{
		Type:            result.Type,
		AuthTypes:       authTypes,
		Endpoint:        result.Endpoint,
		StorageEndpoint: result.StorageEndpoint,
		Regions:         regions,
	}, nil
}

// CloudDefaults returns the cloud defaults for the given users.
func (c *Client) CloudDefaults(user names.UserTag) (jujucloud.Defaults, error) {
	var results params.CloudDefaultsResults
	args := params.Entities{[]params.Entity{{user.String()}}}
	if err := c.facade.FacadeCall("CloudDefaults", args, &results); err != nil {
		return jujucloud.Defaults{}, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return jujucloud.Defaults{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return jujucloud.Defaults{}, results.Results[0].Error
	}
	result := results.Results[0].Result
	cloudTag, err := names.ParseCloudTag(result.CloudTag)
	if err != nil {
		return jujucloud.Defaults{}, errors.Trace(err)
	}
	return jujucloud.Defaults{
		Cloud:      cloudTag.Id(),
		Region:     result.CloudRegion,
		Credential: result.CloudCredential,
	}, nil
}

// Credentials returns the cloud credentials for the user and cloud with
// the given tags.
func (c *Client) Credentials(user names.UserTag, cloud names.CloudTag) (map[string]jujucloud.Credential, error) {
	var results params.CloudCredentialsResults
	args := params.UserClouds{[]params.UserCloud{
		{UserTag: user.String(), CloudTag: cloud.String()},
	}}
	if err := c.facade.FacadeCall("Credentials", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return nil, results.Results[0].Error
	}
	credentials := make(map[string]jujucloud.Credential)
	for name, credential := range results.Results[0].Credentials {
		credentials[name] = jujucloud.NewCredential(
			jujucloud.AuthType(credential.AuthType),
			credential.Attributes,
		)
	}
	return credentials, nil
}

// UpdateCredentials updates the cloud credentials for the user and cloud with
// the given tags. Exiting credentials that are not named in the map will be
// untouched.
func (c *Client) UpdateCredentials(user names.UserTag, cloud names.CloudTag, credentials map[string]jujucloud.Credential) error {
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
		CloudTag:    cloud.String(),
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
