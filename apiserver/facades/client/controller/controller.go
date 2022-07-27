// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/txn/v2"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/usermanager"
	controllerclient "github.com/juju/juju/api/controller/controller"
	"github.com/juju/juju/api/controller/migrationtarget"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/cache"
	coremigration "github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/pubsub/controller"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	jujuversion "github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.controller")

// ControllerAPI provides the Controller API.
type ControllerAPI struct {
	*common.ControllerConfigAPI
	*common.ModelStatusAPI
	cloudspec.CloudSpecer

	state      Backend
	statePool  *state.StatePool
	authorizer facade.Authorizer
	apiUser    names.UserTag
	resources  facade.Resources
	presence   facade.Presence
	hub        facade.Hub
	controller *cache.Controller

	multiwatcherFactory multiwatcher.Factory
}

// LatestAPI is used for testing purposes to create the latest
// controller API.
var LatestAPI = newControllerAPIv11

// TestingAPI is an escape hatch for requesting a controller API that won't
// allow auth to correctly happen for ModelStatus. I'm not convicned this
// should exist at all.
var TestingAPI = LatestAPI

// NewControllerAPI creates a new api server endpoint for operations
// on a controller.
func NewControllerAPI(
	st *state.State,
	pool *state.StatePool,
	authorizer facade.Authorizer,
	resources facade.Resources,
	presence facade.Presence,
	hub facade.Hub,
	factory multiwatcher.Factory,
	controller *cache.Controller,
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
		ControllerConfigAPI: common.NewStateControllerConfig(st),
		ModelStatusAPI: common.NewModelStatusAPI(
			common.NewModelManagerBackend(model, pool),
			authorizer,
			apiUser,
		),
		CloudSpecer: cloudspec.NewCloudSpecV2(
			resources,
			cloudspec.MakeCloudSpecGetter(pool),
			cloudspec.MakeCloudSpecWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
			cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
			common.AuthFuncForTag(model.ModelTag()),
		),
		state:               stateShim{st},
		statePool:           pool,
		authorizer:          authorizer,
		apiUser:             apiUser,
		resources:           resources,
		presence:            presence,
		hub:                 hub,
		multiwatcherFactory: factory,
		controller:          controller,
	}, nil
}

func (c *ControllerAPI) checkIsSuperUser() error {
	isAdmin, err := c.authorizer.HasPermission(permission.SuperuserAccess, c.state.ControllerTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !isAdmin {
		return apiservererrors.ServerError(apiservererrors.ErrPerm)
	}
	return nil
}

// ControllerVersion returns the version information associated with this
// controller binary.
//
// NOTE: the implementation intentionally does not check for SuperuserAccess
// as the Version is known even to users with login access.
func (c *ControllerAPI) ControllerVersion() (params.ControllerVersionResults, error) {
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
func (c *ControllerAPI) IdentityProviderURL() (params.StringResult, error) {
	var result params.StringResult

	cfgRes, err := c.ControllerConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	if cfgRes.Config != nil {
		result.Result = corecontroller.Config(cfgRes.Config).IdentityURL()
	}
	return result, nil
}

// MongoVersion allows the introspection of the mongo version per controller
func (c *ControllerAPI) MongoVersion() (params.StringResult, error) {
	result := params.StringResult{}
	if err := c.checkIsSuperUser(); err != nil {
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
	model *state.Model,
	applicationName string,
) (*params.DashboardConnectionProxy, error) {
	configGetter := stateenvirons.EnvironConfigGetter{Model: model}
	environ, err := common.EnvironFuncForModel(model, configGetter)()
	if err != nil {
		return nil, errors.Trace(err)
	}

	caasBroker, ok := environ.(caas.Broker)
	if !ok {
		return nil, errors.New("cannot get cass environ for model")
	}

	// TODO: (tlm) We are hardcoding port 80 here because the kubernetes
	// dashboard charm doesn't have any config that we can look at to work out
	// what it is. A bug has been raised here: https://github.com/canonical-web-and-design/juju-dashboard-charm/issues/15
	proxier, err := caasBroker.ProxyToApplication(applicationName, "80")
	if err != nil {
		return nil, err
	}

	return params.NewDashboardConnectionProxy(proxier)
}

// dashboardConnectionInforForIAAS returns a dashboard connection for a Juju
// dashboard deployed on IAAS.
func (c *ControllerAPI) dashboardConnectionInfoForIAAS(
	appName string,
	appSettings map[string]interface{},
) (*params.DashboardConnectionSSHTunnel, error) {
	addr, ok := appSettings["dashboard-ingress"]
	if !ok {
		return nil, errors.NotFoundf("dashboard address in relation data")
	}

	dashboardApp, err := c.state.Application(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := dashboardApp.CharmConfig(model.GenerationMaster)
	if err != nil {
		return nil, errors.Trace(err)
	}
	port, ok := cfg["port"]
	if !ok {
		return nil, errors.NotFoundf("dashboard port in charm config")
	}

	return &params.DashboardConnectionSSHTunnel{
		Host: fmt.Sprintf("%s", addr),
		Port: fmt.Sprintf("%d", port),
	}, nil
}

// DashboardConnectionInfo returns the connection information for a client to
// connect to the Juju Dashboard including any proxying information.
func (c *ControllerAPI) DashboardConnectionInfo() (params.DashboardConnectionInfo, error) {
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

			model, _, err := c.statePool.GetModel(rel.ModelUUID())
			if err != nil {
				return rval, errors.Trace(err)
			}

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
					related.ApplicationName,
					appSettings)
				rval.SSHConnection = sshConnection
				return rval, err
			}

			proxyConnection, err := c.dashboardConnectionInfoForCAAS(model, related.ApplicationName)
			rval.ProxyConnection = proxyConnection
			return rval, err
		}

		return rval, errors.NotFoundf("no dashboard found")
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
func (c *ControllerAPI) AllModels() (params.UserModelList, error) {
	result := params.UserModelList{}
	if err := c.checkIsSuperUser(); err != nil {
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
			if errors.IsNotFound(err) {
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

		lastConn, err := model.LastModelConnection(c.apiUser)
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return result, errors.Trace(err)
			}
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
func (c *ControllerAPI) ListBlockedModels() (params.ModelBlockInfoList, error) {
	results := params.ModelBlockInfoList{}
	if err := c.checkIsSuperUser(); err != nil {
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
			logger.Debugf("unable to retrieve model %s: %v", uuid, err)
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

// ModelConfig returns the model config for the controller
// model.  For information on the current model, use
// client.ModelGet
func (c *ControllerAPI) ModelConfig() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	if err := c.checkIsSuperUser(); err != nil {
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
func (c *ControllerAPI) HostedModelConfigs() (params.HostedModelConfigsResults, error) {
	result := params.HostedModelConfigsResults{}
	if err := c.checkIsSuperUser(); err != nil {
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
			if errors.IsNotFound(err) {
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
		modelConf, err := model.Config()
		if err != nil {
			config.Error = apiservererrors.ServerError(err)
		} else {
			config.Config = modelConf.AllAttrs()
		}
		cloudSpec := c.GetCloudSpec(model.ModelTag())
		if config.Error == nil {
			config.CloudSpec = cloudSpec.Result
			config.Error = cloudSpec.Error
		}
		result.Models = append(result.Models, config)
	}

	return result, nil
}

// RemoveBlocks removes all the blocks in the controller.
func (c *ControllerAPI) RemoveBlocks(args params.RemoveBlocksArgs) error {
	if err := c.checkIsSuperUser(); err != nil {
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
func (c *ControllerAPI) WatchAllModels() (params.AllWatcherId, error) {
	if err := c.checkIsSuperUser(); err != nil {
		return params.AllWatcherId{}, errors.Trace(err)
	}
	w := c.multiwatcherFactory.WatchController()
	return params.AllWatcherId{
		AllWatcherId: c.resources.Register(w),
	}, nil
}

// WatchAllModelSummaries starts watching the summary updates from the cache.
// This method is superuser access only, and watches all models in the
// controller.
func (c *ControllerAPI) WatchAllModelSummaries() (params.SummaryWatcherID, error) {
	if err := c.checkIsSuperUser(); err != nil {
		return params.SummaryWatcherID{}, errors.Trace(err)
	}
	w := c.controller.WatchAllModels()
	return params.SummaryWatcherID{
		WatcherID: c.resources.Register(w),
	}, nil
}

// WatchModelSummaries starts watching the summary updates from the cache.
// Only models that the user has access to are returned.
func (c *ControllerAPI) WatchModelSummaries() (params.SummaryWatcherID, error) {
	user := c.apiUser.Id()
	w := c.controller.WatchModelsAsUser(user)
	return params.SummaryWatcherID{
		WatcherID: c.resources.Register(w),
	}, nil
}

// GetControllerAccess returns the level of access the specified users
// have on the controller.
func (c *ControllerAPI) GetControllerAccess(req params.Entities) (params.UserAccessResults, error) {
	results := params.UserAccessResults{}
	isAdmin, err := c.authorizer.HasPermission(permission.SuperuserAccess, c.state.ControllerTag())
	if err != nil {
		return results, errors.Trace(err)
	}

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
		access, err := c.state.UserPermission(userTag, c.state.ControllerTag())
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &params.UserAccess{
			Access:  string(access),
			UserTag: userTag.String()}
	}
	return results, nil
}

// InitiateMigration attempts to begin the migration of one or
// more models to other controllers.
func (c *ControllerAPI) InitiateMigration(reqArgs params.InitiateMigrationArgs) (
	params.InitiateMigrationResults, error,
) {
	out := params.InitiateMigrationResults{
		Results: make([]params.InitiateMigrationResult, len(reqArgs.Specs)),
	}
	if err := c.checkIsSuperUser(); err != nil {
		return out, errors.Trace(err)
	}

	for i, spec := range reqArgs.Specs {
		result := &out.Results[i]
		result.ModelTag = spec.ModelTag
		id, err := c.initiateOneMigration(spec)
		if err != nil {
			result.Error = apiservererrors.ServerError(err)
		} else {
			result.MigrationId = id
		}
	}
	return out, nil
}

func (c *ControllerAPI) initiateOneMigration(spec params.MigrationSpec) (string, error) {
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
	if err := runMigrationPrechecks(
		hostedState.State, systemState,
		&targetInfo, c.presence,
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
func (c *ControllerAPI) ModifyControllerAccess(args params.ModifyControllerAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	hasPermission, err := c.authorizer.HasPermission(permission.SuperuserAccess, c.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Changes {
		if !hasPermission {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		controllerAccess := permission.Access(arg.Access)
		if err := permission.ValidateControllerAccess(controllerAccess); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(errors.Annotate(err, "could not modify controller access"))
			continue
		}

		result.Results[i].Error = apiservererrors.ServerError(
			ChangeControllerAccess(c.state, c.apiUser, targetUserTag, arg.Action, controllerAccess))
	}
	return result, nil
}

// ConfigSet changes the value of specified controller configuration
// settings. Only some settings can be changed after bootstrap.
// Settings that aren't specified in the params are left unchanged.
func (c *ControllerAPI) ConfigSet(args params.ControllerConfigSet) error {
	if err := c.checkIsSuperUser(); err != nil {
		return errors.Trace(err)
	}
	if err := c.state.UpdateControllerConfig(args.Config, nil); err != nil {
		return errors.Trace(err)
	}
	// TODO(thumper): add a version to controller config to allow for
	// simultaneous updates and races in publishing, potentially across
	// HA servers.
	cfg, err := c.state.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := c.hub.Publish(
		controller.ConfigChanged,
		controller.ConfigChangedMessage{cfg}); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// runMigrationPrechecks runs prechecks on the migration and updates
// information in targetInfo as needed based on information
// retrieved from the target controller.
var runMigrationPrechecks = func(
	st, ctlrSt *state.State, targetInfo *coremigration.TargetInfo, presence facade.Presence,
) error {
	// Check model and source controller.
	backend, err := migration.PrecheckShim(st, ctlrSt)
	if err != nil {
		return errors.Annotate(err, "creating backend")
	}
	modelPresence := presence.ModelPresence(st.ModelUUID())
	controllerPresence := presence.ModelPresence(ctlrSt.ModelUUID())

	targetConn, err := api.Open(targetToAPIInfo(targetInfo), migration.ControllerDialOpts())
	if err != nil {
		return errors.Annotate(err, "connect to target controller")
	}
	defer targetConn.Close()

	targetControllerVersion, err := getTargetControllerVersion(targetConn)
	if err != nil {
		return errors.Annotate(err, "cannot get target controller version")
	}

	if err := migration.SourcePrecheck(
		backend,
		targetControllerVersion,
		modelPresence, controllerPresence,
	); err != nil {
		return errors.Annotate(err, "source prechecks failed")
	}

	// Check target controller.
	modelInfo, srcUserList, err := makeModelInfo(st, ctlrSt)
	if err != nil {
		return errors.Trace(err)
	}
	dstUserList, err := getTargetControllerUsers(targetConn)
	if err != nil {
		return errors.Trace(err)
	}
	if err = srcUserList.checkCompatibilityWith(dstUserList); err != nil {
		return errors.Trace(err)
	}
	client := migrationtarget.NewClient(targetConn)
	if targetInfo.CACert == "" {
		targetInfo.CACert, err = client.CACert()
		if err != nil {
			if !params.IsCodeNotImplemented(err) {
				return errors.Annotatef(err, "cannot retrieve CA certificate")
			}
			// If the call's not implemented, it indicates an earlier version
			// of the controller, which we can't migrate to.
			return errors.New("controller API version is too old")
		}
	}
	err = client.Prechecks(modelInfo)
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

func makeModelInfo(st, ctlrSt *state.State) (coremigration.ModelInfo, userList, error) {
	var empty coremigration.ModelInfo
	var ul userList

	model, err := st.Model()
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
	conf, err := model.ModelConfig()
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

	coreConf, err := ctlrSt.ControllerConfig()
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
	}, ul, nil
}

func getTargetControllerVersion(conn api.Connection) (version.Number, error) {
	client := controllerclient.NewClient(conn)
	result, err := client.ControllerVersion()
	if err != nil {
		return version.Number{}, errors.Annotate(err, "failed to obtain target controller version during prechecks")
	}
	number, err := version.Parse(result.Version)
	if err != nil {
		return version.Number{}, errors.Trace(err)
	}
	return number, nil
}

func getTargetControllerUsers(conn api.Connection) (userList, error) {
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
	ul.identityURL, err = ctrlClient.IdentityProviderURL()
	if err != nil {
		return ul, errors.Trace(err)
	}

	return ul, nil
}

func targetToAPIInfo(ti *coremigration.TargetInfo) *api.Info {
	return &api.Info{
		Addrs:     ti.Addrs,
		CACert:    ti.CACert,
		Tag:       ti.AuthTag,
		Password:  ti.Password,
		Macaroons: ti.Macaroons,
	}
}

func grantControllerAccess(accessor ControllerAccess, targetUserTag, apiUser names.UserTag, access permission.Access) error {
	_, err := accessor.AddControllerUser(state.UserAccessSpec{User: targetUserTag, CreatedBy: apiUser, Access: access})
	if errors.IsAlreadyExists(err) {
		controllerTag := accessor.ControllerTag()
		controllerUser, err := accessor.UserAccess(targetUserTag, controllerTag)
		if errors.IsNotFound(err) {
			// Conflicts with prior check, must be inconsistent state.
			err = txn.ErrExcessiveContention
		}
		if err != nil {
			return errors.Annotate(err, "could not look up controller access for user")
		}

		// Only set access if greater access is being granted.
		if controllerUser.Access.EqualOrGreaterControllerAccessThan(access) {
			return errors.Errorf("user already has %q access or greater", access)
		}
		if _, err = accessor.SetUserAccess(controllerUser.UserTag, controllerUser.Object, access); err != nil {
			return errors.Annotate(err, "could not set controller access for user")
		}
		return nil

	}
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func revokeControllerAccess(accessor ControllerAccess, targetUserTag, apiUser names.UserTag, access permission.Access) error {
	controllerTag := accessor.ControllerTag()
	switch access {
	case permission.LoginAccess:
		// Revoking login access removes all access.
		err := accessor.RemoveUserAccess(targetUserTag, controllerTag)
		return errors.Annotate(err, "could not revoke controller access")
	case permission.SuperuserAccess:
		// Revoking superuser sets login.
		controllerUser, err := accessor.UserAccess(targetUserTag, controllerTag)
		if err != nil {
			return errors.Annotate(err, "could not look up controller access for user")
		}
		_, err = accessor.SetUserAccess(controllerUser.UserTag, controllerUser.Object, permission.LoginAccess)
		return errors.Annotate(err, "could not set controller access to login")

	default:
		return errors.Errorf("don't know how to revoke %q access", access)
	}
}

// ChangeControllerAccess performs the requested access grant or revoke action for the
// specified user on the controller.
func ChangeControllerAccess(accessor ControllerAccess, apiUser, targetUserTag names.UserTag, action params.ControllerAction, access permission.Access) error {
	switch action {
	case params.GrantControllerAccess:
		err := grantControllerAccess(accessor, targetUserTag, apiUser, access)
		if err != nil {
			return errors.Annotate(err, "could not grant controller access")
		}
		return nil
	case params.RevokeControllerAccess:
		return revokeControllerAccess(accessor, targetUserTag, apiUser, access)
	default:
		return errors.Errorf("unknown action %q", action)
	}
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
