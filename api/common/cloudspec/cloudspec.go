// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"github.com/juju/errors"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

// CloudSpecAPI provides common client-side API functions
// to call into apiserver/common/cloudspec.CloudSpec.
type CloudSpecAPI struct {
	facade base.FacadeCaller
}

// NewCloudSpecAPI creates a CloudSpecAPI using the provided
// FacadeCaller.
func NewCloudSpecAPI(facade base.FacadeCaller) *CloudSpecAPI {
	return &CloudSpecAPI{facade}
}

// CloudSpec returns the cloud specification for the model
// with the given tag.
func (api *CloudSpecAPI) CloudSpec(tag names.ModelTag) (environs.CloudSpec, error) {
	var results params.CloudSpecResults
	args := params.Entities{Entities: []params.Entity{{tag.String()}}}
	err := api.facade.FacadeCall("CloudSpec", args, &results)
	if err != nil {
		return environs.CloudSpec{}, err
	}
	if n := len(results.Results); n != 1 {
		return environs.CloudSpec{}, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return environs.CloudSpec{}, errors.Annotate(result.Error, "API request failed")
	}
	var credential *cloud.Credential
	if result.Result.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(result.Result.Credential.AuthType),
			result.Result.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := environs.CloudSpec{
		Type:             result.Result.Type,
		Name:             result.Result.Name,
		Region:           result.Result.Region,
		Endpoint:         result.Result.Endpoint,
		IdentityEndpoint: result.Result.IdentityEndpoint,
		StorageEndpoint:  result.Result.StorageEndpoint,
		Credential:       credential,
	}
	if err := spec.Validate(); err != nil {
		return environs.CloudSpec{}, errors.Annotate(err, "validating CloudSpec")
	}
	return spec, nil
}
