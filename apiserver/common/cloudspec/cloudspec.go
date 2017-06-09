// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudspec

import (
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
)

// CloudSpecAPI implements common methods for use by various
// facades for querying the cloud spec of models.
type CloudSpecAPI struct {
	getCloudSpec func(names.ModelTag) (environs.CloudSpec, error)
	getAuthFunc  common.GetAuthFunc
}

// NewCloudSpec returns a new CloudSpecAPI.
func NewCloudSpec(
	getCloudSpec func(names.ModelTag) (environs.CloudSpec, error),
	getAuthFunc common.GetAuthFunc,
) CloudSpecAPI {
	return CloudSpecAPI{getCloudSpec, getAuthFunc}
}

// CloudSpec returns the model's cloud spec.
func (s CloudSpecAPI) CloudSpec(args params.Entities) (params.CloudSpecResults, error) {
	authFunc, err := s.getAuthFunc()
	if err != nil {
		return params.CloudSpecResults{}, err
	}
	results := params.CloudSpecResults{
		Results: make([]params.CloudSpecResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(tag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		results.Results[i] = s.GetCloudSpec(tag)
	}
	return results, nil
}

// GetCloudSpec constucts the CloudSpec for a validated and authorized model.
func (s CloudSpecAPI) GetCloudSpec(tag names.ModelTag) params.CloudSpecResult {
	var result params.CloudSpecResult
	spec, err := s.getCloudSpec(tag)
	if err != nil {
		result.Error = common.ServerError(err)
		return result
	}
	var paramsCloudCredential *params.CloudCredential
	if spec.Credential != nil && spec.Credential.AuthType() != "" {
		paramsCloudCredential = &params.CloudCredential{
			AuthType:   string(spec.Credential.AuthType()),
			Attributes: spec.Credential.Attributes(),
		}
	}
	result.Result = &params.CloudSpec{
		spec.Type,
		spec.Name,
		spec.Region,
		spec.Endpoint,
		spec.IdentityEndpoint,
		spec.StorageEndpoint,
		paramsCloudCredential,
	}
	return result
}
