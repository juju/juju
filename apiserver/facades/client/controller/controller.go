// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/description/v10"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/usermanager"
	controllerclient "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corecontroller "github.com/juju/juju/controller"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremigration "github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/blockcommand"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/docker"
	interrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/rpc/params"
)

// ModelExporter exports a model to a description.Model.
type ModelExporter interface {
	// ExportModel exports a model to a description.Model.
	// It requires a known set of leaders to be passed in, so that applications
	// can have their leader set correctly once imported.
	// The objectstore is used to retrieve charms and resources for export.
	ExportModel(context.Context, objectstore.ObjectStore) (description.Model, error)
}

// ControllerAPIV12 implements the controller APIV12.
type ControllerAPIV12 struct {
	*ControllerAPI
}

// ControllerAPI provides the Controller API.
type ControllerAPI struct {
	*common.ControllerConfigAPI
	*commonmodel.ModelStatusAPI

	authorizer                  facade.Authorizer
	apiUser                     names.UserTag
	controllerConfigService     ControllerConfigService
	accessService               ControllerAccessService
	modelService                ModelService
	modelInfoService            ModelInfoService
	blockCommandService         BlockCommandService
	modelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error)
	credentialServiceGetter     func(context.Context, coremodel.UUID) (CredentialService, error)
	upgradeServiceGetter        func(context.Context, coremodel.UUID) (UpgradeService, error)
	applicationServiceGetter    func(context.Context, coremodel.UUID) (ApplicationService, error)
	relationServiceGetter       func(context.Context, coremodel.UUID) (RelationService, error)
	statusServiceGetter         func(context.Context, coremodel.UUID) (StatusService, error)
	modelAgentServiceGetter     func(context.Context, coremodel.UUID) (ModelAgentService, error)
	modelConfigServiceGetter    func(context.Context, coremodel.UUID) (ModelConfigService, error)
	blockCommandServiceGetter   func(context.Context, coremodel.UUID) (BlockCommandService, error)
	cloudSpecServiceGetter      func(context.Context, coremodel.UUID) (ModelProviderService, error)
	machineServiceGetter        func(context.Context, coremodel.UUID) (MachineService, error)
	removalServiceGetter        func(context.Context, coremodel.UUID) (RemovalService, error)
	proxyService                ProxyService
	modelExporter               func(context.Context, coremodel.UUID) (ModelExporter, error)
	store                       objectstore.ObjectStore
	logger                      corelogger.Logger
	controllerModelUUID         coremodel.UUID
	controllerUUID              string
}

// LatestAPI is used for testing purposes to create the latest
// controller API.
var LatestAPI = makeControllerAPI

// NewControllerAPI creates a new api server endpoint for operations
// on a controller.
func NewControllerAPI(
	ctx context.Context,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	controllerConfigService ControllerConfigService,
	controllerNodeService ControllerNodeService,
	externalControllerService common.ExternalControllerService,
	accessService ControllerAccessService,
	modelService ModelService,
	modelInfoService ModelInfoService,
	blockCommandService BlockCommandService,
	modelMigrationServiceGetter func(context.Context, coremodel.UUID) (ModelMigrationService, error),
	credentialServiceGetter func(context.Context, coremodel.UUID) (CredentialService, error),
	upgradeServiceGetter func(context.Context, coremodel.UUID) (UpgradeService, error),
	applicationServiceGetter func(context.Context, coremodel.UUID) (ApplicationService, error),
	relationServiceGetter func(context.Context, coremodel.UUID) (RelationService, error),
	statusServiceGetter func(context.Context, coremodel.UUID) (StatusService, error),
	modelAgentServiceGetter func(context.Context, coremodel.UUID) (ModelAgentService, error),
	modelConfigServiceGetter func(context.Context, coremodel.UUID) (ModelConfigService, error),
	blockCommandServiceGetter func(context.Context, coremodel.UUID) (BlockCommandService, error),
	cloudSpecServiceGetter func(context.Context, coremodel.UUID) (ModelProviderService, error),
	machineServiceGetter func(context.Context, coremodel.UUID) (MachineService, error),
	removalServiceGetter func(context.Context, coremodel.UUID) (RemovalService, error),
	proxyService ProxyService,
	modelExporter func(context.Context, coremodel.UUID) (ModelExporter, error),
	store objectstore.ObjectStore,
	controllerModelUUID coremodel.UUID,
	controllerUUID string,
) (*ControllerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	return &ControllerAPI{
		ControllerConfigAPI: common.NewControllerConfigAPI(
			controllerConfigService,
			controllerNodeService,
			externalControllerService,
			modelService,
		),
		ModelStatusAPI: commonmodel.NewModelStatusAPI(
			controllerUUID,
			controllerModelUUID.String(),
			modelService,
			func(ctx context.Context, uuid coremodel.UUID) (commonmodel.MachineService, error) {
				return machineServiceGetter(ctx, uuid)
			},
			func(ctx context.Context, uuid coremodel.UUID) (commonmodel.StatusService, error) {
				return statusServiceGetter(ctx, uuid)
			},
			authorizer,
			apiUser,
		),
		authorizer:                  authorizer,
		apiUser:                     apiUser,
		logger:                      logger,
		controllerConfigService:     controllerConfigService,
		accessService:               accessService,
		modelService:                modelService,
		blockCommandService:         blockCommandService,
		modelInfoService:            modelInfoService,
		upgradeServiceGetter:        upgradeServiceGetter,
		applicationServiceGetter:    applicationServiceGetter,
		relationServiceGetter:       relationServiceGetter,
		statusServiceGetter:         statusServiceGetter,
		credentialServiceGetter:     credentialServiceGetter,
		modelAgentServiceGetter:     modelAgentServiceGetter,
		modelConfigServiceGetter:    modelConfigServiceGetter,
		blockCommandServiceGetter:   blockCommandServiceGetter,
		cloudSpecServiceGetter:      cloudSpecServiceGetter,
		machineServiceGetter:        machineServiceGetter,
		removalServiceGetter:        removalServiceGetter,
		modelMigrationServiceGetter: modelMigrationServiceGetter,
		proxyService:                proxyService,
		modelExporter:               modelExporter,
		store:                       store,
		controllerModelUUID:         controllerModelUUID,
		controllerUUID:              controllerUUID,
	}, nil
}

func (c *ControllerAPI) checkIsSuperUser(ctx context.Context) error {
	controllerTag := names.NewControllerTag(c.controllerUUID)
	return c.authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
}

// ControllerVersion returns the version information associated with this
// controller binary.
//
// NOTE: the implementation intentionally does not check for SuperuserAccess
// as the Version is known even to users with login access.
func (c *ControllerAPI) ControllerVersion(ctx context.Context) (params.ControllerVersionResults, error) {
	result := params.ControllerVersionResults{
		Version:   jujuversion.Current.String(),
		GitCommit: jujuversion.GitCommit,
	}
	return result, nil
}

// IdentityProviderURL returns the URL of the configured external identity
// provider for this controller or an empty string if no external identity
// provider has been configured when the controller was bootstrapped.
//
// NOTE: the implementation intentionally does not check for SuperuserAccess
// as the URL is known even to users with login access.
func (c *ControllerAPI) IdentityProviderURL(ctx context.Context) (params.StringResult, error) {
	var result params.StringResult

	cfgRes, err := c.ControllerConfig(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	if cfgRes.Config != nil {
		result.Result = corecontroller.Config(cfgRes.Config).IdentityURL()
	}
	return result, nil
}

// MongoVersion allows the introspection of the mongo version per controller
func (c *ControllerAPI) MongoVersion(ctx context.Context) (params.StringResult, error) {
	return params.StringResult{}, apiservererrors.ServerError(errors.NotSupported)
}

// DashboardConnectionInfo returns the connection information for a client to
// connect to the Juju Dashboard including any proxying information.
func (c *ControllerAPI) DashboardConnectionInfo(_ context.Context) (params.DashboardConnectionInfo, error) {
	// TODO 27-01-2025 (hmlanigan)
	// Reimplement the functionality in the controller charm.
	// The most recent implementation used mongodb state, thus the method will
	// now return Not Implemented rather than a temporary implementation with
	// dqlite.
	return params.DashboardConnectionInfo{}, errors.NotImplementedf(
		"functionality moving to the controller charm in the future, for now dashboard connection info")
}

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c *ControllerAPI) AllModels(ctx context.Context) (params.UserModelList, error) {
	result := params.UserModelList{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}

	models, err := c.modelService.ListAllModels(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, model := range models {
		userModel := params.UserModel{
			Model: params.Model{
				Name:      model.Name,
				Qualifier: model.Qualifier.String(),
				UUID:      model.UUID.String(),
				Type:      model.ModelType.String(),
			},
		}

		lastConn, err := c.accessService.LastModelLogin(ctx, user.NameFromTag(c.apiUser), model.UUID)
		if errors.Is(err, accesserrors.UserNeverAccessedModel) {
			userModel.LastConnection = nil
		} else if errors.Is(err, modelerrors.NotFound) {
			// TODO (aflynn): Once models are fully in domain, replace the line
			// below with a `continue`. When models are still in state, this
			// case is triggered because the model cannot be found in the domain
			// db. Generally, it should only be triggered if the model has been
			// removed since we got the UUID.
			userModel.LastConnection = nil
		} else if err != nil {
			return result, errors.Annotatef(err,
				"getting model last login time for user %q on model %q", c.apiUser.Name(), model.Name)
		} else {
			userModel.LastConnection = &lastConn
		}

		result.UserModels = append(result.UserModels, userModel)
	}

	return result, nil
}

// ListBlockedModels returns a list of all models on the controller
// which have a block in place.  The resulting slice is sorted by model
// name, then owner. Callers must be controller administrators to retrieve the
// list.
func (c *ControllerAPI) ListBlockedModels(ctx context.Context) (params.ModelBlockInfoList, error) {
	results := params.ModelBlockInfoList{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return results, errors.Trace(err)
	}

	models, err := c.modelService.ListAllModels(ctx)
	if err != nil {
		return results, errors.Trace(err)
	}
	for _, model := range models {
		blockService, err := c.blockCommandServiceGetter(ctx, model.UUID)
		if err != nil {
			return results, errors.Trace(err)
		}

		blocks, err := blockService.GetBlocks(ctx)
		if err != nil {
			c.logger.Debugf(ctx, "Unable to get blocks for controller: %s", err)
			return results, errors.Trace(err)
		}
		blockTypes := set.NewStrings()
		for _, block := range blocks {
			blockTypes.Add(encodeBlockType(block.Type))
		}
		results.Models = append(results.Models, params.ModelBlockInfo{
			UUID:      model.UUID.String(),
			Name:      model.Name,
			Qualifier: model.Qualifier.String(),
			Blocks:    blockTypes.SortedValues(),
		})
	}

	// Sort the resulting sequence by model name, then owner.
	sort.Sort(orderedBlockInfo(results.Models))
	return results, nil
}

func encodeBlockType(t blockcommand.BlockType) string {
	switch t {
	case blockcommand.DestroyBlock:
		return "BlockDestroy"
	case blockcommand.RemoveBlock:
		return "BlockRemove"
	case blockcommand.ChangeBlock:
		return "BlockChange"
	default:
		return "unknown"
	}
}

// HostedModelConfigs returns all the information that the client needs in
// order to connect directly with the host model's provider and destroy it
// directly.
func (c *ControllerAPI) HostedModelConfigs(ctx context.Context) (params.HostedModelConfigsResults, error) {
	result := params.HostedModelConfigsResults{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}

	models, err := c.modelService.ListAllModels(ctx)
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerModel, err := c.modelService.ControllerModel(ctx)
	if errors.Is(err, modelerrors.NotFound) {
		return result, errors.NotFoundf("controller model")
	} else if err != nil {
		return result, errors.Trace(err)
	}
	for _, model := range models {

		if model.UUID == controllerModel.UUID {
			continue
		}

		config := params.HostedModelConfig{
			Name:      model.Name,
			Qualifier: model.Qualifier.String(),
		}
		svc, err := c.modelConfigServiceGetter(ctx, model.UUID)
		if err != nil {
			return result, errors.Trace(err)
		}

		modelConf, err := svc.ModelConfig(ctx)
		if errors.Is(err, modelerrors.NotFound) {
			config.Error = apiservererrors.ServerError(errors.NotFoundf("model %q", model.UUID))
		} else if err != nil {
			config.Error = apiservererrors.ServerError(err)
		} else {
			config.Config = modelConf.AllAttrs()
		}

		if config.Error == nil {
			cloudSpec, err := c.getCloudSpec(ctx, model.UUID)
			config.CloudSpec = cloudSpec
			config.Error = apiservererrors.ServerError(err)
		}
		result.Models = append(result.Models, config)
	}

	return result, nil
}

func (c *ControllerAPI) getCloudSpec(ctx context.Context, modelUUID coremodel.UUID) (*params.CloudSpec, error) {
	cloudSpecService, err := c.cloudSpecServiceGetter(ctx, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spec, err := cloudSpecService.GetCloudSpec(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting cloud spec for model %q", modelUUID)
	}
	return common.CloudSpecToParams(spec), nil
}

// RemoveBlocks removes all the blocks in the controller.
func (c *ControllerAPI) RemoveBlocks(ctx context.Context, args params.RemoveBlocksArgs) error {
	if err := c.checkIsSuperUser(ctx); err != nil {
		return errors.Trace(err)
	}

	if !args.All {
		return errors.New("not supported")
	}

	// If there are blocks let the user know.
	uuids, err := c.modelService.ListModelUUIDs(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, uuid := range uuids {
		blockService, err := c.blockCommandServiceGetter(ctx, uuid)
		if err != nil {
			return errors.Trace(err)
		}
		err = blockService.RemoveAllBlocks(ctx)
		if err != nil {
			c.logger.Debugf(ctx, "unable to get blocks for controller: %s", err)
			return errors.Trace(err)
		}
	}

	return nil
}

// WatchAllModels starts watching events for all models in the
// controller. The returned AllWatcherId should be used with Next on the
// AllModelWatcher endpoint to receive deltas.
func (c *ControllerAPI) WatchAllModels(ctx context.Context) (params.AllWatcherId, error) {
	if err := c.checkIsSuperUser(ctx); err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}
	return params.AllWatcherId{}, errors.NotImplementedf("WatchAllModels")
}

// WatchAllModelSummaries starts watching the summary updates from the cache.
// This method is superuser access only, and watches all models in the
// controller.
func (c *ControllerAPI) WatchAllModelSummaries(ctx context.Context) (params.SummaryWatcherID, error) {
	if err := c.checkIsSuperUser(ctx); err != nil {
		return params.SummaryWatcherID{}, errors.Trace(err)
	}
	// TODO(dqlite) - implement me
	//w := c.controller.WatchAllModels()
	//return params.SummaryWatcherID{
	//	WatcherID: c.resources.Register(w),
	//}, nil
	return params.SummaryWatcherID{}, errors.NotSupportedf("WatchAllModelSummaries")
}

// WatchModelSummaries starts watching the summary updates from the cache.
// Only models that the user has access to are returned.
func (c *ControllerAPI) WatchModelSummaries(ctx context.Context) (params.SummaryWatcherID, error) {
	// TODO(dqlite) - implement me
	return params.SummaryWatcherID{}, errors.NotSupportedf("WatchModelSummaries")
	//user := c.apiUser.Id()
	//w := c.controller.WatchModelsAsUser(user)
	//return params.SummaryWatcherID{
	//	WatcherID: c.resources.Register(w),
	//}, nil
}

// GetControllerAccess returns the level of access the specified users
// have on the controller.
func (c *ControllerAPI) GetControllerAccess(ctx context.Context, req params.Entities) (params.UserAccessResults, error) {
	results := params.UserAccessResults{}
	controllerTag := names.NewControllerTag(c.controllerUUID)
	err := c.authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return results, errors.Trace(err)
	}
	isAdmin := err == nil

	users := req.Entities
	results.Results = make([]params.UserAccessResult, len(users))
	for i, userEntity := range users {
		userTag, err := names.ParseUserTag(userEntity.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !isAdmin && !c.authorizer.AuthOwner(userTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		target := permission.ID{
			ObjectType: permission.Controller,
			Key:        c.controllerUUID,
		}
		accessLevel, err := c.accessService.ReadUserAccessLevelForTarget(ctx, user.NameFromTag(userTag), target)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &params.UserAccess{
			Access:  string(accessLevel),
			UserTag: userTag.String()}
	}
	return results, nil
}

// InitiateMigration attempts to begin the migration of one or
// more models to other controllers.
func (c *ControllerAPI) InitiateMigration(ctx context.Context, reqArgs params.InitiateMigrationArgs) (
	params.InitiateMigrationResults, error,
) {
	out := params.InitiateMigrationResults{
		Results: make([]params.InitiateMigrationResult, len(reqArgs.Specs)),
	}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return out, errors.Trace(err)
	}

	for i, spec := range reqArgs.Specs {
		result := &out.Results[i]
		result.ModelTag = spec.ModelTag
		id, err := c.initiateOneMigration(ctx, spec)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.MigrationId = id
		}
	}
	return out, nil
}

func (c *ControllerAPI) initiateOneMigration(ctx context.Context, spec params.MigrationSpec) (string, error) {
	modelTag, err := names.ParseModelTag(spec.ModelTag)
	if err != nil {
		return "", errors.Annotate(err, "model tag")
	}
	modelUUID := coremodel.UUID(modelTag.Id())

	// Ensure the model exists.
	model, err := c.modelService.Model(ctx, modelUUID)
	if interrors.Is(err, modelerrors.NotFound) {
		return "", interrors.Errorf("model %q not found", modelUUID).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", interrors.Capture(err)
	}

	// Construct target info.
	specTarget := spec.TargetInfo
	controllerTag, err := names.ParseControllerTag(specTarget.ControllerTag)
	if err != nil {
		return "", errors.Annotate(err, "controller tag")
	}
	authTag, err := names.ParseUserTag(specTarget.AuthTag)
	if err != nil {
		return "", errors.Annotate(err, "auth tag")
	}
	var macs []macaroon.Slice
	if specTarget.Macaroons != "" {
		if err := json.Unmarshal([]byte(specTarget.Macaroons), &macs); err != nil {
			return "", errors.Annotate(err, "invalid macaroons")
		}
	}
	targetInfo := coremigration.TargetInfo{
		ControllerUUID:  controllerTag.Id(),
		ControllerAlias: specTarget.ControllerAlias,
		Addrs:           specTarget.Addrs,
		CACert:          specTarget.CACert,
		User:            authTag.Id(),
		Password:        specTarget.Password,
		Macaroons:       macs,
		SkipUserChecks:  specTarget.SkipUserChecks,
		Token:           specTarget.Token,
	}

	// Check if the migration is likely to succeed.
	err = c.runMigrationPrechecks(ctx, &targetInfo, model)
	if err != nil {
		return "", errors.Trace(err)
	}

	// Trigger the migration.
	modelMigrationService, err := c.modelMigrationServiceGetter(ctx, modelUUID)
	if err != nil {
		return "", errors.Trace(err)
	}
	migrationID, err := modelMigrationService.InitiateMigration(ctx, targetInfo, c.apiUser.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	return migrationID, nil
}

// ModifyControllerAccess changes the model access granted to users.
func (c *ControllerAPI) ModifyControllerAccess(ctx context.Context, args params.ModifyControllerAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	controllerTag := names.NewControllerTag(c.controllerUUID)
	err := c.authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}
	hasPermission := err == nil

	for i, arg := range args.Changes {
		if !hasPermission {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Annotate(err, "could not modify controller access"))
			continue
		}

		updateArgs := access.UpdatePermissionArgs{
			Change:  permission.AccessChange(string(arg.Action)),
			Subject: user.NameFromTag(targetUserTag),
			AccessSpec: permission.AccessSpec{
				Access: permission.Access(arg.Access),
				Target: permission.ID{
					ObjectType: permission.Controller,
					Key:        c.controllerUUID,
				},
			},
		}
		err = c.accessService.UpdatePermission(ctx, updateArgs)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// Settings that aren't specified in the params are left unchanged.
func (c *ControllerAPI) ConfigSet(ctx context.Context, args params.ControllerConfigSet) error {
	if err := c.checkIsSuperUser(ctx); err != nil {
		return errors.Trace(err)
	}

	currentCfg, err := c.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(dqlite): probably move this garbage logic to domain/controllerconfig/service
	if newValue, ok := args.Config[corecontroller.CAASImageRepo]; ok {
		var newCAASImageRepo docker.ImageRepoDetails
		if v, ok := newValue.(string); ok {
			newCAASImageRepo, err = docker.NewImageRepoDetails(v)
			if err != nil {
				return fmt.Errorf("cannot parse %s: %s%w", corecontroller.CAASImageRepo, err.Error(),
					errors.Hide(errors.NotValid))
			}
		} else {
			return fmt.Errorf("%s expected a string got %v%w", corecontroller.CAASImageRepo, v,
				errors.Hide(errors.NotValid))
		}

		var currentCAASImageRepo docker.ImageRepoDetails
		if currentValue, ok := currentCfg[corecontroller.CAASImageRepo]; !ok {
			return fmt.Errorf("cannot change %s as it is not currently set%w", corecontroller.CAASImageRepo,
				errors.Hide(errors.NotValid))
		} else if v, ok := currentValue.(string); !ok {
			return fmt.Errorf("existing %s expected a string", corecontroller.CAASImageRepo)
		} else {
			currentCAASImageRepo, err = docker.NewImageRepoDetails(v)
			if err != nil {
				return fmt.Errorf("cannot parse existing %s: %w", corecontroller.CAASImageRepo, err)
			}
		}
		// TODO: when podspec is removed, implement changing caas-image-repo.
		if newCAASImageRepo.Repository != currentCAASImageRepo.Repository {
			return fmt.Errorf("cannot change %s: repository read-only, only authentication can be updated", corecontroller.CAASImageRepo)
		}
		if !newCAASImageRepo.IsPrivate() && currentCAASImageRepo.IsPrivate() {
			return fmt.Errorf("cannot change %s: unable to remove authentication details", corecontroller.CAASImageRepo)
		}
		if newCAASImageRepo.IsPrivate() && !currentCAASImageRepo.IsPrivate() {
			return fmt.Errorf("cannot change %s: unable to add authentication details", corecontroller.CAASImageRepo)
		}
	}

	// Write Controller Config to DQLite.
	if err := c.controllerConfigService.UpdateControllerConfig(ctx, args.Config, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// runMigrationPreChecks runs prechecks on the migration and updates
// information in targetInfo as needed based on information
// retrieved from the target controller.
func (c *ControllerAPI) runMigrationPrechecks(
	ctx context.Context,
	targetInfo *coremigration.TargetInfo,
	model coremodel.Model,
) error {
	modelMigrationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ModelMigrationService, error) {
		return c.modelMigrationServiceGetter(ctx, modelUUID)
	}
	credentialServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.CredentialService, error) {
		return c.credentialServiceGetter(ctx, modelUUID)
	}
	upgradeServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.UpgradeService, error) {
		return c.upgradeServiceGetter(ctx, modelUUID)
	}
	applicationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ApplicationService, error) {
		return c.applicationServiceGetter(ctx, modelUUID)
	}
	relationServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.RelationService, error) {
		return c.relationServiceGetter(ctx, modelUUID)
	}
	statusServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.StatusService, error) {
		return c.statusServiceGetter(ctx, modelUUID)
	}
	modelAgentServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.ModelAgentService, error) {
		return c.modelAgentServiceGetter(ctx, modelUUID)
	}
	machineServiceGetterShim := func(ctx context.Context, modelUUID coremodel.UUID) (migration.MachineService, error) {
		return c.machineServiceGetter(ctx, modelUUID)
	}

	if err := migration.SourcePrecheck(
		ctx,
		model.UUID,
		c.controllerModelUUID,
		c.modelService,
		modelMigrationServiceGetterShim,
		credentialServiceGetterShim,
		upgradeServiceGetterShim,
		applicationServiceGetterShim,
		relationServiceGetterShim,
		statusServiceGetterShim,
		modelAgentServiceGetterShim,
		machineServiceGetterShim,
	); err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	modelAgentService, err := c.modelAgentServiceGetter(ctx, model.UUID)
	if err != nil {
		return errors.Trace(err)
	}
	// Check target controller.
	modelInfo, srcUserList, err := makeModelInfo(ctx,
		c.controllerConfigService, c.modelService, modelAgentService, c.modelExporter, c.store, model)
	if err != nil {
		return errors.Trace(err)
	}
	apiInfo, err := targetToAPIInfo(targetInfo)
	if err != nil {
		return errors.Trace(err)
	}
	loginProvider := migration.NewLoginProvider(*targetInfo)
	targetConn, err := api.Open(ctx, apiInfo, migration.ControllerDialOpts(loginProvider))
	if err != nil {
		return errors.Annotate(err, "connect to target controller")
	}
	defer targetConn.Close()

	if !targetInfo.SkipUserChecks {
		dstUserList, err := getTargetControllerUsers(ctx, targetConn)
		if err != nil {
			return errors.Trace(err)
		}
		if err = srcUserList.checkCompatibilityWith(dstUserList); err != nil {
			return errors.Trace(err)
		}
	}

	client := migrationtarget.NewClient(targetConn)
	if targetInfo.CACert == "" {
		targetInfo.CACert, err = client.CACert(ctx)
		if err != nil {
			if !params.IsCodeNotImplemented(err) {
				return errors.Annotatef(err, "cannot retrieve CA certificate")
			}
			// If the call's not implemented, it indicates an earlier version
			// of the controller, which we can't migrate to.
			return errors.New("controller API version is too old")
		}
	}

	err = client.Prechecks(ctx, modelInfo)
	return errors.Annotate(err, "target prechecks failed")
}

// userList encapsulates information about the users who have been granted
// access to a model or the users known to a particular controller.
type userList struct {
	identityURL string
	users       set.Strings
}

// checkCompatibilityWith ensures that the set of users granted access to
// the model being migrated is present in the destination (migration target)
// controller.
func (src *userList) checkCompatibilityWith(dst userList) error {
	srcUsers, dstUsers := src.users, dst.users

	// If external users have access to this model we can only allow the
	// migration to proceed if:
	// - the local users from src exist in the dst, and
	// - both controllers are configured with the same identity provider URL
	srcExtUsers := filterSet(srcUsers, func(u string) bool {
		return strings.Contains(u, "@")
	})

	if srcExtUsers.Size() != 0 {
		localSrcUsers := srcUsers.Difference(srcExtUsers)

		// In this case external user lookups will most likely not work.
		// Display an appropriate error message depending on whether
		// the local users are present in dst or not.
		if src.identityURL != dst.identityURL {
			missing := localSrcUsers.Difference(dstUsers)
			if missing.Size() == 0 {
				return errors.Errorf(`cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you can remove the following users from the current model:
  - %s`,
					strings.Join(srcExtUsers.Values(), "\n  - "),
				)
			}

			return errors.Errorf(`cannot initiate migration as external users have been granted access to the model
and the two controllers have different identity provider configurations. To resolve
this issue you need to remove the following users from the current model:
  - %s

and add the following users to the destination controller or remove them from
the current model:
  - %s`,
				strings.Join(srcExtUsers.Values(), "\n  - "),
				strings.Join(localSrcUsers.Difference(dstUsers).Values(), "\n  - "),
			)

		}

		// External user lookups will work out of the box. We only need
		// to ensure that the local model users are present in dst
		srcUsers = localSrcUsers
	}

	if missing := srcUsers.Difference(dstUsers); missing.Size() != 0 {
		return errors.Errorf(`cannot initiate migration as the users granted access to the model do not exist
on the destination controller. To resolve this issue you can add the following
users to the destination controller or remove them from the current model:
  - %s`, strings.Join(missing.Values(), "\n  - "))
	}

	return nil
}

func makeModelInfo(ctx context.Context,
	controllerConfigService ControllerConfigService,
	modelService ModelService,
	modelAgentService ModelAgentService,
	modelExporterFn func(context.Context, coremodel.UUID) (ModelExporter, error),
	store objectstore.ObjectStore,
	model coremodel.Model,
) (coremigration.ModelInfo, userList, error) {
	var empty coremigration.ModelInfo
	var ul userList

	modelExporter, err := modelExporterFn(ctx, model.UUID)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}
	description, err := modelExporter.ExportModel(ctx, store)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}

	users, err := modelService.GetModelUsers(ctx, model.UUID)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}
	ul.users = set.NewStrings()
	for _, u := range users {
		ul.users.Add(u.Name.Name())
	}

	// Retrieve agent version for the model.
	agentVersion, err := modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return empty, userList{}, fmt.Errorf("getting model %q: %w", model.UUID, err)
	}

	// Retrieve agent version for the controller.
	controllerModel, err := modelService.ControllerModel(ctx)
	if err != nil {
		return empty, userList{}, fmt.Errorf("getting controller model info: %w", err)
	}

	coreConf, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return empty, userList{}, errors.Trace(err)
	}

	ul.identityURL = coreConf.IdentityURL()
	return coremigration.ModelInfo{
		UUID:                   model.UUID.String(),
		Name:                   model.Name,
		Qualifier:              model.Qualifier,
		AgentVersion:           agentVersion,
		ControllerAgentVersion: controllerModel.AgentVersion,
		ModelDescription:       description,
	}, ul, nil
}

func getTargetControllerUsers(ctx context.Context, conn api.Connection) (userList, error) {
	ul := userList{}

	userClient := usermanager.NewClient(conn)
	users, err := userClient.UserInfo(ctx, nil, usermanager.AllUsers)
	if err != nil {
		return ul, errors.Trace(err)
	}

	ul.users = set.NewStrings()
	for _, u := range users {
		ul.users.Add(u.Username)
	}

	ctrlClient := controllerclient.NewClient(conn)
	ul.identityURL, err = ctrlClient.IdentityProviderURL(ctx)
	if err != nil {
		return ul, errors.Trace(err)
	}

	return ul, nil
}

func targetToAPIInfo(ti *coremigration.TargetInfo) (*api.Info, error) {
	if ti.User != "" && !names.IsValidUser(ti.User) {
		return nil, errors.Errorf("user %q is not valid", ti.User)
	}
	info := &api.Info{
		Addrs:     ti.Addrs,
		CACert:    ti.CACert,
		Password:  ti.Password,
		Macaroons: ti.Macaroons,
	}
	// Only local users must be added to the api info.
	// For external users, the tag needs to be left empty.
	if userTag := names.NewUserTag(ti.User); ti.User != "" && userTag.IsLocal() {
		info.Tag = userTag
	}
	return info, nil
}

type orderedBlockInfo []params.ModelBlockInfo

func (o orderedBlockInfo) Len() int {
	return len(o)
}

func (o orderedBlockInfo) Less(i, j int) bool {
	if o[i].Name < o[j].Name {
		return true
	}
	if o[i].Name > o[j].Name {
		return false
	}

	if o[i].Qualifier < o[j].Qualifier {
		return true
	}
	if o[i].Qualifier > o[j].Qualifier {
		return false
	}

	// Unreachable based on the rules of there not being duplicate
	// models of the same name for the same owner, but return false
	// instead of panicing.
	return false
}

func (o orderedBlockInfo) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

func filterSet(s set.Strings, keep func(string) bool) set.Strings {
	out := set.NewStrings()
	for _, v := range s.Values() {
		if keep(v) {
			out.Add(v)
		}
	}

	return out
}
