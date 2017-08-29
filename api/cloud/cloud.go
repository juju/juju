// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
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

// Clouds returns the details of all clouds supported by the controller.
func (c *Client) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	var result params.CloudsResult
	if err := c.facade.FacadeCall("Clouds", nil, &result); err != nil {
		return nil, errors.Trace(err)
	}
	clouds := make(map[names.CloudTag]jujucloud.Cloud)
	for tagString, cloud := range result.Clouds {
		tag, err := names.ParseCloudTag(tagString)
		if err != nil {
			return nil, errors.Trace(err)
		}
		clouds[tag] = common.CloudFromParams(tag.Id(), cloud)
	}
	return clouds, nil
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
	return common.CloudFromParams(tag.Id(), *results.Results[0].Cloud), nil
}

// DefaultCloud returns the tag of the cloud that models will be
// created in by default.
func (c *Client) DefaultCloud() (names.CloudTag, error) {
	var result params.StringResult
	if err := c.facade.FacadeCall("DefaultCloud", nil, &result); err != nil {
		return names.CloudTag{}, errors.Trace(err)
	}
	if result.Error != nil {
		return names.CloudTag{}, result.Error
	}
	cloudTag, err := names.ParseCloudTag(result.Result)
	if err != nil {
		return names.CloudTag{}, errors.Trace(err)
	}
	return cloudTag, nil
}

// UserCredentials returns the tags for cloud credentials available to a user for
// use with a specific cloud.
func (c *Client) UserCredentials(user names.UserTag, cloud names.CloudTag) ([]names.CloudCredentialTag, error) {
	var results params.StringsResults
	args := params.UserClouds{[]params.UserCloud{
		{UserTag: user.String(), CloudTag: cloud.String()},
	}}
	if err := c.facade.FacadeCall("UserCredentials", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	if results.Results[0].Error != nil {
		return nil, results.Results[0].Error
	}
	tags := make([]names.CloudCredentialTag, len(results.Results[0].Result))
	for i, s := range results.Results[0].Result {
		tag, err := names.ParseCloudCredentialTag(s)
		if err != nil {
			return nil, errors.Trace(err)
		}
		tags[i] = tag
	}
	return tags, nil
}

// UpdateCredential updates a cloud credentials.
func (c *Client) UpdateCredential(tag names.CloudCredentialTag, credential jujucloud.Credential) error {
	var results params.ErrorResults
	args := params.TaggedCredentials{
		Credentials: []params.TaggedCredential{{
			Tag: tag.String(),
			Credential: params.CloudCredential{
				AuthType:   string(credential.AuthType()),
				Attributes: credential.Attributes(),
			},
		}},
	}
	if err := c.facade.FacadeCall("UpdateCredentials", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// RemoveCredential removes a cloud credential if no models are using it.
func (c *Client) RemoveCredential(tag names.CloudCredentialTag) error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	}
	if err := c.facade.FacadeCall("RemoveCredentials", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// RevokeCredential unconditionally revokes/deletes a cloud credential.
func (c *Client) RevokeCredential(tag names.CloudCredentialTag) error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: tag.String(),
		}},
	}
	if err := c.facade.FacadeCall("RevokeCredentials", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// Credentials return a slice of credential values for the specified tags.
// Secrets are excluded from the credential attributes.
func (c *Client) Credentials(tags ...names.CloudCredentialTag) ([]params.CloudCredentialResult, error) {
	if len(tags) == 0 {
		return []params.CloudCredentialResult{}, nil
	}
	var results params.CloudCredentialResults
	args := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	if err := c.facade.FacadeCall("Credential", args, &results); err != nil {
		return nil, errors.Trace(err)
	}
	return results.Results, nil
}

// AddCredential adds a credential to the controller with a given tag.
// This can be a credential for a cloud that is not the same cloud as the controller's host.
func (c *Client) AddCredential(tag string, credential jujucloud.Credential) error {
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return errors.NotImplementedf("AddCredential() (need v2+, have v%d)", bestVer)
	}
	var results params.ErrorResults
	cloudCredential := params.CloudCredential{
		AuthType:   string(credential.AuthType()),
		Attributes: credential.Attributes(),
	}
	args := params.TaggedCredentials{
		Credentials: []params.TaggedCredential{{
			Tag:        tag,
			Credential: cloudCredential,
		},
		}}
	if err := c.facade.FacadeCall("AddCredentials", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

func (c *Client) AddCloud(cloud jujucloud.Cloud) error {
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return errors.NotImplementedf("AddCloud() (need v2+, have v%d)", bestVer)
	}
	args := params.AddCloudArgs{Name: cloud.Name, Cloud: common.CloudToParams(cloud)}
	err := c.facade.FacadeCall("AddCloud", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
