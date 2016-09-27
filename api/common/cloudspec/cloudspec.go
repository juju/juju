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
	return api.MakeCloudSpec(result.Result)
}

// MakeCloudSpec creates an environs.CloudSpec from a params.CloudSpec
// that has been returned from the apiserver.
func (api *CloudSpecAPI) MakeCloudSpec(pSpec *params.CloudSpec) (environs.CloudSpec, error) {
	if pSpec == nil {
		return environs.CloudSpec{}, errors.NotValidf("nil value")
	}
	var credential *cloud.Credential
	if pSpec.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(pSpec.Credential.AuthType),
			pSpec.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := environs.CloudSpec{
		Type:             pSpec.Type,
		Name:             pSpec.Name,
		Region:           pSpec.Region,
		Endpoint:         pSpec.Endpoint,
		IdentityEndpoint: pSpec.IdentityEndpoint,
		StorageEndpoint:  pSpec.StorageEndpoint,
		Credential:       credential,
	}
	if err := spec.Validate(); err != nil {
		return environs.CloudSpec{}, errors.Annotate(err, "validating CloudSpec")
	}
	return spec, nil
}
