// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package cloud defines an API end point for functions dealing with
// the controller's cloud definition, and cloud credentials.
package cloud

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/naturalsort"
	"github.com/juju/txn"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// CloudV3 defines the methods on the cloud API facade, version 3.
type CloudV3 interface {
	Clouds() (params.CloudsResult, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	DefaultCloud() (params.StringResult, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	RevokeCredentials(args params.Entities) (params.ErrorResults, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	UpdateCredentialsCheckModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error)
	ModifyCloudAccess(args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
}

// CloudV2 defines the methods on the cloud API facade, version 2.
type CloudV2 interface {
	Clouds() (params.CloudsResult, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	DefaultCloud() (params.StringResult, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	RevokeCredentials(args params.Entities) (params.ErrorResults, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
	AddCloud(cloudArgs params.AddCloudArgs) error
	AddCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	CredentialContents(credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	RemoveClouds(args params.Entities) (params.ErrorResults, error)
}

// CloudV1 defines the methods on the cloud API facade, version 1.
type CloudV1 interface {
	Clouds() (params.CloudsResult, error)
	Cloud(args params.Entities) (params.CloudResults, error)
	DefaultCloud() (params.StringResult, error)
	UserCredentials(args params.UserClouds) (params.StringsResults, error)
	UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error)
	RevokeCredentials(args params.Entities) (params.ErrorResults, error)
	Credential(args params.Entities) (params.CloudCredentialResults, error)
}

// CloudAPI implements the cloud interface and is the concrete implementation
// of the api end point.
type CloudAPI struct {
	backend                Backend
	ctlrBackend            Backend
	authorizer             facade.Authorizer
	apiUser                names.UserTag
	getCredentialsAuthFunc common.GetAuthFunc
	callContext            environscontext.ProviderCallContext
	pool                   ModelPoolBackend
}

// CloudAPIV2 provides a way to wrap the different calls
// between version 2 and version 3 of the cloud API.
type CloudAPIV2 struct {
	*CloudAPI
}

// CloudAPIV1 provides a way to wrap the different calls
// between version 1 and version 2 of the cloud API.
type CloudAPIV1 struct {
	*CloudAPIV2
}

var (
	_ CloudV3 = (*CloudAPI)(nil)
	_ CloudV2 = (*CloudAPIV2)(nil)
	_ CloudV1 = (*CloudAPIV1)(nil)
)

// NewFacadeV3 is used for API registration.
func NewFacadeV3(context facade.Context) (*CloudAPI, error) {
	st := NewStateBackend(context.State())
	pool := NewModelPoolBackend(context.StatePool())
	ctlrSt := NewStateBackend(pool.SystemState())
	return NewCloudAPI(st, ctlrSt, pool, context.Auth(), state.CallContext(context.State()))
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
func NewCloudAPI(backend, ctlrBackend Backend, pool ModelPoolBackend, authorizer facade.Authorizer, callCtx environscontext.ProviderCallContext) (*CloudAPI, error) {
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
		callContext:            callCtx,
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
	for tag, cloud := range clouds {
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
		paramsCloud := common.CloudToParams(cloud)
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
		cloud, err := api.backend.Cloud(tag.Id())
		if err != nil {
			return nil, err
		}
		paramsCloud := common.CloudToParams(cloud)
		return &paramsCloud, nil
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

	cloud, err := api.backend.Cloud(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := params.CloudInfo{
		CloudDetails: cloudToParams(cloud),
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

		if err != nil {
			return nil, errors.Trace(err)
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
func (api *CloudAPI) DefaultCloud() (params.StringResult, error) {
	controllerModel, err := api.ctlrBackend.Model()
	if err != nil {
		return params.StringResult{}, err
	}
	return params.StringResult{
		Result: names.NewCloudTag(controllerModel.Cloud()).String(),
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

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.
func (api *CloudAPI) UpdateCredentialsCheckModels(args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
	return api.commonUpdateCredentials(args)
}

func (api *CloudAPI) commonUpdateCredentials(args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
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

		credentialModels, err := api.backend.CredentialModels(tag)
		if err != nil && !errors.IsNotFound(err) {
			// Could not determine if credential has models - do not continue updating this credential...
			results[i].Error = common.ServerError(err)
			continue
		}

		var modelsErred bool
		if len(credentialModels) > 0 {
			// since we get a map here, for consistency ensure that models are added
			// sorted by model uuid.
			var uuids []string
			for uuid := range credentialModels {
				uuids = append(uuids, uuid)
			}
			naturalsort.Sort(uuids)
			var models []params.UpdateCredentialModelResult
			for _, uuid := range uuids {
				model := params.UpdateCredentialModelResult{ModelUUID: uuid, ModelName: credentialModels[uuid]}
				model.Errors = api.validateCredentialForModel(uuid, tag, &in)
				models = append(models, model)
				if len(model.Errors) > 0 {
					modelsErred = true
				}
			}
			results[i].Models = models
		}

		if modelsErred {
			// Some models that use this credential do not like the new content, do not update the credential...
			results[i].Error = common.ServerError(errors.New("some models are no longer visible"))
			continue
		}

		if err := api.backend.UpdateCloudCredential(tag, in); err != nil {
			if errors.IsNotFound(err) {
				err = errors.Errorf(
					"cannot update credential %q: controller does not manage cloud %q",
					tag.Name(), tag.Cloud().Id())
			}
			results[i].Error = common.ServerError(err)
			continue
		}
	}
	return params.UpdateCredentialResults{results}, nil
}

func (api *CloudAPI) validateCredentialForModel(modelUUID string, tag names.CloudCredentialTag, credential *cloud.Credential) []params.ErrorResult {
	var result []params.ErrorResult

	modelState, err := api.pool.Get(modelUUID)
	if err != nil {
		return append(result, params.ErrorResult{common.ServerError(err)})
	}
	defer modelState.Release()

	modelErrors, err := validateNewCredentialForModelFunc(
		modelState.Model(),
		environs.New,
		api.callContext,
		tag,
		credential,
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

// UpdateCredentials updates a set of cloud credentials' content.
func (api *CloudAPIV2) UpdateCredentials(args params.TaggedCredentials) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Credentials)),
	}
	updateResults, err := api.commonUpdateCredentials(args)
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

// RevokeCredentials revokes a set of cloud credentials.
func (api *CloudAPI) RevokeCredentials(args params.Entities) (params.ErrorResults, error) {
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
		if err := api.backend.RemoveCloudCredential(tag); err != nil {
			results.Results[i].Error = common.ServerError(err)
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
			cloud, err := api.backend.Cloud(cloudName)
			if err != nil {
				return nil, err
			}
			provider, err := environs.Provider(cloud.Type)
			if err != nil {
				return nil, err
			}
			schema := provider.CredentialSchemas()
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
	err := api.backend.AddCloud(common.CloudFromParams(cloudArgs.Name, cloudArgs.Cloud), api.apiUser.Name())
	if err != nil {
		return err
	}
	return nil
}

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
func (api *CloudAPI) CredentialContents(args params.CloudCredentialArgs) (params.CredentialContentResults, error) {
	// Helper to look up and cache credential schemas for clouds.
	schemaCache := make(map[string]map[cloud.AuthType]cloud.CredentialSchema)
	credentialSchemas := func(cloudName string) (map[cloud.AuthType]cloud.CredentialSchema, error) {
		if s, ok := schemaCache[cloudName]; ok {
			return s, nil
		}
		cloud, err := api.backend.Cloud(cloudName)
		if err != nil {
			return nil, err
		}
		provider, err := environs.Provider(cloud.Type)
		if err != nil {
			return nil, err
		}
		schema := provider.CredentialSchemas()
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
