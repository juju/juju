// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cloud"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/rpc/params"
)

// CloudSpecAPI provides common client-side API functions
// to call into apiserver/common/environscloudspec.CloudSpec.
type CloudSpecAPI struct {
	facade   base.FacadeCaller
	modelTag names.ModelTag
}

// NewCloudSpecAPI creates a CloudSpecAPI using the provided
// FacadeCaller.
func NewCloudSpecAPI(facade base.FacadeCaller, modelTag names.ModelTag) *CloudSpecAPI {
	return &CloudSpecAPI{facade, modelTag}
}

// CloudSpec returns the cloud specification for the model associated
// with the API facade.
func (api *CloudSpecAPI) CloudSpec(ctx context.Context) (environscloudspec.CloudSpec, error) {
	var results params.CloudSpecResults
	args := params.Entities{Entities: []params.Entity{{Tag: api.modelTag.String()}}}
	err := api.facade.FacadeCall(ctx, "CloudSpec", args, &results)
	if err != nil {
		return environscloudspec.CloudSpec{}, err
	}
	if n := len(results.Results); n != 1 {
		return environscloudspec.CloudSpec{}, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return environscloudspec.CloudSpec{}, errors.Annotate(result.Error, "API request failed")
	}
	return api.MakeCloudSpec(result.Result)
}

// MakeCloudSpec creates an environscloudspec.CloudSpec from a params.CloudSpec
// that has been returned from the apiserver.
func (api *CloudSpecAPI) MakeCloudSpec(pSpec *params.CloudSpec) (environscloudspec.CloudSpec, error) {
	if pSpec == nil {
		return environscloudspec.CloudSpec{}, errors.NotValidf("nil value")
	}
	var credential *cloud.Credential
	if pSpec.Credential != nil {
		credentialValue := cloud.NewCredential(
			cloud.AuthType(pSpec.Credential.AuthType),
			pSpec.Credential.Attributes,
		)
		credential = &credentialValue
	}
	spec := environscloudspec.CloudSpec{
		Type:              pSpec.Type,
		Name:              pSpec.Name,
		Region:            pSpec.Region,
		Endpoint:          pSpec.Endpoint,
		IdentityEndpoint:  pSpec.IdentityEndpoint,
		StorageEndpoint:   pSpec.StorageEndpoint,
		CACertificates:    pSpec.CACertificates,
		SkipTLSVerify:     pSpec.SkipTLSVerify,
		Credential:        credential,
		IsControllerCloud: pSpec.IsControllerCloud,
	}
	if err := spec.Validate(); err != nil {
		return environscloudspec.CloudSpec{}, errors.Annotate(err, "validating CloudSpec")
	}
	return spec, nil
}
