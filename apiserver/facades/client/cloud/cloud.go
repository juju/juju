// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud defines an API end point for functions dealing with
// the controller's cloud definition, and cloud credentials.
package cloud

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/txn"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.cloud")

// CloudV6 defines the methods on the cloud API facade, version 6.
type CloudV6 interface {
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CheckCredentialsModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
	RevokeCredentialsCheckModels(args params.RevokeCredentialArgs) (params.ErrorResults, error)
	UpdateCredentialsCheckModels(args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	UpdateCloud(cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error)
}

// CloudV5 defines the methods on the cloud API facade, version 5.
type CloudV5 interface {
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CheckCredentialsModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
	RevokeCredentialsCheckModels(args params.RevokeCredentialArgs) (params.ErrorResults, error)
	UpdateCredentialsCheckModels(args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	UpdateCloud(cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error)
}

// CloudV4 defines the methods on the cloud API facade, version 4.
type CloudV4 interface {
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CheckCredentialsModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	DefaultCloud() (params.StringResult, error)
	ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
	RevokeCredentialsCheckModels(args params.RevokeCredentialArgs) (params.ErrorResults, error)
	UpdateCredentialsCheckModels(args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	UpdateCloud(cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error)
}

// CloudV3 defines the methods on the cloud API facade, version 3.
type CloudV3 interface {
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CheckCredentialsModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	DefaultCloud() (params.StringResult, error)
	ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
	RevokeCredentialsCheckModels(args params.RevokeCredentialArgs) (params.ErrorResults, error)
	UpdateCredentialsCheckModels(args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
}

// CloudV2 defines the methods on the cloud API facade, version 2.
type CloudV2 interface {
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	DefaultCloud() (params.StringResult, error)
	RemoveClouds(args params.Entities) (params.ErrorResults, error)
	RevokeCredentials(args params.Entities) (params.ErrorResults, error)
	UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
}

// CloudV1 defines the methods on the cloud API facade, version 1.
type CloudV1 interface {
	Cloud(args params.Entities) (params.CloudResults, error)
	Clouds() (params.CloudsResult, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	DefaultCloud() (params.StringResult, error)
	RevokeCredentials(args params.Entities) (params.ErrorResults, error)
	UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
}

// CloudAPI implements the cloud interface and is the concrete implementation
// of the api end point.
type CloudAPI struct {
	backend                Backend
	ctlrBackend            Backend
	authorizer             facade.Authorizer
	apiUser                names.UserTag
	getCredentialsAuthFunc common.GetAuthFunc
	pool                   ModelPoolBackend
}

// CloudAPIV5 provides a way to wrap the different calls
// between version 5 and version 6 of the cloud API.
type CloudAPIV5 struct {
	*CloudAPI
}

// CloudAPIV4 provides a way to wrap the different calls
// between version 4 and version 5 of the cloud API.
type CloudAPIV4 struct {
	*CloudAPIV5
}

// CloudAPIV3 provides a way to wrap the different calls
// between version 3 and version 4 of the cloud API.
type CloudAPIV3 struct {
	*CloudAPIV4
}

// CloudAPIV2 provides a way to wrap the different calls
// between version 2 and version 3 of the cloud API.
type CloudAPIV2 struct {
	*CloudAPIV3
}

// CloudAPIV1 provides a way to wrap the different calls
// between version 1 and version 2 of the cloud API.
type CloudAPIV1 struct {
	*CloudAPIV2
}

var (
	_ CloudV6 = (*CloudAPI)(nil)
	_ CloudV5 = (*CloudAPIV5)(nil)
	_ CloudV4 = (*CloudAPIV4)(nil)
	_ CloudV3 = (*CloudAPIV3)(nil)
	_ CloudV2 = (*CloudAPIV2)(nil)
	_ CloudV1 = (*CloudAPIV1)(nil)
)

// NewFacadeV6 is used for API registration.
func NewFacadeV6(context facade.Context) (*CloudAPI, error) {
	st := NewStateBackend(context.State())
	pool := NewModelPoolBackend(context.StatePool())
	ctlrSt := NewStateBackend(pool.SystemState())
	return NewCloudAPI(st, ctlrSt, pool, context.Auth())
}

// NewFacadeV5 is used for API registration.
func NewFacadeV5(context facade.Context) (*CloudAPIV5, error) {
	v6, err := NewFacadeV6(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV5{v6}, nil
}

// NewFacadeV4 is used for API registration.
func NewFacadeV4(context facade.Context) (*CloudAPIV4, error) {
	v5, err := NewFacadeV5(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV4{v5}, nil
}

// NewFacadeV3 is used for API registration.
func NewFacadeV3(context facade.Context) (*CloudAPIV3, error) {
	v4, err := NewFacadeV4(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV3{v4}, nil
}

// NewFacadeV2 is used for API registration.
func NewFacadeV2(context facade.Context) (*CloudAPIV2, error) {
	v3, err := NewFacadeV3(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV2{v3}, nil
}

// NewFacadeV1 is used for API registration.
func NewFacadeV1(context facade.Context) (*CloudAPIV1, error) {
	v2, err := NewFacadeV2(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV1{v2}, nil
}

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(backend, ctlrBackend Backend, pool ModelPoolBackend, authorizer facade.Authorizer) (*CloudAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	authUser, _ := authorizer.GetAuthTag().(names.UserTag)
	getUserAuthFunc := func() (common.AuthFunc, error) {
		isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, backend.ControllerTag())
		if err != nil && !errors.IsNotFound(err) {
			return nil, err
		}
		return func(tag names.Tag) bool {
			userTag, ok := tag.(names.UserTag)
			if !ok {
				return false
			}
			return isAdmin || userTag == authUser
		}, nil
	}
	return &CloudAPI{
		backend:                backend,
		ctlrBackend:            ctlrBackend,
		authorizer:             authorizer,
		getCredentialsAuthFunc: getUserAuthFunc,
		apiUser:                authUser,
		pool:                   pool,
	}, nil
}

func (api *CloudAPI) canAccessCloud(cloud string, user names.UserTag, access permission.Access) (bool, error) {
	perm, err := api.ctlrBackend.GetCloudAccess(cloud, user)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return perm.EqualOrGreaterCloudAccessThan(access), nil
}

// Clouds returns the definitions of all clouds supported by the controller
// that the logged in user can see.
func (api *CloudAPI) Clouds() (params.CloudsResult, error) {
	var result params.CloudsResult
	clouds, err := api.backend.Clouds()
	if err != nil {
		return result, err
	}
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return result, errors.Trace(err)
	}
	result.Clouds = make(map[string]params.Cloud)
	for tag, aCloud := range clouds {
		// Ensure user has permission to see the cloud.
		if !isAdmin {
			canAccess, err := api.canAccessCloud(tag.Id(), api.apiUser, permission.AddModelAccess)
			if err != nil {
				return result, err
			}
			if !canAccess {
				continue
			}
		}
		paramsCloud := common.CloudToParams(aCloud)
		result.Clouds[tag.String()] = paramsCloud
	}
	return result, nil
}

// Cloud returns the cloud definitions for the specified clouds.
func (api *CloudAPI) Cloud(args params.Entities) (params.CloudResults, error) {
	results := params.CloudResults{
		Results: make([]params.CloudResult, len(args.Entities)),
	}
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	}
	one := func(arg params.Entity) (*params.Cloud, error) {
		tag, err := names.ParseCloudTag(arg.Tag)
		if err != nil {
			return nil, err
		}
		// Ensure user has permission to see the cloud.
		if !isAdmin {
			canAccess, err := api.canAccessCloud(tag.Id(), api.apiUser, permission.AddModelAccess)
			if err != nil {
				return nil, err
			}
			if !canAccess {
				return nil, errors.NotFoundf("cloud %q", tag.Id())
			}
		}
		aCloud, err := api.backend.Cloud(tag.Id())
		if err != nil {
			return nil, err
		}
		paramsCloud := common.CloudToParams(aCloud)
		return &paramsCloud, nil
	}
	for i, arg := range args.Entities {
		aCloud, err := one(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		} else {
			results.Results[i].Cloud = aCloud
		}
	}
	return results, nil
}

// CloudInfo returns information about the specified clouds.
func (api *CloudAPI) CloudInfo(args params.Entities) (params.CloudInfoResults, error) {
	results := params.CloudInfoResults{
		Results: make([]params.CloudInfoResult, len(args.Entities)),
	}

	oneCloudInfo := func(arg params.Entity) (*params.CloudInfo, error) {
		tag, err := names.ParseCloudTag(arg.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return api.getCloudInfo(tag)
	}

	for i, arg := range args.Entities {
		cloudInfo, err := oneCloudInfo(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = cloudInfo
	}
	return results, nil
}

func cloudToParams(cloud cloud.Cloud) params.CloudDetails {
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
	return params.CloudDetails{
		Type:             cloud.Type,
		AuthTypes:        authTypes,
		Endpoint:         cloud.Endpoint,
		IdentityEndpoint: cloud.IdentityEndpoint,
		StorageEndpoint:  cloud.StorageEndpoint,
		Regions:          regions,
	}
}

func (api *CloudAPI) getCloudInfo(tag names.CloudTag) (*params.CloudInfo, error) {
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	// If not a controller admin, check for cloud admin.
	if !isAdmin {
		perm, err := api.ctlrBackend.GetCloudAccess(tag.Id(), api.apiUser)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		isAdmin = perm == permission.AdminAccess
	}

	aCloud, err := api.backend.Cloud(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := params.CloudInfo{
		CloudDetails: cloudToParams(aCloud),
	}

	cloudUsers, err := api.ctlrBackend.GetCloudUsers(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for userId, perm := range cloudUsers {
		if !isAdmin && api.apiUser.Id() != userId {
			// The authenticated user is neither the a controller
			// superuser, a cloud administrator, nor a cloud user, so
			// has no business knowing about the cloud user.
			continue
		}
		userTag := names.NewUserTag(userId)
		displayName := userId
		if userTag.IsLocal() {
			u, err := api.backend.User(userTag)
			if err != nil {
				if _, ok := err.(state.DeletedUserError); !ok {
					// We ignore deleted users for now. So if it is not a
					// DeletedUserError we return the error.
					return nil, errors.Trace(err)
				}
				continue
			}
			displayName = u.DisplayName()
		}

		userInfo := params.CloudUserInfo{
			UserName:    userId,
			DisplayName: displayName,
			Access:      string(perm),
		}
		info.Users = append(info.Users, userInfo)
	}

	if len(info.Users) == 0 {
		// No users, which means the authenticated user doesn't
		// have access to the cloud.
		return nil, errors.Trace(common.ErrPerm)
	}
	return &info, nil
}

// ListCloudInfo returns clouds that the specified user has access to.
// Controller admins (superuser) can list clouds for any user.
// Other users can only ask about their own clouds.
func (api *CloudAPI) ListCloudInfo(req params.ListCloudsRequest) (params.ListCloudInfoResults, error) {
	result := params.ListCloudInfoResults{}

	userTag, err := names.ParseUserTag(req.UserTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	cloudInfos, err := api.ctlrBackend.CloudsForUser(userTag, req.All)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, ci := range cloudInfos {
		info := &params.ListCloudInfo{
			CloudDetails: cloudToParams(ci.Cloud),
			Access:       string(ci.Access),
		}
		result.Results = append(result.Results, params.ListCloudInfoResult{Result: info})
	}
	return result, nil
}

// DefaultCloud returns the tag of the cloud that models will be
// created in by default.
func (api *CloudAPIV4) DefaultCloud() (params.StringResult, error) {
	controllerModel, err := api.ctlrBackend.Model()
	if err != nil {
		return params.StringResult{}, err
	}
	return params.StringResult{
		Result: names.NewCloudTag(controllerModel.CloudName()).String(),
	}, nil
}

// UserCredentials returns the cloud credentials for a set of users.
func (api *CloudAPI) UserCredentials(args params.UserClouds) (params.StringsResults, error) {
	results := params.StringsResults{
		Results: make([]params.StringsResult, len(args.UserClouds)),
	}
	authFunc, err := api.getCredentialsAuthFunc()
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
		cloudCredentials, err := api.backend.CloudCredentials(userTag, cloudTag.Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		out := make([]string, 0, len(cloudCredentials))
		for tagId := range cloudCredentials {
			if !names.IsValidCloudCredential(tagId) {
				results.Results[i].Error = common.ServerError(errors.NotValidf("cloud credential ID %q", tagId))
				continue
			}
			out = append(out, names.NewCloudCredentialTag(tagId).String())
		}
		results.Results[i].Result = out
	}
	return results, nil
}

// AddCredentials adds new credentials.
// In contrast to UpdateCredentials() below, the new credentials can be
// for a cloud that the controller does not manage (this is required
// for CAAS models)
func (api *CloudAPI) AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Credentials)),
	}

	authFunc, err := api.getCredentialsAuthFunc()
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
		if err := api.backend.UpdateCloudCredential(tag, in); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

// CheckCredentialsModels validates supplied cloud credentials' content against
// models that currently use these credentials.
// If there are any models that are using a credential and these models or their
// cloud instances are not going to be accessible with corresponding credential,
// there will be detailed validation errors per model.
func (api *CloudAPI) CheckCredentialsModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
	return api.commonUpdateCredentials(false, false, args)
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
// Controller admins can 'force' an update of the credential
// regardless of whether it is deemed valid or not.
func (api *CloudAPI) UpdateCredentialsCheckModels(args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error) {
	return api.commonUpdateCredentials(true, args.Force, params.TaggedCredentials{args.Credentials})
}

func (api *CloudAPI) commonUpdateCredentials(update bool, force bool, args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
	if force {
		// Only controller admins can ask for an update to be forced.
		isControllerAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
		if err != nil && !errors.IsNotFound(err) {
			return params.UpdateCredentialResults{}, errors.Trace(err)
		}
		if !isControllerAdmin {
			return params.UpdateCredentialResults{}, errors.Annotatef(common.ErrBadRequest, "unexpected force specified")
		}
	}

	authFunc, err := api.getCredentialsAuthFunc()
	if err != nil {
		return params.UpdateCredentialResults{}, err
	}

	results := make([]params.UpdateCredentialResult, len(args.Credentials))
	for i, arg := range args.Credentials {
		results[i].CredentialTag = arg.Tag
		tag, err := names.ParseCloudCredentialTag(arg.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		in := cloud.NewCredential(
			cloud.AuthType(arg.Credential.AuthType),
			arg.Credential.Attributes,
		)

		models, err := api.credentialModels(tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			if !force {
				// Could not determine if credential has models - do not continue updating this credential...
				continue
			}
		}

		var modelsErred bool
		if len(models) > 0 {
			var modelsResult []params.UpdateCredentialModelResult
			for uuid, name := range models {
				model := params.UpdateCredentialModelResult{
					ModelUUID: uuid,
					ModelName: name,
				}
				model.Errors = api.validateCredentialForModel(uuid, tag, &in)
				modelsResult = append(modelsResult, model)
				if len(model.Errors) > 0 {
					modelsErred = true
				}
			}
			// since we get a map above, for consistency ensure that models are added
			// sorted by model uuid.
			sort.Slice(modelsResult, func(i, j int) bool {
				return modelsResult[i].ModelUUID < modelsResult[j].ModelUUID
			})
			results[i].Models = modelsResult
		}

		if modelsErred {
			results[i].Error = common.ServerError(errors.New("some models are no longer visible"))
			if !force {
				// Some models that use this credential do not like the new content, do not update the credential...
				continue
			}
		}

		if update {
			if err := api.backend.UpdateCloudCredential(tag, in); err != nil {
				if errors.IsNotFound(err) {
					err = errors.Errorf(
						"cannot update credential %q: controller does not manage cloud %q",
						tag.Name(), tag.Cloud().Id())
				}
				results[i].Error = common.ServerError(err)
			}
		}
	}
	return params.UpdateCredentialResults{results}, nil
}

func (api *CloudAPI) credentialModels(tag names.CloudCredentialTag) (map[string]string, error) {
	models, err := api.backend.CredentialModels(tag)
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	return models, nil
}

func (api *CloudAPI) validateCredentialForModel(modelUUID string, tag names.CloudCredentialTag, credential *cloud.Credential) []params.ErrorResult {
	var result []params.ErrorResult

	m, callContext, err := api.pool.GetModelCallContext(modelUUID)
	if err != nil {
		return append(result, params.ErrorResult{common.ServerError(err)})
	}

	modelErrors, err := validateNewCredentialForModelFunc(
		m,
		callContext,
		tag,
		credential,
		false,
	)
	if err != nil {
		return append(result, params.ErrorResult{common.ServerError(err)})
	}
	if len(modelErrors.Results) > 0 {
		return append(result, modelErrors.Results...)
	}
	return result
}

var validateNewCredentialForModelFunc = credentialcommon.ValidateNewModelCredential

// Mask out old methods from the new API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
// UpdateCredentials was dropped in V3, replaced with UpdateCredentialsCheckModels.
func (*CloudAPI) UpdateCredentials(_, _ struct{}) {}

// Mask out old methods from the new API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// CheckCredentialsModels did not exist before V3.
func (*CloudAPIV2) CheckCredentialsModels(_, _ struct{}) {}

// DefaultCloud is gone in V5.
func (*CloudAPI) DefaultCloud(_, _ struct{}) {}

// UpdateCredentials updates a set of cloud credentials' content.
func (api *CloudAPIV2) UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Credentials)),
	}

	updateResults, err := api.commonUpdateCredentials(true, false, args)
	if err != nil {
		return results, err
	}

	// If there are any models that are using a credential and these models
	// are not going to be visible with updated credential content,
	// there will be detailed validation errors per model.
	// However, old return parameter structure could not hold this much detail and,
	// thus, per-model-per-credential errors are squashed into per-credential errors.
	for i, result := range updateResults.Results {
		var resultErrors []params.ErrorResult
		if result.Error != nil {
			resultErrors = append(resultErrors, params.ErrorResult{result.Error})
		}
		for _, m := range result.Models {
			if len(m.Errors) > 0 {
				modelErors := params.ErrorResults{m.Errors}
				combined := errors.Annotatef(modelErors.Combine(), "model %q (uuid %v)", m.ModelName, m.ModelUUID)
				resultErrors = append(resultErrors, params.ErrorResult{common.ServerError(combined)})
			}
		}
		if len(resultErrors) == 1 {
			results.Results[i].Error = resultErrors[0].Error
			continue
		}
		if len(resultErrors) > 1 {
			credentialError := params.ErrorResults{resultErrors}
			results.Results[i].Error = common.ServerError(credentialError.Combine())
		}
	}
	return results, nil
}

// Mask out old methods from the new API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// RevokeCredentials was dropped in V3, replaced with RevokeCredentialsCheckModel.
func (*CloudAPI) RevokeCredentials(_, _ struct{}) {}

// UpdateCredentials updates a set of cloud credentials' content.
func (api *CloudAPIV2) RevokeCredentials(args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	authFunc, err := api.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}
	for i, arg := range args.Entities {
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

		models, err := api.credentialModels(tag)
		if err != nil {
			logger.Warningf("could not get models that use credential %v: %v", tag, err)
		}
		if len(models) > 0 {
			// For backward compatibility, we must proceed here regardless of whether the credential is used by any models,
			// but, at least, let's log it.
			logger.Warningf("credential %v will be deleted but it is still used by model%v", tag, modelsPretty(models))
		}

		if err := api.backend.RemoveCloudCredential(tag); err != nil {
			results.Results[i].Error = common.ServerError(err)
		}
	}
	return results, nil
}

// Mask out old methods from the new API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// RevokeCredentialsCheckModels did not exist before V3.
func (*CloudAPIV2) RevokeCredentialsCheckModels(_, _ struct{}) {}

func plural(length int) string {
	if length == 1 {
		return ""
	}
	return "s"
}

func modelsPretty(in map[string]string) string {
	// map keys are notoriously randomly ordered
	uuids := []string{}
	for uuid := range in {
		uuids = append(uuids, uuid)
	}
	sort.Strings(uuids)

	firstLine := ":\n- "
	if len(uuids) == 1 {
		firstLine = " "
	}

	return fmt.Sprintf("%v%v%v",
		plural(len(in)),
		firstLine,
		strings.Join(uuids, "\n- "),
	)
}

// RevokeCredentialsCheckModels revokes a set of cloud credentials.
// If the credentials are used by any of the models, the credential deletion will be aborted.
// If credential-in-use needs to be revoked nonetheless, this method allows the use of force.
func (api *CloudAPI) RevokeCredentialsCheckModels(args params.RevokeCredentialArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Credentials)),
	}
	authFunc, err := api.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}

	opMessage := func(force bool) string {
		if force {
			return "will be deleted but"
		}
		return "cannot be deleted as"
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

		models, err := api.credentialModels(tag)
		if err != nil {
			if !arg.Force {
				// Could not determine if credential has models - do not continue revoking this credential...
				results.Results[i].Error = common.ServerError(err)
				continue
			}
			logger.Warningf("could not get models that use credential %v: %v", tag, err)
		}
		if len(models) != 0 {
			logger.Warningf("credential %v %v it is used by model%v",
				tag,
				opMessage(arg.Force),
				modelsPretty(models),
			)
			if !arg.Force {
				// Some models still use this credential - do not delete this credential...
				results.Results[i].Error = common.ServerError(errors.Errorf("cannot revoke credential %v: it is still used by %d model%v", tag, len(models), plural(len(models))))
				continue
			}
		}
		err = api.backend.RemoveCloudCredential(tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
		} else {
			// If credential was successfully removed, we also want to clear all references to it from the models.
			// lp#1841885
			if err := api.backend.RemoveModelsCredential(tag); err != nil {
				results.Results[i].Error = common.ServerError(err)
			}
		}
	}
	return results, nil
}

// Credential returns the specified cloud credential for each tag, minus secrets.
func (api *CloudAPI) Credential(args params.Entities) (params.CloudCredentialResults, error) {
	results := params.CloudCredentialResults{
		Results: make([]params.CloudCredentialResult, len(args.Entities)),
	}
	authFunc, err := api.getCredentialsAuthFunc()
	if err != nil {
		return results, err
	}

	for i, arg := range args.Entities {
		credentialTag, err := names.ParseCloudCredentialTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if !authFunc(credentialTag.Owner()) {
			results.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		// Helper to look up and cache credential schemas for clouds.
		schemaCache := make(map[string]map[cloud.AuthType]cloud.CredentialSchema)
		credentialSchemas := func() (map[cloud.AuthType]cloud.CredentialSchema, error) {
			cloudName := credentialTag.Cloud().Id()
			if s, ok := schemaCache[cloudName]; ok {
				return s, nil
			}
			aCloud, err := api.backend.Cloud(cloudName)
			if err != nil {
				return nil, err
			}
			aProvider, err := environs.Provider(aCloud.Type)
			if err != nil {
				return nil, err
			}
			schema := aProvider.CredentialSchemas()
			schemaCache[cloudName] = schema
			return schema, nil
		}
		cloudCredentials, err := api.backend.CloudCredentials(credentialTag.Owner(), credentialTag.Cloud().Id())
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		cred, ok := cloudCredentials[credentialTag.Id()]
		if !ok {
			results.Results[i].Error = common.ServerError(errors.NotFoundf("credential %q", credentialTag.Name()))
			continue
		}

		schemas, err := credentialSchemas()
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}

		attrs := cred.Attributes
		var redacted []string
		// Mask out the secrets.
		if s, ok := schemas[cloud.AuthType(cred.AuthType)]; ok {
			for _, attr := range s {
				if attr.Hidden {
					delete(attrs, attr.Name)
					redacted = append(redacted, attr.Name)
				}
			}
		}
		results.Results[i].Result = &params.CloudCredential{
			AuthType:   cred.AuthType,
			Attributes: attrs,
			Redacted:   redacted,
		}
	}
	return results, nil
}

// AddCloud adds a new cloud, different from the one managed by the controller.
func (api *CloudAPI) AddCloud(cloudArgs params.AddCloudArgs) error {
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	} else if !isAdmin {
		return common.ServerError(common.ErrPerm)
	}

	if cloudArgs.Cloud.Type != string(provider.K8s_ProviderType) {
		// All non-k8s cloud need to go through whitelist.
		controllerInfo, err := api.backend.ControllerInfo()
		if err != nil {
			return errors.Trace(err)
		}
		controllerCloud, err := api.backend.Cloud(controllerInfo.CloudName)
		if err != nil {
			return errors.Trace(err)
		}
		if err := cloud.CurrentWhiteList().Check(controllerCloud.Type, cloudArgs.Cloud.Type); err != nil {
			if cloudArgs.Force == nil || !*cloudArgs.Force {
				return common.ServerError(params.Error{Code: params.CodeIncompatibleClouds, Message: err.Error()})
			}
			logger.Infof("force adding cloud %q of type %q to controller bootstrapped on cloud type %q", cloudArgs.Name, cloudArgs.Cloud.Type, controllerCloud.Type)
		}
	}

	aCloud := common.CloudFromParams(cloudArgs.Name, cloudArgs.Cloud)
	// All clouds must have at least one 'default' region, lp#1819409.
	if len(aCloud.Regions) == 0 {
		aCloud.Regions = []cloud.Region{{Name: cloud.DefaultCloudRegion}}
	}

	err = api.backend.AddCloud(aCloud, api.apiUser.Name())
	return errors.Trace(err)
}

// UpdateCloud updates an existing cloud that the controller knows about.
func (api *CloudAPI) UpdateCloud(cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(cloudArgs.Clouds)),
	}
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	} else if !isAdmin {
		return results, common.ServerError(common.ErrPerm)
	}
	for i, aCloud := range cloudArgs.Clouds {
		err := api.backend.UpdateCloud(common.CloudFromParams(aCloud.Name, aCloud.Cloud))
		results.Results[i].Error = common.ServerError(err)
	}
	return results, nil
}

// Mask out new methods from the new older API versions. The API reflection
// code in rpc/rpcreflect/type.go:newMethod skips 2-argument methods,
// so this removes the method as far as the RPC machinery is concerned.
//
// UpdateCloud did not exist before V4.
func (*CloudAPIV3) UpdateCloud(_, _ struct{}) {}

// RemoveClouds removes the specified clouds from the controller.
// If a cloud is in use (has models deployed to it), the removal will fail.
func (api *CloudAPI) RemoveClouds(args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	isAdmin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return result, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		// Ensure user has permission to remove the cloud.
		if !isAdmin {
			canAccess, err := api.canAccessCloud(tag.Id(), api.apiUser, permission.AdminAccess)
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			if !canAccess {
				result.Results[i].Error = common.ServerError(common.ErrPerm)
				continue
			}
		}
		err = api.backend.RemoveCloud(tag.Id())
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// CredentialContents returns the specified cloud credentials,
// including the secrets if requested.
// If no specific credential name/cloud was passed in, all credentials for this user
// are returned.
// Only credential owner can see its contents as well as what models use it.
// Controller admin has no special superpowers here and is treated the same as all other users.
func (api *CloudAPIV5) CredentialContents(args params.CloudCredentialArgs) (params.CredentialContentResults, error) {
	return api.internalCredentialContents(args, false)
}

// CredentialContents returns the specified cloud credentials,
// including the secrets if requested.
// If no specific credential name/cloud was passed in, all credentials for this user
// are returned.
// Only credential owner can see its contents as well as what models use it.
// Controller admin has no special superpowers here and is treated the same as all other users.
func (api *CloudAPI) CredentialContents(args params.CloudCredentialArgs) (params.CredentialContentResults, error) {
	return api.internalCredentialContents(args, true)
}

func (api *CloudAPI) internalCredentialContents(args params.CloudCredentialArgs, includeValidity bool) (params.CredentialContentResults, error) {
	// Helper to look up and cache credential schemas for clouds.
	schemaCache := make(map[string]map[cloud.AuthType]cloud.CredentialSchema)
	credentialSchemas := func(cloudName string) (map[cloud.AuthType]cloud.CredentialSchema, error) {
		if s, ok := schemaCache[cloudName]; ok {
			return s, nil
		}
		aCloud, err := api.backend.Cloud(cloudName)
		if err != nil {
			return nil, err
		}
		aProvider, err := environs.Provider(aCloud.Type)
		if err != nil {
			return nil, err
		}
		schema := aProvider.CredentialSchemas()
		schemaCache[cloudName] = schema
		return schema, nil
	}

	// Helper to parse state.Credential into an expected result item.
	stateIntoParam := func(credential state.Credential, includeSecrets bool) params.CredentialContentResult {
		schemas, err := credentialSchemas(credential.Cloud)
		if err != nil {
			return params.CredentialContentResult{Error: common.ServerError(err)}
		}
		attrs := map[string]string{}
		// Filter out the secrets.
		if s, ok := schemas[cloud.AuthType(credential.AuthType)]; ok {
			for _, attr := range s {
				if value, exists := credential.Attributes[attr.Name]; exists {
					if attr.Hidden && !includeSecrets {
						continue
					}
					attrs[attr.Name] = value
				}
			}
		}
		info := params.ControllerCredentialInfo{
			Content: params.CredentialContent{
				Name:       credential.Name,
				AuthType:   credential.AuthType,
				Attributes: attrs,
				Cloud:      credential.Cloud,
			},
		}
		if includeValidity {
			valid := credential.IsValid()
			info.Content.Valid = &valid
		}

		// get models
		tag, err := credential.CloudCredentialTag()
		if err != nil {
			return params.CredentialContentResult{Error: common.ServerError(err)}
		}

		models, err := api.backend.CredentialModelsAndOwnerAccess(tag)
		if err != nil && !errors.IsNotFound(err) {
			return params.CredentialContentResult{Error: common.ServerError(err)}
		}
		info.Models = make([]params.ModelAccess, len(models))
		for i, m := range models {
			info.Models[i] = params.ModelAccess{m.ModelName, string(m.OwnerAccess)}
		}

		return params.CredentialContentResult{Result: &info}
	}

	var result []params.CredentialContentResult
	if len(args.Credentials) == 0 {
		credentials, err := api.backend.AllCloudCredentials(api.apiUser)
		if err != nil {
			return params.CredentialContentResults{}, errors.Trace(err)
		}
		result = make([]params.CredentialContentResult, len(credentials))
		for i, credential := range credentials {
			result[i] = stateIntoParam(credential, args.IncludeSecrets)
		}
	} else {
		// Helper to construct credential tag from cloud and name.
		credId := func(cloudName, credentialName string) string {
			return fmt.Sprintf("%s/%s/%s",
				cloudName, api.apiUser.Id(), credentialName,
			)
		}

		result = make([]params.CredentialContentResult, len(args.Credentials))
		for i, given := range args.Credentials {
			id := credId(given.CloudName, given.CredentialName)
			if !names.IsValidCloudCredential(id) {
				result[i] = params.CredentialContentResult{
					Error: common.ServerError(errors.NotValidf("cloud credential ID %q", id)),
				}
				continue
			}
			tag := names.NewCloudCredentialTag(id)
			credential, err := api.backend.CloudCredential(tag)
			if err != nil {
				result[i] = params.CredentialContentResult{
					Error: common.ServerError(err),
				}
				continue
			}
			result[i] = stateIntoParam(credential, args.IncludeSecrets)
		}
	}
	return params.CredentialContentResults{result}, nil
}

// ModifyCloudAccess changes the model access granted to users.
func (c *CloudAPI) ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		cloudTag, err := names.ParseCloudTag(arg.CloudTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		_, err = c.backend.Cloud(cloudTag.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if c.apiUser.String() == arg.UserTag {
			result.Results[i].Error = common.ServerError(errors.New("cannot change your own cloud access"))
			continue
		}

		isAdmin, err := c.authorizer.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !isAdmin {
			callerAccess, err := c.backend.GetCloudAccess(cloudTag.Id(), c.apiUser)
			if err != nil {
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			if callerAccess != permission.AdminAccess {
				result.Results[i].Error = common.ServerError(common.ErrPerm)
				continue
			}
		}

		cloudAccess := permission.Access(arg.Access)
		if err := permission.ValidateCloudAccess(cloudAccess); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify cloud access"))
			continue
		}

		result.Results[i].Error = common.ServerError(
			ChangeCloudAccess(c.backend, cloudTag.Id(), targetUserTag, arg.Action, cloudAccess))
	}
	return result, nil
}

// ChangeCloudAccess performs the requested access grant or revoke action for the
// specified user on the cloud.
func ChangeCloudAccess(backend Backend, cloud string, targetUserTag names.UserTag, action params.CloudAction, access permission.Access) error {
	switch action {
	case params.GrantCloudAccess:
		err := grantCloudAccess(backend, cloud, targetUserTag, access)
		if err != nil {
			return errors.Annotate(err, "could not grant cloud access")
		}
		return nil
	case params.RevokeCloudAccess:
		return revokeCloudAccess(backend, cloud, targetUserTag, access)
	default:
		return errors.Errorf("unknown action %q", action)
	}
}

func grantCloudAccess(backend Backend, cloud string, targetUserTag names.UserTag, access permission.Access) error {
	err := backend.CreateCloudAccess(cloud, targetUserTag, access)
	if errors.IsAlreadyExists(err) {
		cloudAccess, err := backend.GetCloudAccess(cloud, targetUserTag)
		if errors.IsNotFound(err) {
			// Conflicts with prior check, must be inconsistent state.
			err = txn.ErrExcessiveContention
		}
		if err != nil {
			return errors.Annotate(err, "could not look up cloud access for user")
		}

		// Only set access if greater access is being granted.
		if cloudAccess.EqualOrGreaterCloudAccessThan(access) {
			return errors.Errorf("user already has %q access or greater", access)
		}
		if err = backend.UpdateCloudAccess(cloud, targetUserTag, access); err != nil {
			return errors.Annotate(err, "could not set cloud access for user")
		}
		return nil

	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func revokeCloudAccess(backend Backend, cloud string, targetUserTag names.UserTag, access permission.Access) error {
	switch access {
	case permission.AddModelAccess:
		// Revoking add-model access removes all access.
		err := backend.RemoveCloudAccess(cloud, targetUserTag)
		return errors.Annotate(err, "could not revoke cloud access")
	case permission.AdminAccess:
		// Revoking admin sets add-model.
		err := backend.UpdateCloudAccess(cloud, targetUserTag, permission.AddModelAccess)
		return errors.Annotate(err, "could not set cloud access to add-model")

	default:
		return errors.Errorf("don't know how to revoke %q access", access)
	}
}
