// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/credential/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
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
	cloudService       CloudService
	cloudAccessService CloudAccessService
	credentialService  CredentialService

	authorizer             facade.Authorizer
	apiUser                names.UserTag
	isAdmin                bool
	getCredentialsAuthFunc common.GetAuthFunc

	controllerTag   names.ControllerTag
	controllerCloud string

	logger corelogger.Logger
}

var (
	_ CloudV7 = (*CloudAPI)(nil)
)

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(
	ctx context.Context,
	controllerTag names.ControllerTag,
	controllerCloud string,
	cloudService CloudService,
	cloudAccessService CloudAccessService,
	credentialService CredentialService,
	authorizer facade.Authorizer, logger corelogger.Logger,
) (*CloudAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	err := authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, err
	}
	isAdmin := err == nil
	authUser, _ := authorizer.GetAuthTag().(names.UserTag)
	getUserAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
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
		cloudService:           cloudService,
		cloudAccessService:     cloudAccessService,
		credentialService:      credentialService,
		authorizer:             authorizer,
		getCredentialsAuthFunc: getUserAuthFunc,
		apiUser:                authUser,
		isAdmin:                isAdmin,
		logger:                 logger,
	}, nil
}

func (api *CloudAPI) canAccessCloud(ctx context.Context, cloud string, user user.Name, access permission.Access) (bool, error) {
	id := permission.ID{ObjectType: permission.Cloud, Key: cloud}
	perm, err := api.cloudAccessService.ReadUserAccessLevelForTarget(ctx, user, id)
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
	err = api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
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
			canAccess, err := api.canAccessCloud(ctx, aCloud.Name, user.NameFromTag(api.apiUser), permission.AddModelAccess)
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
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
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
			canAccess, err := api.canAccessCloud(ctx, tag.Id(), user.NameFromTag(api.apiUser), permission.AddModelAccess)
			if err != nil {
				return nil, err
			}
			if !canAccess {
				return nil, errors.NotFoundf("cloud %q", tag.Id())
			}
		}
		aCloud, err := api.cloudService.Cloud(ctx, tag.Id())
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
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil
	// If not a controller admin, check for cloud admin.
	if !isAdmin {
		isAdmin, err = api.canAccessCloud(ctx, tag.Id(), user.NameFromTag(api.apiUser), permission.AdminAccess)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return nil, errors.Trace(err)
		}
	}

	aCloud, err := api.cloudService.Cloud(ctx, tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := params.CloudInfo{
		CloudDetails: cloudDetailsToParams(*aCloud),
	}

	cloudUsers, err := api.cloudAccessService.ReadAllUserAccessForTarget(ctx, permission.ID{Key: tag.Id(), ObjectType: permission.Cloud})
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, perm := range cloudUsers {
		if !isAdmin && api.apiUser.Id() != perm.UserName.Name() {
			// The authenticated user is neither the controller
			// superuser, a cloud administrator, nor a cloud user, so
			// has no business knowing about the cloud user.
			continue
		}
		userInfo := params.CloudUserInfo{
			UserName:    perm.UserName.Name(),
			DisplayName: perm.DisplayName,
			Access:      string(perm.Access),
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

	cloudAccess, err := api.cloudAccessService.ReadAllAccessForUserAndObjectType(ctx, user.NameFromTag(userTag), permission.Cloud)
	if err != nil {
		return result, errors.Trace(err)
	}

	cloudsByName := make(map[string]cloud.Cloud)
	for _, cld := range allClouds {
		cloudsByName[cld.Name] = cld
	}

	for _, ca := range cloudAccess {
		cld, ok := cloudsByName[ca.UserName.Name()]
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
	authFunc, err := api.getCredentialsAuthFunc(ctx)
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
		cloudCredentials, err := api.credentialService.CloudCredentialsForOwner(ctx, user.NameFromTag(userTag), cloudTag.Id())
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

	authFunc, err := api.getCredentialsAuthFunc(ctx)
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
		if err := api.credentialService.UpdateCloudCredential(ctx, credential.KeyFromTag(tag), in); err != nil {
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
		err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
		if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return params.UpdateCredentialResults{}, errors.Trace(err)
		}
		if err != nil {
			return params.UpdateCredentialResults{}, errors.Annotatef(apiservererrors.ErrBadRequest, "unexpected force specified")
		}
	}

	authFunc, err := api.getCredentialsAuthFunc(ctx)
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
		modelResults, err := api.credentialService.CheckAndUpdateCredential(ctx, credential.KeyFromTag(tag), in, args.Force)
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
	authFunc, err := api.getCredentialsAuthFunc(ctx)
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

		if err = api.credentialService.CheckAndRevokeCredential(ctx, credential.KeyFromTag(tag), arg.Force); err != nil {
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
	authFunc, err := api.getCredentialsAuthFunc(ctx)
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
			aCloud, err := api.cloudService.Cloud(ctx, cloudName)
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
		cloudCredentials, err := api.credentialService.CloudCredentialsForOwner(ctx, user.NameFromTag(credentialTag.Owner()), credentialTag.Cloud().Id())
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
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
	if err != nil {
		return err
	}

	if cloudArgs.Cloud.Type != k8sconstants.CAASProviderType {
		// All non-k8s cloud need to go through whitelist.
		controllerCloud, err := api.cloudService.Cloud(ctx, api.controllerCloud)
		if err != nil {
			return errors.Trace(err)
		}
		if err := cloud.CurrentWhiteList().Check(controllerCloud.Type, cloudArgs.Cloud.Type); err != nil {
			if cloudArgs.Force == nil || !*cloudArgs.Force {
				return apiservererrors.ServerError(params.Error{Code: params.CodeIncompatibleClouds, Message: err.Error()})
			}
			api.logger.Infof(context.TODO(), "force adding cloud %q of type %q to controller bootstrapped on cloud type %q", cloudArgs.Name, cloudArgs.Cloud.Type, controllerCloud.Type)
		}
	}

	aCloud := cloudFromParams(cloudArgs.Name, cloudArgs.Cloud)
	// All clouds must have at least one 'default' region, lp#1819409.
	if len(aCloud.Regions) == 0 {
		aCloud.Regions = []cloud.Region{{Name: cloud.DefaultCloudRegion}}
	}

	err = api.cloudService.CreateCloud(ctx, user.NameFromTag(api.apiUser), aCloud)
	if err != nil {
		return errors.Annotatef(err, "creating cloud %q", cloudArgs.Name)
	}
	return nil
}

// UpdateCloud updates an existing cloud that the controller knows about.
func (api *CloudAPI) UpdateCloud(ctx context.Context, cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(cloudArgs.Clouds)),
	}
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	} else if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	for i, aCloud := range cloudArgs.Clouds {
		err := api.cloudService.UpdateCloud(ctx, cloudFromParams(aCloud.Name, aCloud.Cloud))
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
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, api.controllerTag)
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
			canAccess, err := api.canAccessCloud(ctx, tag.Id(), user.NameFromTag(api.apiUser), permission.AdminAccess)
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			if !canAccess {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			}
		}
		err = api.cloudService.DeleteCloud(ctx, tag.Id())
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
		aCloud, err := api.cloudService.Cloud(ctx, cloudName)
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
	toParam := func(key credential.Key, cred cloud.Credential, includeSecrets bool) params.CredentialContentResult {
		schemas, err := credentialSchemas(key.Cloud)
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
				Cloud:      key.Cloud,
			},
		}
		if includeValidity {
			valid := !cred.Invalid
			info.Content.Valid = &valid
		}

		// get model access
		models, err := api.cloudAccessService.AllModelAccessForCloudCredential(ctx, key)
		if err != nil && !errors.Is(err, accesserrors.PermissionNotFound) {
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
		}
		info.Models = make([]params.ModelAccess, len(models))
		for i, m := range models {
			info.Models[i] = params.ModelAccess{Model: m.ModelName, Access: m.OwnerAccess.String()}
		}

		return params.CredentialContentResult{Result: &info}
	}

	var result []params.CredentialContentResult
	if len(args.Credentials) == 0 {
		credentials, err := api.credentialService.AllCloudCredentialsForOwner(ctx, user.NameFromTag(api.apiUser))
		if err != nil {
			return params.CredentialContentResults{}, errors.Trace(err)
		}
		for key, cred := range credentials {
			result = append(result, toParam(key, cred, args.IncludeSecrets))
		}
	} else {
		// Helper to construct credential ID from cloud and name.
		credKey := func(cloudName, credentialName string) credential.Key {
			return credential.Key{
				Cloud: cloudName, Owner: user.NameFromTag(api.apiUser), Name: credentialName}
		}

		result = make([]params.CredentialContentResult, len(args.Credentials))
		for i, given := range args.Credentials {
			key := credKey(given.CloudName, given.CredentialName)
			if err := key.Validate(); err != nil {
				result[i] = params.CredentialContentResult{
					Error: apiservererrors.ServerError(errors.NotValidf("cloud credential ID %q", key)),
				}
				continue
			}
			cred, err := api.credentialService.CloudCredential(ctx, key)
			if err != nil {
				result[i] = params.CredentialContentResult{
					Error: apiservererrors.ServerError(err),
				}
				continue
			}
			result[i] = toParam(key, cred, args.IncludeSecrets)
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
		if !api.isAdmin {
			err := api.authorizer.HasPermission(ctx, permission.AdminAccess, cloudTag)
			if errors.Is(err, authentication.ErrorEntityMissingPermission) {
				result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
				continue
			} else if err != nil {
				return result, errors.Trace(err)
			}
		}
		if api.apiUser.String() == arg.UserTag {
			result.Results[i].Error = apiservererrors.ServerError(errors.New("cannot change your own cloud access"))
			continue
		}
		userTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		updateArgs := access.UpdatePermissionArgs{
			AccessSpec: permission.AccessSpec{
				Target: permission.ID{
					ObjectType: permission.Cloud,
					Key:        cloudTag.Id(),
				},
				Access: permission.Access(arg.Access),
			},
			Change:  permission.AccessChange(arg.Action),
			Subject: user.NameFromTag(userTag),
		}
		result.Results[i].Error = apiservererrors.ServerError(
			api.cloudAccessService.UpdatePermission(ctx, updateArgs))
	}
	return result, nil
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
