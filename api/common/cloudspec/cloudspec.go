// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
)

// CloudSpecAPI provides common client-side API functions
// to call into apiserver/common/cloudspec.CloudSpec.
type CloudSpecAPI struct {
	facade   base.FacadeCaller
	modelTag names.ModelTag
}

// NewCloudSpecAPI creates a CloudSpecAPI using the provided
// FacadeCaller.
func NewCloudSpecAPI(facade base.FacadeCaller, modelTag names.ModelTag) *CloudSpecAPI {
	return &CloudSpecAPI{facade, modelTag}
}

// WatchCloudSpecChanges returns a NotifyWatcher waiting for the
// model's cloud to change.
func (api *CloudSpecAPI) WatchCloudSpecChanges() (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{api.modelTag.String()}}}
	err := api.facade.FacadeCall("WatchCloudSpecsChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Annotate(result.Error, "API request failed")
	}
	return apiwatcher.NewNotifyWatcher(api.facade.RawAPICaller(), result), nil
}

// CloudSpec returns the cloud specification for the model associated
// with the API facade.
func (api *CloudSpecAPI) CloudSpec() (environs.CloudSpec, error) {
	var results params.CloudSpecResults
	args := params.Entities{Entities: []params.Entity{{api.modelTag.String()}}}
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
		CACertificates:   pSpec.CACertificates,
		Credential:       credential,
	}
	if err := spec.Validate(); err != nil {
		return environs.CloudSpec{}, errors.Annotate(err, "validating CloudSpec")
	}
	return spec, nil
}
