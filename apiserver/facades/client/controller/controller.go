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
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/usermanager"
	controllerclient "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
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
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ModelExporter exports a model to a description.Model.
type ModelExporter interface {
	// ExportModel exports a model to a description.Model.
	// It requires a known set of leaders to be passed in, so that applications
	// can have their leader set correctly once imported.
	// The objectstore is used to retrieve charms and resources for export.
	ExportModel(context.Context, map[string]string, objectstore.ObjectStore) (description.Model, error)
}

// ControllerAPI provides the Controller API.
type ControllerAPI struct {
	*common.ControllerConfigAPI
	*commonmodel.ModelStatusAPI
	cloudspec.CloudSpecer

	state                     Backend
	statePool                 *state.StatePool
	authorizer                facade.Authorizer
	apiUser                   names.UserTag
	resources                 facade.Resources
	hub                       facade.Hub
	cloudService              CloudService
	credentialService         CredentialService
	upgradeService            UpgradeService
	controllerConfigService   ControllerConfigService
	accessService             ControllerAccessService
	modelService              ModelService
	modelInfoService          ModelInfoService
	blockCommandService       common.BlockCommandService
	applicationServiceGetter  func(context.Context, coremodel.UUID) (ApplicationService, error)
	modelAgentServiceGetter   func(context.Context, coremodel.UUID) (common.ModelAgentService, error)
	modelConfigServiceGetter  func(context.Context, coremodel.UUID) (cloudspec.ModelConfigService, error)
	blockCommandServiceGetter func(context.Context, coremodel.UUID) (BlockCommandService, error)
	proxyService              ProxyService
	modelExporter             func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error)
	store                     objectstore.ObjectStore
	leadership                leadership.Reader
	logger                    corelogger.Logger
	controllerTag             names.ControllerTag
}

// LatestAPI is used for testing purposes to create the latest
// controller API.
var LatestAPI = makeControllerAPI

// NewControllerAPI creates a new api server endpoint for operations
// on a controller.
func NewControllerAPI(
	ctx context.Context,
	st *state.State,
	pool *state.StatePool,
	authorizer facade.Authorizer,
	resources facade.Resources,
	hub facade.Hub,
	logger corelogger.Logger,
	controllerConfigService ControllerConfigService,
	externalControllerService common.ExternalControllerService,
	cloudService CloudService,
	credentialService CredentialService,
	upgradeService UpgradeService,
	accessService ControllerAccessService,
	machineServiceGetter func(context.Context, coremodel.UUID) (commonmodel.MachineService, error),
	modelService ModelService,
	modelInfoService ModelInfoService,
	blockCommandService common.BlockCommandService,
	applicationServiceGetter func(context.Context, coremodel.UUID) (ApplicationService, error),
	modelAgentServiceGetter func(context.Context, coremodel.UUID) (common.ModelAgentService, error),
	modelConfigServiceGetter func(context.Context, coremodel.UUID) (cloudspec.ModelConfigService, error),
	blockCommandServiceGetter func(context.Context, coremodel.UUID) (BlockCommandService, error),
	proxyService ProxyService,
	modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
	store objectstore.ObjectStore,
	leadership leadership.Reader,
) (*ControllerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(apiservererrors.ErrPerm)
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ControllerAPI{
		ControllerConfigAPI: common.NewControllerConfigAPI(
			st,
			controllerConfigService,
			externalControllerService,
		),
		ModelStatusAPI: commonmodel.NewModelStatusAPI(
			commonmodel.NewModelManagerBackend(model, pool),
			machineServiceGetter,
			authorizer,
			apiUser,
		),
		CloudSpecer: cloudspec.NewCloudSpecV2(
			resources,
			cloudspec.MakeCloudSpecGetter(pool, cloudService, credentialService, modelConfigServiceGetter),
			cloudspec.MakeCloudSpecWatcherForModel(st, cloudService),
			cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, credentialService),
			common.AuthFuncForTag(model.ModelTag()),
		),
		state:                     stateShim{State: st},
		statePool:                 pool,
		authorizer:                authorizer,
		apiUser:                   apiUser,
		resources:                 resources,
		hub:                       hub,
		logger:                    logger,
		controllerConfigService:   controllerConfigService,
		credentialService:         credentialService,
		upgradeService:            upgradeService,
		cloudService:              cloudService,
		applicationServiceGetter:  applicationServiceGetter,
		accessService:             accessService,
		modelService:              modelService,
		blockCommandService:       blockCommandService,
		modelInfoService:          modelInfoService,
		modelAgentServiceGetter:   modelAgentServiceGetter,
		modelConfigServiceGetter:  modelConfigServiceGetter,
		blockCommandServiceGetter: blockCommandServiceGetter,
		proxyService:              proxyService,
		controllerTag:             st.ControllerTag(),
		modelExporter:             modelExporter,
		store:                     store,
		leadership:                leadership,
	}, nil
}

func (c *ControllerAPI) checkIsSuperUser(ctx context.Context) error {
	return c.authorizer.HasPermission(ctx, permission.SuperuserAccess, c.controllerTag)
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
	result := params.StringResult{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}
	version, err := c.state.MongoVersion()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Result = version
	return result, nil
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

	modelUUIDs, err := c.state.AllModelUUIDs()
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, modelUUID := range modelUUIDs {
		st, err := c.statePool.Get(modelUUID)
		if err != nil {
			// This model could have been removed.
			if errors.Is(err, errors.NotFound) {
				continue
			}
			return result, errors.Trace(err)
		}
		defer st.Release()

		model, err := st.Model()
		if err != nil {
			return result, errors.Trace(err)
		}

		userModel := params.UserModel{
			Model: params.Model{
				Name:     model.Name(),
				UUID:     model.UUID(),
				Type:     string(model.Type()),
				OwnerTag: model.Owner().String(),
			},
		}

		lastConn, err := c.accessService.LastModelLogin(ctx, user.NameFromTag(c.apiUser), coremodel.UUID(model.UUID()))
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
				"getting model last login time for user %q on model %q", c.apiUser.Name(), model.Name())
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

	// If there are blocks let the user know.
	uuids, err := c.state.AllModelUUIDs()
	if err != nil {
		return results, errors.Trace(err)
	}
	modelBlocks := make(map[string]set.Strings)
	for _, uuid := range uuids {
		blockService, err := c.blockCommandServiceGetter(ctx, coremodel.UUID(uuid))
		if err != nil {
			return results, errors.Trace(err)
		}

		blocks, err := blockService.GetBlocks(ctx)
		if err != nil {
			c.logger.Debugf(context.TODO(), "Unable to get blocks for controller: %s", err)
			return results, errors.Trace(err)
		}
		blockTypes := set.NewStrings()
		for _, block := range blocks {
			blockTypes.Add(encodeBlockType(block.Type))
		}
		modelBlocks[uuid] = blockTypes
	}

	for uuid, blocks := range modelBlocks {
		if len(blocks) == 0 {
			continue
		}
		model, ph, err := c.statePool.GetModel(uuid)
		if err != nil {
			c.logger.Debugf(context.TODO(), "unable to retrieve model %s: %v", uuid, err)
			continue
		}
		results.Models = append(results.Models, params.ModelBlockInfo{
			UUID:     model.UUID(),
			Name:     model.Name(),
			OwnerTag: model.Owner().String(),
			Blocks:   blocks.SortedValues(),
		})
		ph.Release()
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

	modelUUIDs, err := c.state.AllModelUUIDs()
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, modelUUID := range modelUUIDs {
		if modelUUID == c.state.ControllerModelUUID() {
			continue
		}
		st, err := c.statePool.Get(modelUUID)
		if err != nil {
			// This model could have been removed.
			if errors.Is(err, errors.NotFound) {
				continue
			}
			return result, errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return result, errors.Trace(err)
		}

		config := params.HostedModelConfig{
			Name:     model.Name(),
			OwnerTag: model.Owner().String(),
		}
		svc, err := c.modelConfigServiceGetter(ctx, coremodel.UUID(modelUUID))
		if err != nil {
			return result, errors.Trace(err)
		}
		modelConf, err := svc.ModelConfig(ctx)
		if err != nil {
			config.Error = apiservererrors.ServerError(err)
		} else {
			config.Config = modelConf.AllAttrs()
		}

		if config.Error == nil {
			cloudSpec := c.GetCloudSpec(ctx, model.ModelTag())
			config.CloudSpec = cloudSpec.Result
			config.Error = cloudSpec.Error
		}
		result.Models = append(result.Models, config)
	}

	return result, nil
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
	uuids, err := c.state.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}
	for _, uuid := range uuids {
		blockService, err := c.blockCommandServiceGetter(ctx, coremodel.UUID(uuid))
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
	err := c.authorizer.HasPermission(ctx, permission.SuperuserAccess, c.controllerTag)
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
			Key:        c.controllerTag.Id(),
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

	// Ensure the model exists.
	if modelExists, err := c.state.ModelExists(modelTag.Id()); err != nil {
		return "", errors.Annotate(err, "reading model")
	} else if !modelExists {
		return "", errors.NotFoundf("model")
	}

	hostedState, err := c.statePool.Get(modelTag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	defer hostedState.Release()

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
		ControllerTag:   controllerTag,
		ControllerAlias: specTarget.ControllerAlias,
		Addrs:           specTarget.Addrs,
		CACert:          specTarget.CACert,
		AuthTag:         authTag,
		Password:        specTarget.Password,
		Macaroons:       macs,
	}

	// TODO (stickupkid): This is terrible, this should be 1 call, not lots of
	// individual calls to get the services.
	modelConfigService, err := c.modelConfigServiceGetter(ctx, coremodel.UUID(modelTag.Id()))
	if err != nil {
		return "", errors.Trace(err)
	}
	modelAgentService, err := c.modelAgentServiceGetter(ctx, coremodel.UUID(modelTag.Id()))
	if err != nil {
		return "", errors.Trace(err)
	}

	// Check if the migration is likely to succeed.
	systemState, err := c.statePool.SystemState()
	if err != nil {
		return "", errors.Trace(err)
	}
	leaders, err := c.leadership.Leaders()
	if err != nil {
		return "", errors.Trace(err)
	}
	applicationService, err := c.applicationServiceGetter(ctx, coremodel.UUID(hostedState.ModelUUID()))
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := runMigrationPrechecks(
		ctx,
		hostedState.State,
		systemState,
		&targetInfo,
		c.controllerConfigService,
		c.cloudService,
		c.credentialService,
		modelAgentService,
		modelConfigService,
		c.upgradeService,
		c.modelService,
		applicationService,
		c.modelExporter,
		c.store,
		leaders,
	); err != nil {
		return "", errors.Trace(err)
	}

	// Trigger the migration.
	mig, err := hostedState.CreateMigration(state.MigrationSpec{
		InitiatedBy: c.apiUser,
		TargetInfo:  targetInfo,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return mig.Id(), nil
}

// ModifyControllerAccess changes the model access granted to users.
func (c *ControllerAPI) ModifyControllerAccess(ctx context.Context, args params.ModifyControllerAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	err := c.authorizer.HasPermission(ctx, permission.SuperuserAccess, c.controllerTag)
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
					Key:        c.controllerTag.Id(),
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
	// TODO(thumper): add a version to controller config to allow for
	// simultaneous updates and races in publishing, potentially across
	// HA servers.
	cfg, err := c.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := c.hub.Publish(
		controller.ConfigChanged,
		controller.ConfigChangedMessage{Config: cfg}); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// runMigrationPreChecks runs prechecks on the migration and updates
// information in targetInfo as needed based on information
// retrieved from the target controller.
var runMigrationPrechecks = func(
	ctx context.Context,
	st, ctlrSt *state.State,
	targetInfo *coremigration.TargetInfo,
	controllerConfigService ControllerConfigService,
	cloudService CloudService,
	credentialService CredentialService,
	modelAgentService ModelAgentService,
	modelConfigService ModelConfigService,
	upgradeService UpgradeService,
	modelService ModelService,
	applicationService ApplicationService,
	modelExporter func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
	store objectstore.ObjectStore,
	leaders map[string]string,
) error {
	// Check model and source controller.
	backend, err := migration.PrecheckShim(st, ctlrSt)
	if err != nil {
		return errors.Annotate(err, "creating backend")
	}

	if err := migration.SourcePrecheck(
		ctx,
		backend,
		cloudspec.MakeCloudSpecGetterForModel(st, cloudService, credentialService, modelConfigService),
		credentialService,
		upgradeService,
		applicationService,
		modelAgentService,
	); err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	// Check target controller.
	modelInfo, srcUserList, err := makeModelInfo(ctx, st,
		controllerConfigService, modelService, modelAgentService, modelExporter, store, leaders)
	if err != nil {
		return errors.Trace(err)
	}
	targetConn, err := api.Open(ctx, targetToAPIInfo(targetInfo), migration.ControllerDialOpts())
	if err != nil {
		return errors.Annotate(err, "connect to target controller")
	}
	defer targetConn.Close()
	dstUserList, err := getTargetControllerUsers(ctx, targetConn)
	if err != nil {
		return errors.Trace(err)
	}
	if err = srcUserList.checkCompatibilityWith(dstUserList); err != nil {
		return errors.Trace(err)
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

func makeModelInfo(ctx context.Context, st *state.State,
	controllerConfigService ControllerConfigService,
	modelService ModelService,
	modelAgentService ModelAgentService,
	modelExporterFn func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (ModelExporter, error),
	store objectstore.ObjectStore,
	leaders map[string]string,
) (coremigration.ModelInfo, userList, error) {
	var empty coremigration.ModelInfo
	var ul userList

	model, err := st.Model()
	if err != nil {
		return empty, ul, errors.Trace(err)
	}

	modelExporter, err := modelExporterFn(ctx, coremodel.UUID(model.UUID()), st)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}
	description, err := modelExporter.ExportModel(ctx, leaders, store)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}

	modelID := coremodel.UUID(model.UUID())

	users, err := modelService.GetModelUsers(ctx, modelID)
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
		return empty, userList{}, fmt.Errorf("getting model %q: %w", modelID, err)
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
		UUID:                   model.UUID(),
		Name:                   model.Name(),
		Owner:                  model.Owner(),
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

func targetToAPIInfo(ti *coremigration.TargetInfo) *api.Info {
	info := &api.Info{
		Addrs:     ti.Addrs,
		CACert:    ti.CACert,
		Password:  ti.Password,
		Macaroons: ti.Macaroons,
	}
	// Only local users must be added to the api info.
	// For external users, the tag needs to be left empty.
	if ti.AuthTag.IsLocal() {
		info.Tag = ti.AuthTag
	}
	return info
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

	if o[i].OwnerTag < o[j].OwnerTag {
		return true
	}
	if o[i].OwnerTag > o[j].OwnerTag {
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
