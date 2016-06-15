// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud defines an API end point for functions dealing with
// the controller's cloud definition, and cloud credentials.
package cloud

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.cloud")

func init() {
	common.RegisterStandardFacade("Cloud", 1, newFacade)
}

// CloudAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type CloudAPI struct {
	backend    Backend
	authorizer common.Authorizer
	apiUser    names.UserTag
}

func newFacade(st *state.State, resources *common.Resources, auth common.Authorizer) (*CloudAPI, error) {
	return NewCloudAPI(NewStateBackend(st), auth)
}

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(backend Backend, authorizer common.Authorizer) (*CloudAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	return &CloudAPI{
		backend:    backend,
		authorizer: authorizer,
	}, nil
}

// Cloud returns the controller's cloud definition.
func (mm *CloudAPI) Cloud() (params.Cloud, error) {
	cloud, err := mm.backend.Cloud()
	if err != nil {
		return params.Cloud{}, err
	}
	authTypes := make([]string, len(cloud.AuthTypes))
	for i, authType := range cloud.AuthTypes {
		authTypes[i] = string(authType)
	}
	regions := make([]params.CloudRegion, len(cloud.Regions))
	for i, region := range cloud.Regions {
		regions[i] = params.CloudRegion{
			Name:            region.Name,
			Endpoint:        region.Endpoint,
			StorageEndpoint: region.StorageEndpoint,
		}
	}
	return params.Cloud{
		Type:            cloud.Type,
		AuthTypes:       authTypes,
		Endpoint:        cloud.Endpoint,
		StorageEndpoint: cloud.StorageEndpoint,
		Regions:         regions,
	}, nil
}

func (mm *CloudAPI) Credentials(args params.Entities) (params.CloudCredentialsResults, error) {
	results := params.CloudCredentialsResults{
		Results: make([]params.CloudCredentialsResult, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		// TODO(axw) auth on userTag
		cloudCredentials, err := mm.backend.CloudCredentials(userTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		out := make(map[string]params.CloudCredential)
		for name, credential := range cloudCredentials {
			out[name] = params.CloudCredential{
				string(credential.AuthType()),
				credential.Attributes(),
			}
		}
		results.Results[i].Credentials = out
	}
	return results, nil
}

func (mm *CloudAPI) UpdateCredentials(args params.UsersCloudCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Users)),
	}
	for i, arg := range args.Users {
		userTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		// TODO(axw) auth on userTag
		in := make(map[string]cloud.Credential)
		for name, credential := range arg.Credentials {
			in[name] = cloud.NewCredential(
				cloud.AuthType(credential.AuthType), credential.Attributes,
			)
		}
		if err := mm.backend.UpdateCloudCredentials(userTag, in); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}
