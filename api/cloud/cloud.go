// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
)

var logger = loggo.GetLogger("juju.api.cloud")

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
	if err := results.Results[0].Error; err != nil {
		if params.IsCodeNotFound(err) {
			return jujucloud.Cloud{}, errors.NotFoundf("cloud %s", tag.Id())
		}
		return jujucloud.Cloud{}, err
	}
	return common.CloudFromParams(tag.Id(), *results.Results[0].Cloud), nil
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

// UpdateCloudsCredentials updates clouds credentials content on the controller.
// Passed in credentials are keyed on the credential tag.
// This operation can be forced to ignore validation checks.
func (c *Client) UpdateCloudsCredentials(cloudCredentials map[string]jujucloud.Credential, force bool) ([]params.UpdateCredentialResult, error) {
	return c.internalUpdateCloudsCredentials(params.UpdateCredentialArgs{Force: force}, cloudCredentials)
}

// AddCloudsCredentials adds/uploads clouds credentials content to the controller.
// Passed in credentials are keyed on the credential tag.
func (c *Client) AddCloudsCredentials(cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
	return c.internalUpdateCloudsCredentials(params.UpdateCredentialArgs{}, cloudCredentials)
}

func (c *Client) internalUpdateCloudsCredentials(in params.UpdateCredentialArgs, cloudCredentials map[string]jujucloud.Credential) ([]params.UpdateCredentialResult, error) {
	for tag, credential := range cloudCredentials {
		in.Credentials = append(in.Credentials, params.TaggedCredential{
			Tag: tag,
			Credential: params.CloudCredential{
				AuthType:   string(credential.AuthType()),
				Attributes: credential.Attributes(),
			},
		})
	}
	count := len(cloudCredentials)

	countErr := func(got int) error {
		plural := "s"
		if count == 1 {
			plural = ""
		}
		return errors.Errorf("expected %d result%v got %d when updating credentials", count, plural, got)
	}
	if c.facade.BestAPIVersion() < 3 {
		var out params.ErrorResults
		if err := c.facade.FacadeCall("UpdateCredentials", in, &out); err != nil {
			return nil, errors.Trace(err)
		}
		if len(out.Results) != count {
			return nil, countErr(len(out.Results))
		}
		converted := make([]params.UpdateCredentialResult, count)
		for i, one := range out.Results {
			converted[i] = params.UpdateCredentialResult{CredentialTag: in.Credentials[i].Tag, Error: one.Error}
		}
		return converted, nil
	}

	var out params.UpdateCredentialResults
	if err := c.facade.FacadeCall("UpdateCredentialsCheckModels", in, &out); err != nil {
		return nil, errors.Trace(err)
	}
	if len(out.Results) != count {
		return nil, countErr(len(out.Results))
	}
	// Older facades incorrectly set an error if models are invalid.
	// The model result structs themselves contain the errors.
	for i, r := range out.Results {
		if r.Error == nil {
			continue
		}
		if r.Error.Message == "some models are no longer visible" {
			r.Error = nil
		}
		out.Results[i] = r
	}
	return out.Results, nil
}

// UpdateCredentialsCheckModels updates a cloud credential content
// stored on the controller. This call validates that the new content works
// for all models that are using this credential.
func (c *Client) UpdateCredentialsCheckModels(tag names.CloudCredentialTag, credential jujucloud.Credential) ([]params.UpdateCredentialModelResult, error) {
	out, err := c.UpdateCloudsCredentials(map[string]jujucloud.Credential{tag.String(): credential}, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if out[0].Error != nil {
		// Unlike many other places, we want to return something valid here to provide more details.
		return out[0].Models, errors.Trace(out[0].Error)
	}
	return out[0].Models, nil
}

// RevokeCredential revokes/deletes a cloud credential.
func (c *Client) RevokeCredential(tag names.CloudCredentialTag, force bool) error {
	var results params.ErrorResults

	if c.facade.BestAPIVersion() < 3 {
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

	args := params.RevokeCredentialArgs{
		Credentials: []params.RevokeCredentialArg{
			{Tag: tag.String(), Force: force},
		},
	}
	if err := c.facade.FacadeCall("RevokeCredentialsCheckModels", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// Credentials returns a slice of credential values for the specified tags.
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

// AddCloud adds a new cloud to current controller.
func (c *Client) AddCloud(cloud jujucloud.Cloud, force bool) error {
	bestVer := c.BestAPIVersion()
	if bestVer < 2 {
		return errors.NotImplementedf("AddCloud() (need v2+, have v%d)", bestVer)
	}
	if bestVer < 6 && force {
		return errors.NotImplementedf("AddCloud() with force (need v6+, have v%d)", bestVer)
	}
	if (len(cloud.Config) > 0 || len(cloud.RegionConfig) > 0) && bestVer < 5 {
		return errors.New("adding a cloud with config parameters is not supported by this version of Juju")
	}
	args := params.AddCloudArgs{Name: cloud.Name, Cloud: common.CloudToParams(cloud)}
	if force {
		args.Force = &force
	}
	err := c.facade.FacadeCall("AddCloud", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// UpdateCloud updates an existing cloud on a current controller.
func (c *Client) UpdateCloud(cloud jujucloud.Cloud) error {
	if c.BestAPIVersion() < 4 {
		return errors.New("updating controller cloud is not supported by this version of Juju")
	}
	args := params.UpdateCloudArgs{
		Clouds: []params.AddCloudArgs{{
			Name:  cloud.Name,
			Cloud: common.CloudToParams(cloud),
		}},
	}
	var results params.ErrorResults
	if err := c.facade.FacadeCall("UpdateCloud", args, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// RemoveCloud removes a cloud from the current controller.
func (c *Client) RemoveCloud(cloud string) error {
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return errors.NotImplementedf("RemoveCloud() (need v2+, have v%d)", bestVer)
	}
	args := params.Entities{Entities: []params.Entity{{Tag: names.NewCloudTag(cloud).String()}}}
	var result params.ErrorResults
	err := c.facade.FacadeCall("RemoveClouds", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// CredentialContents returns contents of the credential values for the specified
// cloud and credential name. Secrets will be included if requested.
func (c *Client) CredentialContents(cloud, credential string, withSecrets bool) ([]params.CredentialContentResult, error) {
	if bestVer := c.BestAPIVersion(); bestVer < 2 {
		return nil, errors.NotImplementedf("CredentialContents() (need v2+, have v%d)", bestVer)
	}
	oneCredential := params.CloudCredentialArg{}
	if cloud == "" && credential == "" {
		// this is valid and means we want all.
	} else if cloud == "" {
		return nil, errors.New("cloud name must be supplied")
	} else if credential == "" {
		return nil, errors.New("credential name must be supplied")
	} else {
		oneCredential.CloudName = cloud
		oneCredential.CredentialName = credential
	}
	var out params.CredentialContentResults
	in := params.CloudCredentialArgs{
		IncludeSecrets: withSecrets,
	}
	if !oneCredential.IsEmpty() {
		in.Credentials = []params.CloudCredentialArg{oneCredential}
	}
	err := c.facade.FacadeCall("CredentialContents", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !oneCredential.IsEmpty() && len(out.Results) != 1 {
		return nil, errors.Errorf("expected 1 result for credential %q on cloud %q, got %d", cloud, credential, len(out.Results))
	}
	return out.Results, nil
}

// GrantCloud grants a user access to a cloud.
func (c *Client) GrantCloud(user, access string, clouds ...string) error {
	if bestVer := c.BestAPIVersion(); bestVer < 3 {
		return errors.NotImplementedf("GrantCloud() (need v3+, have v%d)", bestVer)
	}
	return c.modifyCloudUser(params.GrantCloudAccess, user, access, clouds)
}

// RevokeCloud revokes a user's access to a cloud.
func (c *Client) RevokeCloud(user, access string, clouds ...string) error {
	if bestVer := c.BestAPIVersion(); bestVer < 3 {
		return errors.NotImplementedf("RevokeCloud() (need v3+, have v%d)", bestVer)
	}
	return c.modifyCloudUser(params.RevokeCloudAccess, user, access, clouds)
}

func (c *Client) modifyCloudUser(action params.CloudAction, user, access string, clouds []string) error {
	var args params.ModifyCloudAccessRequest

	if !names.IsValidUser(user) {
		return errors.Errorf("invalid username: %q", user)
	}
	userTag := names.NewUserTag(user)

	cloudAccess := permission.Access(access)
	if err := permission.ValidateCloudAccess(cloudAccess); err != nil {
		return errors.Trace(err)
	}
	for _, cloud := range clouds {
		if !names.IsValidCloud(cloud) {
			return errors.NotValidf("cloud %q", cloud)
		}
		cloudTag := names.NewCloudTag(cloud)
		args.Changes = append(args.Changes, params.ModifyCloudAccess{
			UserTag:  userTag.String(),
			Action:   action,
			Access:   access,
			CloudTag: cloudTag.String(),
		})
	}

	var result params.ErrorResults
	err := c.facade.FacadeCall("ModifyCloudAccess", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Changes) {
		return errors.Errorf("expected %d results, got %d", len(args.Changes), len(result.Results))
	}

	for i, r := range result.Results {
		if r.Error != nil && r.Error.Code == params.CodeAlreadyExists {
			logger.Warningf("cloud %q is already shared with %q", clouds[i], userTag.Id())
			result.Results[i].Error = nil
		}
	}
	return result.Combine()
}
