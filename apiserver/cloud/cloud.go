// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud defines an API end point for functions dealing with
// the controller's cloud definition, and cloud credentials.
package cloud

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
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
	backend                  Backend
	authorizer               facade.Authorizer
	apiUser                  names.UserTag
	getCredentialsAuthFunc   common.GetAuthFunc
	getCloudDefaultsAuthFunc common.GetAuthFunc
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
		isAdmin, err := backend.IsControllerAdmin(authUser)
		if err != nil {
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
		backend:                  backend,
		authorizer:               authorizer,
		getCredentialsAuthFunc:   getUserAuthFunc,
		getCloudDefaultsAuthFunc: getUserAuthFunc,
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
				Name:            region.Name,
				Endpoint:        region.Endpoint,
				StorageEndpoint: region.StorageEndpoint,
			}
		}
		return &params.Cloud{
			Type:            cloud.Type,
			AuthTypes:       authTypes,
			Endpoint:        cloud.Endpoint,
			StorageEndpoint: cloud.StorageEndpoint,
			Regions:         regions,
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

// CloudDefaults returns the cloud defaults for a set of users.
func (mm *CloudAPI) CloudDefaults(args params.Entities) (params.CloudDefaultsResults, error) {
	results := params.CloudDefaultsResults{
		Results: make([]params.CloudDefaultsResult, len(args.Entities)),
	}
	authFunc, err := mm.getCloudDefaultsAuthFunc()
	if err != nil {
		return results, err
	}
	controllerModel, err := mm.backend.ControllerModel()
	if err != nil {
		return results, err
	}
	for i, arg := range args.Entities {
		userTag, err := names.ParseUserTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(userTag) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		isAdmin, err := mm.backend.IsControllerAdmin(userTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		cloudDefaults := params.CloudDefaults{
			CloudTag:    names.NewCloudTag(controllerModel.Cloud()).String(),
			CloudRegion: controllerModel.CloudRegion(),
		}
		if isAdmin {
			// As a special case, controller admins will default to
			// using the same credential that was used to bootstrap.
			cloudDefaults.CloudCredential = controllerModel.CloudCredential()
		}
		results.Results[i].Result = &cloudDefaults
	}
	return results, nil
}

// Credentials returns the cloud credentials for a set of users.
func (mm *CloudAPI) Credentials(args params.UserClouds) (params.CloudCredentialsResults, error) {
	results := params.CloudCredentialsResults{
		Results: make([]params.CloudCredentialsResult, len(args.UserClouds)),
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

// UpdateCredentials updates the cloud credentials for a set of users.
func (mm *CloudAPI) UpdateCredentials(args params.UsersCloudCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Users)),
	}
	authFunc, err := mm.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}
	for i, arg := range args.Users {
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
		in := make(map[string]cloud.Credential)
		for name, credential := range arg.Credentials {
			in[name] = cloud.NewCredential(
				cloud.AuthType(credential.AuthType), credential.Attributes,
			)
		}
		if err := mm.backend.UpdateCloudCredentials(userTag, cloudTag.Id(), in); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}
