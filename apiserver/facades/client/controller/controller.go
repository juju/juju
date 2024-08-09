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
	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/usermanager"
	controllerclient "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
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
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/pubsub/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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
	*common.ModelStatusAPI
	cloudspec.CloudSpecer

	state                    Backend
	statePool                *state.StatePool
	authorizer               facade.Authorizer
	apiUser                  names.UserTag
	resources                facade.Resources
	presence                 facade.Presence
	hub                      facade.Hub
	cloudService             common.CloudService
	credentialService        common.CredentialService
	upgradeService           UpgradeService
	controllerConfigService  ControllerConfigService
	accessService            ControllerAccessService
	modelService             ModelService
	modelConfigServiceGetter func(coremodel.UUID) ModelConfigService
	modelExporter            ModelExporter
	store                    objectstore.ObjectStore
	leadership               leadership.Reader

	logger corelogger.Logger

	controllerTag names.ControllerTag
}

type ControllerAPIv11 struct {
	*ControllerAPI
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
	presence facade.Presence,
	hub facade.Hub,
	logger corelogger.Logger,
	controllerConfigService ControllerConfigService,
	externalControllerService common.ExternalControllerService,
	cloudService common.CloudService,
	credentialService common.CredentialService,
	upgradeService UpgradeService,
	accessService ControllerAccessService,
	modelService ModelService,
	modelConfigServiceGetter func(coremodel.UUID) ModelConfigService,
	modelExporter ModelExporter,
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
		ModelStatusAPI: common.NewModelStatusAPI(
			common.NewModelManagerBackend(environs.ProviderConfigSchemaSource(cloudService), model, pool),
			authorizer,
			apiUser,
		),
		CloudSpecer: cloudspec.NewCloudSpecV2(
			resources,
			cloudspec.MakeCloudSpecGetter(pool, cloudService, credentialService),
			cloudspec.MakeCloudSpecWatcherForModel(st, cloudService),
			cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, credentialService),
			common.AuthFuncForTag(model.ModelTag()),
		),
		state:                    stateShim{State: st},
		statePool:                pool,
		authorizer:               authorizer,
		apiUser:                  apiUser,
		resources:                resources,
		presence:                 presence,
		hub:                      hub,
		logger:                   logger,
		controllerConfigService:  controllerConfigService,
		credentialService:        credentialService,
		upgradeService:           upgradeService,
		cloudService:             cloudService,
		accessService:            accessService,
		modelService:             modelService,
		modelConfigServiceGetter: modelConfigServiceGetter,
		controllerTag:            st.ControllerTag(),
		modelExporter:            modelExporter,
		store:                    store,
		leadership:               leadership,
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

// dashboardConnectionInforForCAAS returns a dashboard connection for a Juju
// dashboard deployed on CAAS.
func (c *ControllerAPI) dashboardConnectionInfoForCAAS(
	ctx context.Context,
	m *state.Model,
	applicationName string,
) (*params.Proxy, error) {
	configGetter := stateenvirons.EnvironConfigGetter{Model: m, CloudService: c.cloudService, CredentialService: c.credentialService}
	environ, err := common.EnvironFuncForModel(m, c.cloudService, c.credentialService, configGetter)(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	caasBroker, ok := environ.(caas.Broker)
	if !ok {
		return nil, errors.New("cannot get CAAS environ for model")
	}

	dashboardApp, err := c.state.Application(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := dashboardApp.CharmConfig(coremodel.GenerationMaster)
	if err != nil {
		return nil, errors.Trace(err)
	}
	port, ok := cfg["port"]
	if !ok {
		return nil, errors.NotFoundf("dashboard port in charm config")
	}

	proxier, err := caasBroker.ProxyToApplication(ctx, applicationName, fmt.Sprint(port))
	if err != nil {
		return nil, err
	}

	return params.NewProxy(proxier)
}

// dashboardConnectionInforForIAAS returns a dashboard connection for a Juju
// dashboard deployed on IAAS.
func (c *ControllerAPI) dashboardConnectionInfoForIAAS(
	ctx context.Context,
	appName string,
	appSettings map[string]interface{},
) (*params.DashboardConnectionSSHTunnel, error) {
	addr, ok := appSettings["dashboard-ingress"]
	if !ok {
		return nil, errors.NotFoundf("dashboard address in relation data")
	}

	// TODO: support cross-model relations
	// If the dashboard app is in a different model, this will try to look in
	// the controller model, returning `application "dashboard" not found`
	dashboardApp, err := c.state.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := dashboardApp.CharmConfig(coremodel.GenerationMaster)
	if err != nil {
		return nil, errors.Trace(err)
	}
	port, ok := cfg["port"]
	if !ok {
		return nil, errors.NotFoundf("dashboard port in charm config")
	}

	model, err := c.state.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelName := model.Name()
	ctrCfg, err := c.controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerName := ctrCfg.ControllerName()

	return &params.DashboardConnectionSSHTunnel{
		Model:  fmt.Sprintf("%s:%s", controllerName, modelName),
		Entity: fmt.Sprintf("%s/leader", appName),
		Host:   fmt.Sprintf("%s", addr),
		Port:   fmt.Sprintf("%d", port),
	}, nil
}

// DashboardConnectionInfo returns the connection information for a client to
// connect to the Juju Dashboard including any proxying information.
func (c *ControllerAPI) DashboardConnectionInfo(ctx context.Context) (params.DashboardConnectionInfo, error) {
	getDashboardInfo := func() (params.DashboardConnectionInfo, error) {
		rval := params.DashboardConnectionInfo{}
		controllerApp, err := c.state.Application(bootstrap.ControllerApplicationName)
		if err != nil {
			return rval, errors.Trace(err)
		}

		rels, err := controllerApp.Relations()
		if err != nil {
			return rval, errors.Trace(err)
		}

		for _, rel := range rels {
			ep, err := rel.Endpoint(controllerApp.Name())
			if err != nil {
				return rval, errors.Trace(err)
			}
			if ep.Name != "dashboard" {
				continue
			}

			model, ph, err := c.statePool.GetModel(rel.ModelUUID())
			if err != nil {
				return rval, errors.Trace(err)
			}
			defer ph.Release()

			relatedEps, err := rel.RelatedEndpoints(controllerApp.Name())
			if err != nil {
				return rval, errors.Trace(err)
			}
			related := relatedEps[0]

			appSettings, err := rel.ApplicationSettings(related.ApplicationName)
			if err != nil {
				return rval, errors.Trace(err)
			}

			if model.Type() != state.ModelTypeCAAS {
				sshConnection, err := c.dashboardConnectionInfoForIAAS(
					ctx,
					related.ApplicationName,
					appSettings)
				rval.SSHConnection = sshConnection
				return rval, err
			}

			proxyConnection, err := c.dashboardConnectionInfoForCAAS(ctx, model, related.ApplicationName)
			rval.ProxyConnection = proxyConnection
			return rval, err
		}

		return rval, errors.NotFoundf("dashboard")
	}
	conInfo, err := getDashboardInfo()

	if conInfo.ProxyConnection != nil && conInfo.SSHConnection != nil {
		return params.DashboardConnectionInfo{},
			errors.New("cannot set both proxy and ssh connection for dashboard connection info")
	}
	conInfo.Error = apiservererrors.ServerError(err)
	return conInfo, nil
}

// AllModels allows controller administrators to get the list of all the
// models in the controller.
func (c *ControllerAPI) AllModels(ctx context.Context) (params.UserModelList, error) {
	result := params.UserModelList{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}

	modelsWithLogins, err := c.modelService.ModelsWithLastLogin(ctx, user.UUID(c.apiUser.Id()))
	if err != nil {
		return params.UserModelList{}, fmt.Errorf("getting models: %w", err)
	}

	for _, model := range modelsWithLogins {
		result.UserModels = append(result.UserModels, params.UserModel{
			Model: params.Model{
				Name:     model.Model.Name,
				UUID:     model.Model.UUID.String(),
				Type:     model.Model.ModelType.String(),
				OwnerTag: names.NewUserTag(model.Model.OwnerName).String(),
			},
			LastConnection: model.LastLogin,
		})
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
	blocks, err := c.state.AllBlocksForController()
	if err != nil {
		return results, errors.Trace(err)
	}

	modelBlocks := make(map[string][]string)
	for _, block := range blocks {
		uuid := block.ModelUUID()
		types, ok := modelBlocks[uuid]
		if !ok {
			types = []string{block.Type().String()}
		} else {
			types = append(types, block.Type().String())
		}
		modelBlocks[uuid] = types
	}

	for uuid, blocks := range modelBlocks {
		model, ph, err := c.statePool.GetModel(uuid)
		if err != nil {
			c.logger.Debugf("unable to retrieve model %s: %v", uuid, err)
			continue
		}
		results.Models = append(results.Models, params.ModelBlockInfo{
			UUID:     model.UUID(),
			Name:     model.Name(),
			OwnerTag: model.Owner().String(),
			Blocks:   blocks,
		})
		ph.Release()
	}

	// Sort the resulting sequence by model name, then owner.
	sort.Sort(orderedBlockInfo(results.Models))
	return results, nil
}

// ModelConfig returns the model config for the controller model.
//
// Deprecated: this facade method will be removed in 4.0 when this facade is
// converted to a multi-model facade. Please use the ModelConfig facade's
// ModelGet method instead:
// [github.com/juju/juju/apiserver/facades/client/modelconfig.ModelConfigAPI.ModelGet]
func (c *ControllerAPIv11) ModelConfig(ctx context.Context) (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}

	controllerState, err := c.statePool.SystemState()
	if err != nil {
		return result, errors.Trace(err)
	}
	controllerModel, err := controllerState.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	cfg, err := controllerModel.Config()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = make(map[string]params.ConfigValue)
	for name, val := range cfg.AllAttrs() {
		result.Config[name] = params.ConfigValue{
			Value: val,
		}
	}
	return result, nil
}

// HostedModelConfigs returns all the information that the client needs in
// order to connect directly with the host model's provider and destroy it
// directly.
func (c *ControllerAPI) HostedModelConfigs(ctx context.Context) (params.HostedModelConfigsResults, error) {
	result := params.HostedModelConfigsResults{}
	if err := c.checkIsSuperUser(ctx); err != nil {
		return result, errors.Trace(err)
	}

	hostedModels, err := c.modelService.HostedModels(ctx)
	if err != nil {
		return result, fmt.Errorf("getting hosted models: %w", err)
	}

	for _, hostedModel := range hostedModels {
		config := params.HostedModelConfig{
			Name:     hostedModel.Model.Name,
			OwnerTag: names.NewUserTag(hostedModel.Model.OwnerName).String(),
		}

		modelConf, err := c.modelConfigServiceGetter(hostedModel.Model.UUID).ModelConfig(ctx)
		if err != nil {
			config.Error = apiservererrors.ServerError(err)
			continue
		}
		config.Config = modelConf.AllAttrs()

		cloudSpec := c.GetCloudSpec(ctx, names.NewModelTag(hostedModel.Model.UUID.String()))
		if config.Error == nil {
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
	return errors.Trace(c.state.RemoveAllBlocksForController())
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
	for i, user := range users {
		userTag, err := names.ParseUserTag(user.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if !isAdmin && !c.authorizer.AuthOwner(userTag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		spec := permission.AccessSpec{
			Access: permission.SuperuserAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        c.controllerTag.Id(),
			},
		}
		accessLevel, err := c.accessService.ReadUserAccessLevelForTarget(ctx, userTag.Id(), spec.Target)
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

	// Check if the migration is likely to succeed.
	systemState, err := c.statePool.SystemState()
	if err != nil {
		return "", errors.Trace(err)
	}
	leaders, err := c.leadership.Leaders()
	if err != nil {
		return "", errors.Trace(err)
	}
	if err := runMigrationPrechecks(
		ctx,
		hostedState.State, systemState,
		&targetInfo, c.presence,
		c.controllerConfigService,
		c.cloudService,
		c.credentialService,
		c.upgradeService,
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

		external := !targetUserTag.IsLocal()
		updateArgs := access.UpdatePermissionArgs{
			AccessSpec: permission.AccessSpec{
				Access: permission.Access(arg.Access),
				Target: permission.ID{
					ObjectType: permission.Controller,
					Key:        c.controllerTag.Id(),
				},
			},
			AddUser:  true,
			External: &external,
			ApiUser:  c.apiUser.Id(),
			Change:   permission.AccessChange(string(arg.Action)),
			Subject:  targetUserTag.Id(),
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
	presence facade.Presence,
	controllerConfigService ControllerConfigService,
	cloudService common.CloudService,
	credentialService common.CredentialService,
	upgradeService UpgradeService,
	modelExporter ModelExporter,
	store objectstore.ObjectStore,
	leaders map[string]string,
) error {
	// Check model and source controller.
	backend, err := migration.PrecheckShim(st, ctlrSt)
	if err != nil {
		return errors.Annotate(err, "creating backend")
	}
	modelPresence := presence.ModelPresence(st.ModelUUID())
	controllerPresence := presence.ModelPresence(ctlrSt.ModelUUID())

	if err := migration.SourcePrecheck(
		ctx,
		backend,
		modelPresence, controllerPresence,
		cloudspec.MakeCloudSpecGetterForModel(st, cloudService, credentialService),
		credentialService,
		upgradeService,
	); err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	// Check target controller.
	modelInfo, srcUserList, err := makeModelInfo(ctx, st, ctlrSt,
		controllerConfigService, modelExporter, store, leaders)
	if err != nil {
		return errors.Trace(err)
	}
	targetConn, err := api.Open(targetToAPIInfo(targetInfo), migration.ControllerDialOpts())
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
	return errors.Annotate(err, "target prechecks failed 2")
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

func makeModelInfo(ctx context.Context, st, ctlrSt *state.State,
	controllerConfigService ControllerConfigService,
	modelExporter ModelExporter,
	store objectstore.ObjectStore,
	leaders map[string]string,
) (coremigration.ModelInfo, userList, error) {
	var empty coremigration.ModelInfo
	var ul userList

	model, err := st.Model()
	if err != nil {
		return empty, ul, errors.Trace(err)
	}

	description, err := modelExporter.ExportModel(ctx, leaders, store)
	if err != nil {
		return empty, ul, errors.Trace(err)
	}

	users, err := model.Users()
	if err != nil {
		return empty, ul, errors.Trace(err)
	}
	ul.users = set.NewStrings()
	for _, u := range users {
		ul.users.Add(u.UserName)
	}

	// Retrieve agent version for the model.
	conf, err := model.ModelConfig(ctx)
	if err != nil {
		return empty, userList{}, errors.Trace(err)
	}
	agentVersion, _ := conf.AgentVersion()

	// Retrieve agent version for the controller.
	controllerModel, err := ctlrSt.Model()
	if err != nil {
		return empty, userList{}, errors.Trace(err)
	}
	controllerConfig, err := controllerModel.Config()
	if err != nil {
		return empty, userList{}, errors.Trace(err)
	}
	controllerVersion, _ := controllerConfig.AgentVersion()

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
		ControllerAgentVersion: controllerVersion,
		ModelDescription:       description,
	}, ul, nil
}

func getTargetControllerUsers(ctx context.Context, conn api.Connection) (userList, error) {
	ul := userList{}

	userClient := usermanager.NewClient(conn)
	users, err := userClient.UserInfo(nil, usermanager.AllUsers)
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
