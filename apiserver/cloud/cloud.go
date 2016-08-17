// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud defines an API end point for functions dealing with
// the controller's cloud definition, and cloud credentials.
package cloud

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.cloud")

func init() {
	common.RegisterStandardFacade("Cloud", 1, newFacade)
}

// CloudAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type CloudAPI struct {
	backend                Backend
	authorizer             facade.Authorizer
	apiUser                names.UserTag
	getCredentialsAuthFunc common.GetAuthFunc
}

func newFacade(st *state.State, resources facade.Resources, auth facade.Authorizer) (*CloudAPI, error) {
	return NewCloudAPI(NewStateBackend(st), auth)
}

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(backend Backend, authorizer facade.Authorizer) (*CloudAPI, error) {

	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	getUserAuthFunc := func() (common.AuthFunc, error) {
		authUser, _ := authorizer.GetAuthTag().(names.UserTag)
		isAdmin, err := authorizer.HasPermission(description.SuperuserAccess, backend.ControllerTag())
		if err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		return func(tag names.Tag) bool {
			userTag, ok := tag.(names.UserTag)
			if !ok {
				return false
			}
			return isAdmin || userTag.Canonical() == authUser.Canonical()
		}, nil
	}
	return &CloudAPI{
		backend:                backend,
		authorizer:             authorizer,
		getCredentialsAuthFunc: getUserAuthFunc,
	}, nil
}

// Cloud returns the cloud definitions for the specified clouds.
func (mm *CloudAPI) Cloud(args params.Entities) (params.CloudResults, error) {
	results := params.CloudResults{
		Results: make([]params.CloudResult, len(args.Entities)),
	}
	one := func(arg params.Entity) (*params.Cloud, error) {
		tag, err := names.ParseCloudTag(arg.Tag)
		if err != nil {
			return nil, err
		}
		cloud, err := mm.backend.Cloud(tag.Id())
		if err != nil {
			return nil, err
		}
		authTypes := make([]string, len(cloud.AuthTypes))
		for i, authType := range cloud.AuthTypes {
			authTypes[i] = string(authType)
		}
		regions := make([]params.CloudRegion, len(cloud.Regions))
		for i, region := range cloud.Regions {
			regions[i] = params.CloudRegion{
				Name:             region.Name,
				Endpoint:         region.Endpoint,
				IdentityEndpoint: region.IdentityEndpoint,
				StorageEndpoint:  region.StorageEndpoint,
			}
		}
		return &params.Cloud{
			Type:             cloud.Type,
			AuthTypes:        authTypes,
			Endpoint:         cloud.Endpoint,
			IdentityEndpoint: cloud.IdentityEndpoint,
			StorageEndpoint:  cloud.StorageEndpoint,
			Regions:          regions,
		}, nil
	}
	for i, arg := range args.Entities {
		cloud, err := one(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		} else {
			results.Results[i].Cloud = cloud
		}
	}
	return results, nil
}

// DefaultCloud returns the tag of the cloud that models will be
// created in by default.
func (mm *CloudAPI) DefaultCloud() (params.StringResult, error) {
	controllerModel, err := mm.backend.ControllerModel()
	if err != nil {
		return params.StringResult{}, err
	}

	return params.StringResult{
		Result: names.NewCloudTag(controllerModel.Cloud()).String(),
	}, nil
}

// Credentials returns the cloud credentials for a set of users.
func (mm *CloudAPI) Credentials(args params.UserClouds) (params.StringsResults, error) {
	results := params.StringsResults{
		Results: make([]params.StringsResult, len(args.UserClouds)),
	}
	authFunc, err := mm.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}
	for i, arg := range args.UserClouds {
		userTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(userTag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		cloudTag, err := names.ParseCloudTag(arg.CloudTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		cloudCredentials, err := mm.backend.CloudCredentials(userTag, cloudTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		out := make([]string, 0, len(cloudCredentials))
		for tag := range cloudCredentials {
			out = append(out, tag.String())
		}
		results.Results[i].Result = out
	}
	return results, nil
}

// UpdateCredentials updates a set of cloud credentials.
func (mm *CloudAPI) UpdateCredentials(args params.UpdateCloudCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Credentials)),
	}
	authFunc, err := mm.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}
	for i, arg := range args.Credentials {
		tag, err := names.ParseCloudCredentialTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		in := cloud.NewCredential(
			cloud.AuthType(arg.Credential.AuthType),
			arg.Credential.Attributes,
		)
		if err := mm.backend.UpdateCloudCredential(tag, in); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}
