// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"
	"fmt"
	coreuser "github.com/juju/juju/core/user"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	stateerrors "github.com/juju/juju/state/errors"
)

// CloudV7 defines the methods on the cloud API facade, version 7.
type CloudV7 interface {
	AddCloud(ctx context.Context, cloudArgs params.AddCloudArgs) error
	AddCredentials(ctx context.Context, args params.TaggedCredentials) (params.ErrorResults, error)
	Cloud(ctx context.Context, args params.Entities) (params.CloudResults, error)
	Clouds(ctx context.Context) (params.CloudsResult, error)
	Credential(ctx context.Context, args params.Entities) (params.CloudCredentialResults, error)
	CredentialContents(ctx context.Context, credentialArgs params.CloudCredentialArgs) (params.CredentialContentResults, error)
	ModifyCloudAccess(ctx context.Context, args params.ModifyCloudAccessRequest) (params.ErrorResults, error)
	RevokeCredentialsCheckModels(ctx context.Context, args params.RevokeCredentialArgs) (params.ErrorResults, error)
	UpdateCredentialsCheckModels(ctx context.Context, args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error)
	UserCredentials(ctx context.Context, args params.UserClouds) (params.StringsResults, error)
	UpdateCloud(ctx context.Context, cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error)
}

// CloudAPI implements the cloud interface and is the concrete implementation
// of the api end point.
type CloudAPI struct {
	userService            UserService
	modelCredentialService ModelCredentialService

	cloudService           CloudService
	cloudPermissionService CloudPermissionService
	credentialService      CredentialService

	authorizer             facade.Authorizer
	apiUser                names.UserTag
	isAdmin                bool
	getCredentialsAuthFunc common.GetAuthFunc

	controllerTag   names.ControllerTag
	controllerCloud string

	logger loggo.Logger
}

var (
	_ CloudV7 = (*CloudAPI)(nil)
)

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(
	controllerTag names.ControllerTag,
	controllerCloud string,
	userService UserService,
	modelCredentialService ModelCredentialService,
	cloudService CloudService,
	cloudPermissionService CloudPermissionService,
	credentialService CredentialService,
	authorizer facade.Authorizer, logger loggo.Logger,
) (*CloudAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	err := authorizer.HasPermission(permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, err
	}
	isAdmin := err == nil
	authUser, _ := authorizer.GetAuthTag().(names.UserTag)
	getUserAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			userTag, ok := tag.(names.UserTag)
			if !ok {
				return false
			}
			return isAdmin || userTag == authUser
		}, nil
	}
	return &CloudAPI{
		controllerTag:          controllerTag,
		controllerCloud:        controllerCloud,
		userService:            userService,
		modelCredentialService: modelCredentialService,
		cloudService:           cloudService,
		cloudPermissionService: cloudPermissionService,
		credentialService:      credentialService,
		authorizer:             authorizer,
		getCredentialsAuthFunc: getUserAuthFunc,
		apiUser:                authUser,
		isAdmin:                isAdmin,
		logger:                 logger,
	}, nil
}

func (api *CloudAPI) canAccessCloud(cloud string, user names.UserTag, access permission.Access) (bool, error) {
	perm, err := api.cloudPermissionService.GetCloudAccess(cloud, user)
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.Trace(err)
	}
	return perm.EqualOrGreaterCloudAccessThan(access), nil
}

// Clouds returns the definitions of all clouds supported by the controller
// that the logged in user can see.
func (api *CloudAPI) Clouds(ctx context.Context) (params.CloudsResult, error) {
	var result params.CloudsResult
	clouds, err := api.cloudService.ListAll(ctx)
	if err != nil {
		return result, err
	}
	err = api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil &&
		!errors.Is(err, authentication.ErrorEntityMissingPermission) &&
		!errors.Is(err, errors.NotFound) {
		return result, errors.Trace(err)
	}
	isAdmin := err == nil
	result.Clouds = make(map[string]params.Cloud)
	for _, aCloud := range clouds {
		// Ensure user has permission to see the cloud.
		if !isAdmin {
			canAccess, err := api.canAccessCloud(aCloud.Name, api.apiUser, permission.AddModelAccess)
			if err != nil {
				return result, err
			}
			if !canAccess {
				continue
			}
		}
		paramsCloud := cloudToParams(aCloud)
		result.Clouds[names.NewCloudTag(aCloud.Name).String()] = paramsCloud
	}
	return result, nil
}

// Cloud returns the cloud definitions for the specified clouds.
func (api *CloudAPI) Cloud(ctx context.Context, args params.Entities) (params.CloudResults, error) {
	results := params.CloudResults{
		Results: make([]params.CloudResult, len(args.Entities)),
	}
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil &&
		!errors.Is(err, authentication.ErrorEntityMissingPermission) &&
		!errors.Is(err, errors.NotFound) {
		return results, errors.Trace(err)
	}
	isAdmin := err == nil
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
		aCloud, err := api.cloudService.Get(ctx, tag.Id())
		if err != nil {
			return nil, err
		}
		paramsCloud := cloudToParams(*aCloud)
		return &paramsCloud, nil
	}
	for i, arg := range args.Entities {
		aCloud, err := one(arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			results.Results[i].Cloud = aCloud
		}
	}
	return results, nil
}

// CloudInfo returns information about the specified clouds.
func (api *CloudAPI) CloudInfo(ctx context.Context, args params.Entities) (params.CloudInfoResults, error) {
	results := params.CloudInfoResults{
		Results: make([]params.CloudInfoResult, len(args.Entities)),
	}

	oneCloudInfo := func(arg params.Entity) (*params.CloudInfo, error) {
		tag, err := names.ParseCloudTag(arg.Tag)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return api.getCloudInfo(ctx, tag)
	}

	for i, arg := range args.Entities {
		cloudInfo, err := oneCloudInfo(arg)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = cloudInfo
	}
	return results, nil
}

func (api *CloudAPI) getCloudInfo(ctx context.Context, tag names.CloudTag) (*params.CloudInfo, error) {
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil
	// If not a controller admin, check for cloud admin.
	if !isAdmin {
		perm, err := api.cloudPermissionService.GetCloudAccess(tag.Id(), api.apiUser)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
		isAdmin = perm == permission.AdminAccess
	}

	aCloud, err := api.cloudService.Get(ctx, tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := params.CloudInfo{
		CloudDetails: cloudDetailsToParams(*aCloud),
	}

	// TODO(wallyworld) - refactor once permissions are on dqlite.
	cloudUsers, err := api.cloudPermissionService.GetCloudUsers(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	for userId, perm := range cloudUsers {
		if !isAdmin && api.apiUser.Id() != userId {
			// The authenticated user is neither the controller
			// superuser, a cloud administrator, nor a cloud user, so
			// has no business knowing about the cloud user.
			continue
		}
		userTag := names.NewUserTag(userId)
		displayName := userId
		if userTag.IsLocal() {
			u, err := api.userService.GetUserByName(ctx, userTag.Name())
			if err != nil {
				if !stateerrors.IsDeletedUserError(err) {
					// We ignore deleted users for now. So if it is not a
					// DeletedUserError we return the error.
					return nil, errors.Trace(err)
				}
				continue
			}
			displayName = u.DisplayName
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
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}
	return &info, nil
}

// ListCloudInfo returns clouds that the specified user has access to.
// Controller admins (superuser) can list clouds for any user.
// Other users can only ask about their own clouds.
func (api *CloudAPI) ListCloudInfo(ctx context.Context, req params.ListCloudsRequest) (params.ListCloudInfoResults, error) {
	result := params.ListCloudInfoResults{}

	userTag, err := names.ParseUserTag(req.UserTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	allClouds, err := api.cloudService.ListAll(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	if req.All && api.isAdmin {
		for _, cld := range allClouds {
			info := &params.ListCloudInfo{
				CloudDetails: cloudDetailsToParams(cld),
				Access:       string(permission.AdminAccess),
			}
			result.Results = append(result.Results, params.ListCloudInfoResult{Result: info})
		}
		return result, nil
	}

	// TODO(wallyworld) - refactor once permissions are on dqlite.
	cloudAccess, err := api.cloudPermissionService.CloudsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	cloudsByName := make(map[string]cloud.Cloud)
	for _, cld := range allClouds {
		cloudsByName[cld.Name] = cld
	}

	for _, ca := range cloudAccess {
		cld, ok := cloudsByName[ca.Name]
		if !ok {
			continue
		}
		info := &params.ListCloudInfo{
			CloudDetails: cloudDetailsToParams(cld),
			Access:       string(ca.Access),
		}
		result.Results = append(result.Results, params.ListCloudInfoResult{Result: info})
	}
	return result, nil
}

// UserCredentials returns the cloud credentials for a set of users.
func (api *CloudAPI) UserCredentials(ctx context.Context, args params.UserClouds) (params.StringsResults, error) {
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !authFunc(userTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		cloudTag, err := names.ParseCloudTag(arg.CloudTag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		cloudCredentials, err := api.credentialService.CloudCredentialsForOwner(ctx, userTag.Id(), cloudTag.Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		out := make([]string, 0, len(cloudCredentials))
		for tagId := range cloudCredentials {
			if !names.IsValidCloudCredential(tagId) {
				results.Results[i].Error = apiservererrors.ServerError(errors.NotValidf("cloud credential ID %q", tagId))
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
func (api *CloudAPI) AddCredentials(ctx context.Context, args params.TaggedCredentials) (params.ErrorResults, error) {
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		in := cloud.NewCredential(
			cloud.AuthType(arg.Credential.AuthType),
			arg.Credential.Attributes,
		)
		if err := api.credentialService.UpdateCloudCredential(ctx, credential.IdFromTag(tag), in); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}
	return results, nil
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.  Such model errors are returned
// separately and do not contribute to the overall method error status.
// Controller admins can 'force' an update of the credential
// regardless of whether it is deemed valid or not.
func (api *CloudAPI) UpdateCredentialsCheckModels(ctx context.Context, args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error) {
	if args.Force {
		// Only controller admins can ask for an update to be forced.
		err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
		if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return params.UpdateCredentialResults{}, errors.Trace(err)
		}
		if err != nil {
			return params.UpdateCredentialResults{}, errors.Annotatef(apiservererrors.ErrBadRequest, "unexpected force specified")
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
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		in := cloud.NewCredential(
			cloud.AuthType(arg.Credential.AuthType),
			arg.Credential.Attributes,
		)
		modelResults, err := api.credentialService.CheckAndUpdateCredential(ctx, credential.IdFromTag(tag), in, args.Force)
		results[i].Models = modelResultsToParams(modelResults)
		if err != nil {
			if !args.Force {
				results[i].Error = apiservererrors.ServerError(err)
			}
			continue
		}
	}
	return params.UpdateCredentialResults{Results: results}, nil
}

func modelResultsToParams(modelResults []service.UpdateCredentialModelResult) []params.UpdateCredentialModelResult {
	result := make([]params.UpdateCredentialModelResult, len(modelResults))
	for i, modelResult := range modelResults {
		resultParams := params.UpdateCredentialModelResult{
			ModelUUID: string(modelResult.ModelUUID),
			ModelName: modelResult.ModelName,
		}
		resultParams.Errors = make([]params.ErrorResult, len(modelResult.Errors))
		for j, resultErr := range modelResult.Errors {
			resultParams.Errors[j].Error = apiservererrors.ServerError(resultErr)
		}
		result[i] = resultParams
	}
	return result
}

// RevokeCredentialsCheckModels revokes a set of cloud credentials.
// If the credentials are used by any of the models, the credential deletion will be aborted.
// If credential-in-use needs to be revoked nonetheless, this method allows the use of force.
func (api *CloudAPI) RevokeCredentialsCheckModels(ctx context.Context, args params.RevokeCredentialArgs) (params.ErrorResults, error) {
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if err = api.credentialService.CheckAndRevokeCredential(ctx, credential.IdFromTag(tag), arg.Force); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		}
	}
	return results, nil
}

// Credential returns the specified cloud credential for each tag, minus secrets.
func (api *CloudAPI) Credential(ctx context.Context, args params.Entities) (params.CloudCredentialResults, error) {
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !authFunc(credentialTag.Owner()) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		// Helper to look up and cache credential schemas for clouds.
		schemaCache := make(map[string]map[cloud.AuthType]cloud.CredentialSchema)
		credentialSchemas := func() (map[cloud.AuthType]cloud.CredentialSchema, error) {
			cloudName := credentialTag.Cloud().Id()
			if s, ok := schemaCache[cloudName]; ok {
				return s, nil
			}
			aCloud, err := api.cloudService.Get(ctx, cloudName)
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
		cloudCredentials, err := api.credentialService.CloudCredentialsForOwner(ctx, credentialTag.Owner().Id(), credentialTag.Cloud().Id())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		cred, ok := cloudCredentials[credentialTag.Id()]
		if !ok {
			results.Results[i].Error = apiservererrors.ServerError(errors.NotFoundf("credential %q", credentialTag.Name()))
			continue
		}

		schemas, err := credentialSchemas()
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		attrs := cred.Attributes()
		var redacted []string
		// Mask out the secrets.
		if s, ok := schemas[cred.AuthType()]; ok {
			for _, attr := range s {
				if attr.Hidden {
					delete(attrs, attr.Name)
					redacted = append(redacted, attr.Name)
				}
			}
		}
		results.Results[i].Result = &params.CloudCredential{
			AuthType:   string(cred.AuthType()),
			Attributes: attrs,
			Redacted:   redacted,
		}
	}
	return results, nil
}

// AddCloud adds a new cloud, different from the one managed by the controller.
func (api *CloudAPI) AddCloud(ctx context.Context, cloudArgs params.AddCloudArgs) error {
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil {
		return err
	}

	if cloudArgs.Cloud.Type != k8sconstants.CAASProviderType {
		// All non-k8s cloud need to go through whitelist.
		controllerCloud, err := api.cloudService.Get(ctx, api.controllerCloud)
		if err != nil {
			return errors.Trace(err)
		}
		if err := cloud.CurrentWhiteList().Check(controllerCloud.Type, cloudArgs.Cloud.Type); err != nil {
			if cloudArgs.Force == nil || !*cloudArgs.Force {
				return apiservererrors.ServerError(params.Error{Code: params.CodeIncompatibleClouds, Message: err.Error()})
			}
			api.logger.Infof("force adding cloud %q of type %q to controller bootstrapped on cloud type %q", cloudArgs.Name, cloudArgs.Cloud.Type, controllerCloud.Type)
		}
	}

	aCloud := cloudFromParams(cloudArgs.Name, cloudArgs.Cloud)
	// All clouds must have at least one 'default' region, lp#1819409.
	if len(aCloud.Regions) == 0 {
		aCloud.Regions = []cloud.Region{{Name: cloud.DefaultCloudRegion}}
	}

	usr, err := api.userService.GetUserByName(ctx, api.apiUser.Name())
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(wallyworld) - refactor once permissions are on dqlite.
	err = api.cloudService.Save(ctx, aCloud)
	if err != nil {
		return errors.Annotatef(err, "creating cloud %q", cloudArgs.Name)
	}
	err = api.cloudPermissionService.CreateCloudAccess(usr, cloudArgs.Name, api.apiUser, permission.AdminAccess)
	return errors.Trace(err)
}

// UpdateCloud updates an existing cloud that the controller knows about.
func (api *CloudAPI) UpdateCloud(ctx context.Context, cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(cloudArgs.Clouds)),
	}
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	} else if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	for i, aCloud := range cloudArgs.Clouds {
		err := api.cloudService.Save(ctx, cloudFromParams(aCloud.Name, aCloud.Cloud))
		results.Results[i].Error = apiservererrors.ServerError(err)
	}
	return results, nil
}

// RemoveClouds removes the specified clouds from the controller.
// If a cloud is in use (has models deployed to it), the removal will fail.
func (api *CloudAPI) RemoveClouds(ctx context.Context, args params.Entities) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}
	isAdmin := err == nil
	for i, entity := range args.Entities {
		tag, err := names.ParseCloudTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// Ensure user has permission to remove the cloud.
		if !isAdmin {
			canAccess, err := api.canAccessCloud(tag.Id(), api.apiUser, permission.AdminAccess)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			if !canAccess {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			}
		}
		err = api.cloudService.Delete(ctx, tag.Id())
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// CredentialContents returns the specified cloud credentials,
// including the secrets if requested.
// If no specific credential name/cloud was passed in, all credentials for this user
// are returned.
// Only credential owner can see its contents as well as what models use it.
// Controller admin has no special superpowers here and is treated the same as all other users.
func (api *CloudAPI) CredentialContents(ctx context.Context, args params.CloudCredentialArgs) (params.CredentialContentResults, error) {
	return api.internalCredentialContents(ctx, args, true)
}

func (api *CloudAPI) internalCredentialContents(ctx context.Context, args params.CloudCredentialArgs, includeValidity bool) (params.CredentialContentResults, error) {
	// Helper to look up and cache credential schemas for clouds.
	schemaCache := make(map[string]map[cloud.AuthType]cloud.CredentialSchema)
	credentialSchemas := func(cloudName string) (map[cloud.AuthType]cloud.CredentialSchema, error) {
		if s, ok := schemaCache[cloudName]; ok {
			return s, nil
		}
		aCloud, err := api.cloudService.Get(ctx, cloudName)
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

	// Helper to parse cloud.CloudCredential into an expected result item.
	toParam := func(id credential.ID, cred cloud.Credential, includeSecrets bool) params.CredentialContentResult {
		schemas, err := credentialSchemas(id.Cloud)
		if err != nil {
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
		}
		attrs := map[string]string{}
		// Filter out the secrets.
		if s, ok := schemas[cred.AuthType()]; ok {
			for _, attr := range s {
				if value, exists := cred.Attributes()[attr.Name]; exists {
					if attr.Hidden && !includeSecrets {
						continue
					}
					attrs[attr.Name] = value
				}
			}
		}
		info := params.ControllerCredentialInfo{
			Content: params.CredentialContent{
				Name:       cred.Label,
				AuthType:   string(cred.AuthType()),
				Attributes: attrs,
				Cloud:      id.Cloud,
			},
		}
		if includeValidity {
			valid := !cred.Invalid
			info.Content.Valid = &valid
		}

		usr, err := api.userService.GetUserByName(ctx, api.apiUser.Name())
		if err != nil {
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
		}

		// get models
		credTag := names.NewCloudCredentialTag(fmt.Sprintf("%s/%s/%s", id.Cloud, id.Owner, id.Name))
		models, err := api.modelCredentialService.CredentialModelsAndOwnerAccess(usr, credTag)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
		}
		info.Models = make([]params.ModelAccess, len(models))
		for i, m := range models {
			info.Models[i] = params.ModelAccess{Model: m.ModelName, Access: string(m.OwnerAccess)}
		}

		return params.CredentialContentResult{Result: &info}
	}

	var result []params.CredentialContentResult
	if len(args.Credentials) == 0 {
		credentials, err := api.credentialService.AllCloudCredentialsForOwner(ctx, api.apiUser.Id())
		if err != nil {
			return params.CredentialContentResults{}, errors.Trace(err)
		}
		for id, cred := range credentials {
			result = append(result, toParam(id, cred, args.IncludeSecrets))
		}
	} else {
		// Helper to construct credential ID from cloud and name.
		credId := func(cloudName, credentialName string) credential.ID {
			return credential.ID{
				Cloud: cloudName, Owner: api.apiUser.Id(), Name: credentialName}
		}

		result = make([]params.CredentialContentResult, len(args.Credentials))
		for i, given := range args.Credentials {
			id := credId(given.CloudName, given.CredentialName)
			if err := id.Validate(); err != nil {
				result[i] = params.CredentialContentResult{
					Error: apiservererrors.ServerError(errors.NotValidf("cloud credential ID %q", id)),
				}
				continue
			}
			cred, err := api.credentialService.CloudCredential(ctx, id)
			if err != nil {
				result[i] = params.CredentialContentResult{
					Error: apiservererrors.ServerError(err),
				}
				continue
			}
			result[i] = toParam(id, cred, args.IncludeSecrets)
		}
	}
	return params.CredentialContentResults{Results: result}, nil
}

// ModifyCloudAccess changes the model access granted to users.
func (api *CloudAPI) ModifyCloudAccess(ctx context.Context, args params.ModifyCloudAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		cloudTag, err := names.ParseCloudTag(arg.CloudTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		_, err = api.cloudService.Get(ctx, cloudTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if api.apiUser.String() == arg.UserTag {
			result.Results[i].Error = apiservererrors.ServerError(errors.New("cannot change your own cloud access"))
			continue
		}

		usr, err := api.userService.GetUserByName(ctx, api.apiUser.Name())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		err = api.authorizer.HasPermission(permission.SuperuserAccess, api.controllerTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err != nil {
			callerAccess, err := api.cloudPermissionService.GetCloudAccess(cloudTag.Id(), api.apiUser)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			if callerAccess != permission.AdminAccess {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			}
		}

		cloudAccess := permission.Access(arg.Access)
		if err := permission.ValidateCloudAccess(cloudAccess); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Annotate(err, "could not modify cloud access"))
			continue
		}

		result.Results[i].Error = apiservererrors.ServerError(
			ChangeCloudAccess(usr, api.cloudPermissionService, cloudTag.Id(), targetUserTag, arg.Action, cloudAccess))
	}
	return result, nil
}

// ChangeCloudAccess performs the requested access grant or revoke action for the
// specified user on the cloud.
func ChangeCloudAccess(usr coreuser.User, backend CloudPermissionService, cloud string, targetUserTag names.UserTag, action params.CloudAction, access permission.Access) error {
	switch action {
	case params.GrantCloudAccess:
		err := grantCloudAccess(usr, backend, cloud, targetUserTag, access)
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

func grantCloudAccess(usr coreuser.User, backend CloudPermissionService, cloud string, targetUserTag names.UserTag, access permission.Access) error {
	err := backend.CreateCloudAccess(usr, cloud, targetUserTag, access)
	if errors.Is(err, errors.AlreadyExists) {
		cloudAccess, err := backend.GetCloudAccess(cloud, targetUserTag)
		if errors.Is(err, errors.NotFound) {
			// Conflicts with prior check, must be inconsistent state.
			err = jujutxn.ErrExcessiveContention
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

func revokeCloudAccess(backend CloudPermissionService, cloud string, targetUserTag names.UserTag, access permission.Access) error {
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

func cloudFromParams(cloudName string, p params.Cloud) cloud.Cloud {
	authTypes := make([]cloud.AuthType, len(p.AuthTypes))
	for i, authType := range p.AuthTypes {
		authTypes[i] = cloud.AuthType(authType)
	}
	regions := make([]cloud.Region, len(p.Regions))
	for i, region := range p.Regions {
		regions[i] = cloud.Region{
			Name:             region.Name,
			Endpoint:         region.Endpoint,
			IdentityEndpoint: region.IdentityEndpoint,
			StorageEndpoint:  region.StorageEndpoint,
		}
	}
	var regionConfig map[string]cloud.Attrs
	for r, attr := range p.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]cloud.Attrs)
		}
		regionConfig[r] = attr
	}
	return cloud.Cloud{
		Name:              cloudName,
		Type:              p.Type,
		AuthTypes:         authTypes,
		Endpoint:          p.Endpoint,
		IdentityEndpoint:  p.IdentityEndpoint,
		StorageEndpoint:   p.StorageEndpoint,
		Regions:           regions,
		CACertificates:    p.CACertificates,
		SkipTLSVerify:     p.SkipTLSVerify,
		Config:            p.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: p.IsControllerCloud,
	}
}

func cloudToParams(cloud cloud.Cloud) params.Cloud {
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
	var regionConfig map[string]map[string]interface{}
	for r, attr := range cloud.RegionConfig {
		if regionConfig == nil {
			regionConfig = make(map[string]map[string]interface{})
		}
		regionConfig[r] = attr
	}
	return params.Cloud{
		Type:              cloud.Type,
		HostCloudRegion:   cloud.HostCloudRegion,
		AuthTypes:         authTypes,
		Endpoint:          cloud.Endpoint,
		IdentityEndpoint:  cloud.IdentityEndpoint,
		StorageEndpoint:   cloud.StorageEndpoint,
		Regions:           regions,
		CACertificates:    cloud.CACertificates,
		SkipTLSVerify:     cloud.SkipTLSVerify,
		Config:            cloud.Config,
		RegionConfig:      regionConfig,
		IsControllerCloud: cloud.IsControllerCloud,
	}
}

func cloudDetailsToParams(cloud cloud.Cloud) params.CloudDetails {
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
