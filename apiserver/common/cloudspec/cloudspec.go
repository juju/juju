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

// NewCloudSpecForModel returns a new CloudSpecAPI that permits access to only
// one model.
func NewCloudSpecForModel(
	modelTag names.ModelTag,
	getCloudSpec func() (environs.CloudSpec, error),
) CloudSpecAPI {
	return CloudSpecAPI{
		func(names.ModelTag) (environs.CloudSpec, error) {
			// The tag passed in is guaranteed to be the
			// same as "modelTag", as the authorizer below
			// would have failed otherwise.
			return getCloudSpec()
		},
		func() (common.AuthFunc, error) {
			return func(tag names.Tag) bool {
				return tag == modelTag
			}, nil
		},
	}
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
		spec, err := s.getCloudSpec(tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		var paramsCloudCredential *params.CloudCredential
		if spec.Credential != nil && spec.Credential.AuthType() != "" {
			paramsCloudCredential = &params.CloudCredential{
				AuthType:   string(spec.Credential.AuthType()),
				Attributes: spec.Credential.Attributes(),
			}
		}
		results.Results[i].Result = &params.CloudSpec{
			spec.Type,
			spec.Name,
			spec.Region,
			spec.Endpoint,
			spec.IdentityEndpoint,
			spec.StorageEndpoint,
			paramsCloudCredential,
		}
	}
	return results, nil
}
