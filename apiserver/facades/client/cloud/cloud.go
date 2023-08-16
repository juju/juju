// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/credentialcommon"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	stateerrors "github.com/juju/juju/state/errors"
)

// CloudV7 defines the methods on the cloud API facade, version 7.
type CloudV7 interface {
	AddCloud(ctx context.Context, cloudArgs params.AddCloudArgs) error
	AddCredentials(ctx context.Context, args params.TaggedCredentials) (params.ErrorResults, error)
	CheckCredentialsModels(ctx context.Context, args params.TaggedCredentials) (params.UpdateCredentialResults, error)
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
	backend                Backend
	ctlrBackend            Backend
	authorizer             facade.Authorizer
	apiUser                names.UserTag
	isAdmin                bool
	getCredentialsAuthFunc common.GetAuthFunc
	pool                   ModelPoolBackend
	logger                 loggo.Logger
}

var (
	_ CloudV7 = (*CloudAPI)(nil)
)

// NewCloudAPI creates a new API server endpoint for managing the controller's
// cloud definition and cloud credentials.
func NewCloudAPI(backend, ctlrBackend Backend, pool ModelPoolBackend, authorizer facade.Authorizer, logger loggo.Logger) (*CloudAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	err := authorizer.HasPermission(permission.SuperuserAccess, backend.ControllerTag())
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
		backend:                backend,
		ctlrBackend:            ctlrBackend,
		authorizer:             authorizer,
		getCredentialsAuthFunc: getUserAuthFunc,
		apiUser:                authUser,
		isAdmin:                isAdmin,
		pool:                   pool,
		logger:                 logger,
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
func (api *CloudAPI) Clouds(ctx context.Context) (params.CloudsResult, error) {
	var result params.CloudsResult
	clouds, err := api.backend.Clouds()
	if err != nil {
		return result, err
	}
	err = api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil &&
		!errors.Is(err, authentication.ErrorEntityMissingPermission) &&
		!errors.Is(err, errors.NotFound) {
		return result, errors.Trace(err)
	}
	isAdmin := err == nil
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
		paramsCloud := cloudToParams(aCloud)
		result.Clouds[tag.String()] = paramsCloud
	}
	return result, nil
}

// Cloud returns the cloud definitions for the specified clouds.
func (api *CloudAPI) Cloud(ctx context.Context, args params.Entities) (params.CloudResults, error) {
	results := params.CloudResults{
		Results: make([]params.CloudResult, len(args.Entities)),
	}
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
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
		aCloud, err := api.backend.Cloud(tag.Id())
		if err != nil {
			return nil, err
		}
		paramsCloud := cloudToParams(aCloud)
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
		return api.getCloudInfo(tag)
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

func (api *CloudAPI) getCloudInfo(tag names.CloudTag) (*params.CloudInfo, error) {
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, errors.Trace(err)
	}
	isAdmin := err == nil
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
		CloudDetails: cloudDetailsToParams(aCloud),
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
				if !stateerrors.IsDeletedUserError(err) {
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

	cloudInfos, err := api.ctlrBackend.CloudsForUser(userTag, req.All && api.isAdmin)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, ci := range cloudInfos {
		info := &params.ListCloudInfo{
			CloudDetails: cloudDetailsToParams(ci.Cloud),
			Access:       string(ci.Access),
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
		cloudCredentials, err := api.backend.CloudCredentials(userTag, cloudTag.Id())
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
		if err := api.backend.UpdateCloudCredential(tag, in); err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
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
// There's no Juju API client which uses this, but JAAS does,
func (api *CloudAPI) CheckCredentialsModels(ctx context.Context, args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
	return api.commonUpdateCredentials(false, false, true, args)
}

// UpdateCredentialsCheckModels updates a set of cloud credentials' content.
// If there are any models that are using a credential and these models
// are not going to be visible with updated credential content,
// there will be detailed validation errors per model.  Such model errors are returned
// separately and do not contribute to the overall method error status.
// Controller admins can 'force' an update of the credential
// regardless of whether it is deemed valid or not.
func (api *CloudAPI) UpdateCredentialsCheckModels(ctx context.Context, args params.UpdateCredentialArgs) (params.UpdateCredentialResults, error) {
	return api.commonUpdateCredentials(true, args.Force, false, params.TaggedCredentials{Credentials: args.Credentials})
}

func (api *CloudAPI) commonUpdateCredentials(update bool, force, legacy bool, args params.TaggedCredentials) (params.UpdateCredentialResults, error) {
	if force {
		// Only controller admins can ask for an update to be forced.
		err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
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

		models, err := api.credentialModels(tag)
		if err != nil {
			if legacy || !force {
				results[i].Error = apiservererrors.ServerError(err)
			}
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
			if legacy {
				results[i].Error = apiservererrors.ServerError(errors.New("some models are no longer visible"))
			}
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
				results[i].Error = apiservererrors.ServerError(err)
			}
		}
	}
	return params.UpdateCredentialResults{Results: results}, nil
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
		return append(result, params.ErrorResult{Error: apiservererrors.ServerError(err)})
	}

	modelErrors, err := validateNewCredentialForModelFunc(
		m,
		callContext,
		tag,
		credential,
		false,
	)
	if err != nil {
		return append(result, params.ErrorResult{Error: apiservererrors.ServerError(err)})
	}
	if len(modelErrors.Results) > 0 {
		return append(result, modelErrors.Results...)
	}
	return result
}

var validateNewCredentialForModelFunc = credentialcommon.ValidateNewModelCredential

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
func (api *CloudAPI) RevokeCredentialsCheckModels(ctx context.Context, args params.RevokeCredentialArgs) (params.ErrorResults, error) {
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
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		// NOTE(axw) if we add ACLs for cloud credentials, we'll need
		// to change this auth check.
		if !authFunc(tag.Owner()) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		models, err := api.credentialModels(tag)
		if err != nil {
			if !arg.Force {
				// Could not determine if credential has models - do not continue revoking this credential...
				results.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			api.logger.Warningf("could not get models that use credential %v: %v", tag, err)
		}
		if len(models) != 0 {
			api.logger.Warningf("credential %v %v it is used by model%v",
				tag,
				opMessage(arg.Force),
				modelsPretty(models),
			)
			if !arg.Force {
				// Some models still use this credential - do not delete this credential...
				results.Results[i].Error = apiservererrors.ServerError(errors.Errorf("cannot revoke credential %v: it is still used by %d model%v", tag, len(models), plural(len(models))))
				continue
			}
		}
		err = api.backend.RemoveCloudCredential(tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
		} else {
			// If credential was successfully removed, we also want to clear all references to it from the models.
			// lp#1841885
			if err := api.backend.RemoveModelsCredential(tag); err != nil {
				results.Results[i].Error = apiservererrors.ServerError(err)
			}
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
func (api *CloudAPI) AddCloud(ctx context.Context, cloudArgs params.AddCloudArgs) error {
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil {
		return err
	}

	if cloudArgs.Cloud.Type != k8sconstants.CAASProviderType {
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

	err = api.backend.AddCloud(aCloud, api.apiUser.Name())
	return errors.Trace(err)
}

// UpdateCloud updates an existing cloud that the controller knows about.
func (api *CloudAPI) UpdateCloud(ctx context.Context, cloudArgs params.UpdateCloudArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(cloudArgs.Clouds)),
	}
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
	if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	} else if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	for i, aCloud := range cloudArgs.Clouds {
		err := api.backend.UpdateCloud(cloudFromParams(aCloud.Name, aCloud.Cloud))
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
	err := api.authorizer.HasPermission(permission.SuperuserAccess, api.ctlrBackend.ControllerTag())
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
		err = api.backend.RemoveCloud(tag.Id())
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
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
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
			return params.CredentialContentResult{Error: apiservererrors.ServerError(err)}
		}

		models, err := api.backend.CredentialModelsAndOwnerAccess(tag)
		if err != nil && !errors.IsNotFound(err) {
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
					Error: apiservererrors.ServerError(errors.NotValidf("cloud credential ID %q", id)),
				}
				continue
			}
			tag := names.NewCloudCredentialTag(id)
			credential, err := api.backend.CloudCredential(tag)
			if err != nil {
				result[i] = params.CredentialContentResult{
					Error: apiservererrors.ServerError(err),
				}
				continue
			}
			result[i] = stateIntoParam(credential, args.IncludeSecrets)
		}
	}
	return params.CredentialContentResults{Results: result}, nil
}

// ModifyCloudAccess changes the model access granted to users.
func (c *CloudAPI) ModifyCloudAccess(ctx context.Context, args params.ModifyCloudAccessRequest) (params.ErrorResults, error) {
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
		_, err = c.backend.Cloud(cloudTag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if c.apiUser.String() == arg.UserTag {
			result.Results[i].Error = apiservererrors.ServerError(errors.New("cannot change your own cloud access"))
			continue
		}

		err = c.authorizer.HasPermission(permission.SuperuserAccess, c.backend.ControllerTag())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if err != nil {
			callerAccess, err := c.backend.GetCloudAccess(cloudTag.Id(), c.apiUser)
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
