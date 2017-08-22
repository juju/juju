// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelmanager defines an API end point for functions dealing with
// models.  Creating, listing and sharing models. This facade is available at
// the root of the controller API, and as such, there is no implicit Model
// assocated.
package modelmanager

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/description"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.apiserver.modelmanager")

// ModelManagerV4 defines the methods on the version 2 facade for the
// modelmanager API endpoint.
type ModelManagerV4 interface {
	CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error)
	DumpModels(args params.DumpModelRequest) params.StringResults
	DumpModelsDB(args params.Entities) params.MapResults
	ListModels(user params.Entity) (params.UserModelList, error)
	DestroyModels(args params.DestroyModelsParams) (params.ErrorResults, error)
}

// ModelManagerV3 defines the methods on the version 2 facade for the
// modelmanager API endpoint.
type ModelManagerV3 interface {
	CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error)
	DumpModels(args params.DumpModelRequest) params.StringResults
	DumpModelsDB(args params.Entities) params.MapResults
	ListModels(user params.Entity) (params.UserModelList, error)
	DestroyModels(args params.Entities) (params.ErrorResults, error)
}

// ModelManagerV2 defines the methods on the version 2 facade for the
// modelmanager API endpoint.
type ModelManagerV2 interface {
	CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error)
	DumpModels(args params.Entities) params.MapResults
	DumpModelsDB(args params.Entities) params.MapResults
	ListModels(user params.Entity) (params.UserModelList, error)
	DestroyModels(args params.Entities) (params.ErrorResults, error)
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	*common.ModelStatusAPI
	state       common.ModelManagerBackend
	ctlrState   common.ModelManagerBackend
	check       *common.BlockChecker
	authorizer  facade.Authorizer
	toolsFinder *common.ToolsFinder
	apiUser     names.UserTag
	isAdmin     bool
}

// ModelManagerAPIV2 provides a way to wrap the different calls between
// version 3 and version 4 of the model manager API
type ModelManagerAPIV3 struct {
	*ModelManagerAPI
}

// ModelManagerAPIV2 provides a way to wrap the different calls between
// version 2 and version 3 of the model manager API
type ModelManagerAPIV2 struct {
	*ModelManagerAPIV3
}

var (
	_ ModelManagerV4 = (*ModelManagerAPI)(nil)
	_ ModelManagerV3 = (*ModelManagerAPIV3)(nil)
	_ ModelManagerV2 = (*ModelManagerAPIV2)(nil)
)

// NewFacadeV4 is used for API registration.
func NewFacadeV4(ctx facade.Context) (*ModelManagerAPI, error) {
	st := ctx.State()
	pool := ctx.StatePool()
	ctlrSt := pool.SystemState()
	auth := ctx.Auth()
	configGetter := stateenvirons.EnvironConfigGetter{st}

	return NewModelManagerAPI(
		common.NewModelManagerBackend(st, pool),
		common.NewModelManagerBackend(ctlrSt, pool),
		configGetter,
		auth,
	)
}

// NewFacadeV3 is used for API registration.
func NewFacadeV3(ctx facade.Context) (*ModelManagerAPIV3, error) {
	v4, err := NewFacadeV4(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV3{v4}, nil
}

// NewFacade is used for API registration.
func NewFacadeV2(ctx facade.Context) (*ModelManagerAPIV2, error) {
	v3, err := NewFacadeV3(ctx)
	if err != nil {
		return nil, err
	}
	return &ModelManagerAPIV2{v3}, nil
}

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	st common.ModelManagerBackend,
	ctlrSt common.ModelManagerBackend,
	configGetter environs.EnvironConfigGetter,
	authorizer facade.Authorizer,
) (*ModelManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), st)
	return &ModelManagerAPI{
		ModelStatusAPI: common.NewModelStatusAPI(st, authorizer, apiUser),
		state:          st,
		ctlrState:      ctlrSt,
		check:          common.NewBlockChecker(st),
		authorizer:     authorizer,
		toolsFinder:    common.NewToolsFinder(configGetter, st, urlGetter),
		apiUser:        apiUser,
		isAdmin:        isAdmin,
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (m *ModelManagerAPI) authCheck(user names.UserTag) error {
	if m.isAdmin {
		logger.Tracef("%q is a controller admin", m.apiUser.Id())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Canonical value of the user tag.
	if m.apiUser == user {
		return nil
	}
	return common.ErrPerm
}

func (m *ModelManagerAPI) hasWriteAccess(modelTag names.ModelTag) (bool, error) {
	canWrite, err := m.authorizer.HasPermission(permission.WriteAccess, modelTag)
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

func (m *ModelManagerAPI) newModelConfig(
	cloudSpec environs.CloudSpec,
	args params.ModelCreateArgs,
	source ConfigSource,
) (*config.Config, error) {
	// For now, we just smash to the two maps together as we store
	// the account values and the model config together in the
	// *config.Config instance.
	joint := make(map[string]interface{})
	for key, value := range args.Config {
		joint[key] = value
	}
	if _, ok := joint["uuid"]; ok {
		return nil, errors.New("uuid is generated, you cannot specify one")
	}
	if args.Name == "" {
		return nil, errors.NewNotValid(nil, "Name must be specified")
	}
	if _, ok := joint[config.NameKey]; ok {
		return nil, errors.New("name must not be specified in config")
	}
	joint[config.NameKey] = args.Name

	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	regionSpec := &environs.RegionSpec{Cloud: cloudSpec.Name, Region: cloudSpec.Region}
	if joint, err = m.state.ComposeNewModelConfig(joint, regionSpec); err != nil {
		return nil, errors.Trace(err)
	}

	creator := modelmanager.ModelConfigCreator{
		Provider: environs.Provider,
		FindTools: func(n version.Number) (tools.List, error) {
			result, err := m.toolsFinder.FindTools(params.FindToolsParams{
				Number: n,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return result.List, nil
		},
	}
	return creator.NewModelConfig(cloudSpec, baseConfig, joint)
}

// CreateModel creates a new model using the account and
// model config specified in the args.
func (m *ModelManagerAPI) CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error) {
	result := params.ModelInfo{}
	canAddModel, err := m.authorizer.HasPermission(permission.AddModelAccess, m.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	if !canAddModel {
		return result, common.ErrPerm
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	// a special case of ErrPerm will happen if the user has add-model permission but is trying to
	// create a model for another person, which is not yet supported.
	if !m.isAdmin && ownerTag != m.apiUser {
		return result, errors.Annotatef(common.ErrPerm, "%q permission does not permit creation of models for different owners", permission.AddModelAccess)
	}

	// Get the controller model first. We need it both for the state
	// server owner and the ability to get the config.
	controllerModel, err := m.ctlrState.Model()
	if err != nil {
		return result, errors.Trace(err)
	}

	var cloudTag names.CloudTag
	cloudRegionName := args.CloudRegion
	if args.CloudTag != "" {
		var err error
		cloudTag, err = names.ParseCloudTag(args.CloudTag)
		if err != nil {
			return result, errors.Trace(err)
		}
	} else {
		cloudTag = names.NewCloudTag(controllerModel.Cloud())
	}
	if cloudRegionName == "" && cloudTag.Id() == controllerModel.Cloud() {
		cloudRegionName = controllerModel.CloudRegion()
	}

	cloud, err := m.state.Cloud(cloudTag.Id())
	if err != nil {
		if errors.IsNotFound(err) && args.CloudTag != "" {
			// A cloud was specified, and it was not found.
			// Annotate the error with the supported clouds.
			clouds, err := m.state.Clouds()
			if err != nil {
				return result, errors.Trace(err)
			}
			cloudNames := make([]string, 0, len(clouds))
			for tag := range clouds {
				cloudNames = append(cloudNames, tag.Id())
			}
			sort.Strings(cloudNames)
			return result, errors.NewNotFound(err, fmt.Sprintf(
				"cloud %q not found, expected one of %q",
				cloudTag.Id(), cloudNames,
			))
		}
		return result, errors.Annotate(err, "getting cloud definition")
	}

	var cloudCredentialTag names.CloudCredentialTag
	if args.CloudCredentialTag != "" {
		var err error
		cloudCredentialTag, err = names.ParseCloudCredentialTag(args.CloudCredentialTag)
		if err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if ownerTag == controllerModel.Owner() {
			cloudCredentialTag, _ = controllerModel.CloudCredential()
		} else {
			// TODO(axw) check if the user has one and only one
			// cloud credential, and if so, use it? For now, we
			// require the user to specify a credential unless
			// the cloud does not require one.
			var hasEmpty bool
			for _, authType := range cloud.AuthTypes {
				if authType != jujucloud.EmptyAuthType {
					continue
				}
				hasEmpty = true
				break
			}
			if !hasEmpty {
				return result, errors.NewNotValid(nil, "no credential specified")
			}
		}
	}

	var credential *jujucloud.Credential
	if cloudCredentialTag != (names.CloudCredentialTag{}) {
		credentialValue, err := m.state.CloudCredential(cloudCredentialTag)
		if err != nil {
			return result, errors.Annotate(err, "getting credential")
		}
		credential = &credentialValue
	}

	cloudSpec, err := environs.MakeCloudSpec(cloud, cloudRegionName, credential)
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerCfg, err := m.state.ControllerConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	newConfig, err := m.newModelConfig(cloudSpec, args, controllerModel)
	if err != nil {
		return result, errors.Annotate(err, "failed to create config")
	}

	// Create the Environ.
	env, err := environs.New(environs.OpenParams{
		Cloud:  cloudSpec,
		Config: newConfig,
	})
	if err != nil {
		return result, errors.Annotate(err, "failed to open environ")
	}
	if err := env.Create(environs.CreateParams{
		ControllerUUID: controllerCfg.ControllerUUID(),
	}); err != nil {
		return result, errors.Annotate(err, "failed to create environ")
	}
	storageProviderRegistry := stateenvirons.NewStorageProviderRegistry(env)

	// NOTE: check the agent-version of the config, and if it is > the current
	// version, it is not supported, also check existing tools, and if we don't
	// have tools for that version, also die.
	model, st, err := m.state.NewModel(state.ModelArgs{
		Type:            state.ModelTypeIAAS,
		CloudName:       cloudTag.Id(),
		CloudRegion:     cloudRegionName,
		CloudCredential: cloudCredentialTag,
		Config:          newConfig,
		Owner:           ownerTag,
		StorageProviderRegistry: storageProviderRegistry,
		EnvironVersion:          env.Provider().Version(),
	})
	if err != nil {
		return result, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	if err = st.ReloadSpaces(env); err != nil {
		if errors.IsNotSupported(err) {
			logger.Debugf("Not performing spaces load on a non-networking environment")
		} else {
			return result, errors.Annotate(err, "Failed to perform spaces discovery")
		}
	}

	return m.getModelInfo(model.ModelTag())
}

func (m *ModelManagerAPI) dumpModel(args params.Entity, simplified bool) ([]byte, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	isModelAdmin, err := m.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isModelAdmin && !m.isAdmin {
		return nil, common.ErrPerm
	}

	st, release, err := m.state.GetBackend(modelTag.Id())
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.Trace(common.ErrBadId)
		}
		return nil, errors.Trace(err)
	}
	defer release()

	var exportConfig state.ExportConfig
	if simplified {
		exportConfig.SkipActions = true
		exportConfig.SkipAnnotations = true
		exportConfig.SkipCloudImageMetadata = true
		exportConfig.SkipCredentials = true
		exportConfig.SkipIPAddresses = true
		exportConfig.SkipSettings = true
		exportConfig.SkipSSHHostKeys = true
		exportConfig.SkipStatusHistory = true
		exportConfig.SkipLinkLayerDevices = true
	}

	model, err := st.ExportPartial(exportConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	bytes, err := description.Serialize(model)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes, nil
}

func (m *ModelManagerAPIV2) dumpModel(args params.Entity) (map[string]interface{}, error) {
	bytes, err := m.ModelManagerAPI.dumpModel(args, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Now read it back into a map.
	var asMap map[string]interface{}
	err = yaml.Unmarshal(bytes, &asMap)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// In order to serialize the map through JSON, we need to make sure
	// that all the embedded maps are map[string]interface{}, not
	// map[interface{}]interface{} which is what YAML gives by default.
	out, err := utils.ConformYAML(asMap)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.(map[string]interface{}), nil
}

func (m *ModelManagerAPI) dumpModelDB(args params.Entity) (map[string]interface{}, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	isModelAdmin, err := m.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isModelAdmin && !m.isAdmin {
		return nil, common.ErrPerm
	}

	st := m.state
	if st.ModelTag() != modelTag {
		newSt, release, err := m.state.GetBackend(modelTag.Id())
		if errors.IsNotFound(err) {
			return nil, errors.Trace(common.ErrBadId)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		defer release()
		st = newSt
	}

	return st.DumpAll()
}

// DumpModels will export the models into the database agnostic
// representation. The user needs to either be a controller admin, or have
// admin privileges on the model itself.
func (m *ModelManagerAPI) DumpModels(args params.DumpModelRequest) params.StringResults {
	results := params.StringResults{
		Results: make([]params.StringResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		bytes, err := m.dumpModel(entity, args.Simplified)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		// We know here that the bytes are valid YAML.
		results.Results[i].Result = string(bytes)
	}
	return results
}

// DumpModels will export the models into the database agnostic
// representation. The user needs to either be a controller admin, or have
// admin privileges on the model itself.
func (m *ModelManagerAPIV2) DumpModels(args params.Entities) params.MapResults {
	results := params.MapResults{
		Results: make([]params.MapResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		dumped, err := m.dumpModel(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = dumped
	}
	return results
}

// DumpModelsDB will gather all documents from all model collections
// for the specified model. The map result contains a map of collection
// names to lists of documents represented as maps.
func (m *ModelManagerAPI) DumpModelsDB(args params.Entities) params.MapResults {
	results := params.MapResults{
		Results: make([]params.MapResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		dumped, err := m.dumpModelDB(entity)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = dumped
	}
	return results
}

// ListModels returns the models that the specified user
// has access to in the current server.  Only that controller owner
// can list models for any user (at this stage).  Other users
// can only ask about their own models.
func (m *ModelManagerAPI) ListModels(user params.Entity) (params.UserModelList, error) {
	result := params.UserModelList{}

	userTag, err := names.ParseUserTag(user.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = m.authCheck(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	model, err := m.state.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelUUIDs, err := model.ModelUUIDsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, modelUUID := range modelUUIDs {
		st, release, err := m.state.GetBackend(modelUUID)
		if err != nil {
			return result, errors.Trace(err)
		}
		defer release()

		var lastConn *time.Time
		userLastConn, err := model.LastModelConnection(userTag)
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return result, errors.Trace(err)
			}
		} else {
			lastConn = &userLastConn
		}

		model, err := st.Model()
		if err != nil {
			return result, errors.Trace(err)
		}

		result.UserModels = append(result.UserModels, params.UserModel{
			Model: params.Model{
				Name:     model.Name(),
				UUID:     model.UUID(),
				OwnerTag: model.Owner().String(),
			},
			LastConnection: lastConn,
		})
	}

	return result, nil
}

// DestroyModels will try to destroy the specified models.
// If there is a block on destruction, this method will return an error.
func (m *ModelManagerAPIV3) DestroyModels(args params.Entities) (params.ErrorResults, error) {
	// v3 DestroyModels is implemented in terms of v4:
	// storage is unconditionally destroyed, as was the
	// old behaviour.
	destroyStorage := true
	v4Args := params.DestroyModelsParams{
		Models: make([]params.DestroyModelParams, len(args.Entities)),
	}
	for i, arg := range args.Entities {
		v4Args.Models[i] = params.DestroyModelParams{
			ModelTag:       arg.Tag,
			DestroyStorage: &destroyStorage,
		}
	}
	return m.ModelManagerAPI.DestroyModels(v4Args)
}

// DestroyModels will try to destroy the specified models.
// If there is a block on destruction, this method will return an error.
func (m *ModelManagerAPI) DestroyModels(args params.DestroyModelsParams) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Models)),
	}

	destroyModel := func(modelUUID string, destroyStorage *bool) error {
		model, releaseModel, err := m.state.GetModel(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer releaseModel()
		if err := m.authCheck(model.Owner()); err != nil {
			return errors.Trace(err)
		}

		st, releaseSt, err := m.state.GetBackend(modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer releaseSt()

		return errors.Trace(common.DestroyModel(st, destroyStorage))
	}

	for i, arg := range args.Models {
		tag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := destroyModel(tag.Id(), arg.DestroyStorage); err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
	}
	return results, nil
}

// ModelInfo returns information about the specified models.
func (m *ModelManagerAPI) ModelInfo(args params.Entities) (params.ModelInfoResults, error) {
	results := params.ModelInfoResults{
		Results: make([]params.ModelInfoResult, len(args.Entities)),
	}

	getModelInfo := func(arg params.Entity) (params.ModelInfo, error) {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
		return m.getModelInfo(tag)
	}

	for i, arg := range args.Entities {
		modelInfo, err := getModelInfo(arg)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = &modelInfo
	}
	return results, nil
}

func (m *ModelManagerAPI) getModelInfo(tag names.ModelTag) (params.ModelInfo, error) {
	st, release, err := m.state.GetBackend(tag.Id())
	if errors.IsNotFound(err) {
		return params.ModelInfo{}, errors.Trace(common.ErrPerm)
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	defer release()

	model, err := st.Model()
	if errors.IsNotFound(err) {
		return params.ModelInfo{}, errors.Trace(common.ErrPerm)
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	owner := model.Owner()
	info := params.ModelInfo{
		Name:           model.Name(),
		UUID:           model.UUID(),
		ControllerUUID: model.ControllerUUID(),
		OwnerTag:       owner.String(),
		Life:           params.Life(model.Life().String()),
		CloudTag:       names.NewCloudTag(model.Cloud()).String(),
		CloudRegion:    model.CloudRegion(),
	}

	if cloudCredentialTag, ok := model.CloudCredential(); ok {
		info.CloudCredentialTag = cloudCredentialTag.String()
	}

	// All users with access to the model can see the SLA information.
	info.SLA = &params.ModelSLAInfo{
		Level: model.SLALevel(),
		Owner: model.SLAOwner(),
	}

	// If model is not alive - dying or dead - or if it is being imported,
	// there is no guarantee that the rest of the call will succeed.
	// For these models we can ignore NotFound errors coming from persistence layer.
	// However, for Alive models, these errors are genuine and cannot be ignored.
	ignoreNotFoundError := model.Life() != state.Alive || model.MigrationMode() == state.MigrationModeImporting

	// If we received an an error and cannot ignore it, we should consider it fatal and surface it.
	// We should do the same if we can ignore NotFound errors but the given error is of some other type.
	shouldErr := func(thisErr error) bool {
		if thisErr == nil {
			return false
		}
		return !ignoreNotFoundError || !errors.IsNotFound(thisErr)
	}
	cfg, err := model.Config()
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		info.ProviderType = cfg.Type()
		info.DefaultSeries = config.PreferredSeries(cfg)
		if agentVersion, exists := cfg.AgentVersion(); exists {
			info.AgentVersion = &agentVersion
		}
	}

	status, err := model.Status()
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		entityStatus := common.EntityStatusFromState(status)
		info.Status = entityStatus
	}

	authorizedOwner := m.authCheck(owner) == nil

	users, err := model.Users()
	if shouldErr(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		for _, user := range users {
			if !authorizedOwner && m.authCheck(user.UserTag) != nil {
				// The authenticated user is neither the owner
				// nor administrator, nor the model user, so
				// has no business knowing about the model user.
				continue
			}

			userInfo, err := common.ModelUserInfo(user, model)
			if err != nil {
				return params.ModelInfo{}, errors.Trace(err)
			}
			info.Users = append(info.Users, userInfo)
		}

		if len(info.Users) == 0 {
			// No users, which means the authenticated user doesn't
			// have access to the model.
			return params.ModelInfo{}, errors.Trace(common.ErrPerm)
		}
	}

	canSeeMachines := authorizedOwner
	if !canSeeMachines {
		if canSeeMachines, err = m.hasWriteAccess(tag); err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
	}
	if canSeeMachines {
		if info.Machines, err = common.ModelMachineInfo(st); shouldErr(err) {
			return params.ModelInfo{}, err
		}
	}

	migration, err := st.LatestMigration()
	if err != nil && !errors.IsNotFound(err) {
		return params.ModelInfo{}, errors.Trace(err)
	}
	if err == nil {
		startTime := migration.StartTime()
		endTime := new(time.Time)
		*endTime = migration.EndTime()
		var zero time.Time
		if *endTime == zero {
			endTime = nil
		}
		info.Migration = &params.ModelMigrationStatus{
			Status: migration.StatusMessage(),
			Start:  &startTime,
			End:    endTime,
		}
	}
	return info, nil
}

// ModifyModelAccess changes the model access granted to users.
func (m *ModelManagerAPI) ModifyModelAccess(args params.ModifyModelAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}

	canModifyController, err := m.authorizer.HasPermission(permission.SuperuserAccess, m.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		modelAccess := permission.Access(arg.Access)
		if err := permission.ValidateModelAccess(modelAccess); err != nil {
			err = errors.Annotate(err, "could not modify model access")
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		modelTag, err := names.ParseModelTag(arg.ModelTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}
		canModifyModel, err := m.authorizer.HasPermission(permission.AdminAccess, modelTag)
		if err != nil {
			return result, errors.Trace(err)
		}
		canModify := canModifyController || canModifyModel

		if !canModify {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}

		result.Results[i].Error = common.ServerError(
			changeModelAccess(m.state, modelTag, m.apiUser, targetUserTag, arg.Action, modelAccess, m.isAdmin))
		// TODO MMCC: should we be passing in the model instead of m.state?
	}
	return result, nil
}

func userAuthorizedToChangeAccess(st common.ModelManagerBackend, userIsAdmin bool, userTag names.UserTag) error {
	model, err := st.Model()
	if userIsAdmin {
		// Just confirm that the model that has been given is a valid model.
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	} else {
		if err != nil {
			return common.ErrPerm
		}
	}

	// Get the current user's ModelUser for the Model to see if the user has
	// permission to grant or revoke permissions on the model.
	currentUser, err := model.UserAccess(userTag, st.ModelTag())
	if err != nil {
		if errors.IsNotFound(err) {
			// No, this user doesn't have permission.
			return common.ErrPerm
		}
		return errors.Annotate(err, "could not retrieve user")
	}
	if currentUser.Access != permission.AdminAccess {
		return common.ErrPerm
	}
	return nil
}

// changeModelAccess performs the requested access grant or revoke action for the
// specified user on the specified model.
func changeModelAccess(accessor common.ModelManagerBackend, modelTag names.ModelTag, apiUser, targetUserTag names.UserTag, action params.ModelAction, access permission.Access, userIsAdmin bool) error {
	st, release, err := accessor.GetBackend(modelTag.Id())
	if err != nil {
		return errors.Annotate(err, "could not lookup model")
	}
	defer release()

	if err := userAuthorizedToChangeAccess(st, userIsAdmin, apiUser); err != nil {
		return errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	switch action {
	case params.GrantModelAccess:
		_, err = model.AddModelUser(modelTag.Id(), state.UserAccessSpec{User: targetUserTag, CreatedBy: apiUser, Access: access})
		if errors.IsAlreadyExists(err) {
			modelUser, err := model.UserAccess(targetUserTag, modelTag)
			if errors.IsNotFound(err) {
				// Conflicts with prior check, must be inconsistent state.
				err = txn.ErrExcessiveContention
			}
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}

			// Only set access if greater access is being granted.
			if modelUser.Access.EqualOrGreaterModelAccessThan(access) {
				return errors.Errorf("user already has %q access or greater", access)
			}
			if _, err = model.SetUserAccess(modelUser.UserTag, modelUser.Object, access); err != nil {
				return errors.Annotate(err, "could not set model access for user")
			}
			return nil
		}
		return errors.Annotate(err, "could not grant model access")

	case params.RevokeModelAccess:
		switch access {
		case permission.ReadAccess:
			// Revoking read access removes all access.
			err := model.RemoveUserAccess(targetUserTag, modelTag)
			return errors.Annotate(err, "could not revoke model access")
		case permission.WriteAccess:
			// Revoking write access sets read-only.
			modelUser, err := model.UserAccess(targetUserTag, modelTag)
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}
			_, err = model.SetUserAccess(modelUser.UserTag, modelUser.Object, permission.ReadAccess)
			return errors.Annotate(err, "could not set model access to read-only")
		case permission.AdminAccess:
			// Revoking admin access sets read-write.
			modelUser, err := model.UserAccess(targetUserTag, modelTag)
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}
			_, err = model.SetUserAccess(modelUser.UserTag, modelUser.Object, permission.WriteAccess)
			return errors.Annotate(err, "could not set model access to read-write")

		default:
			return errors.Errorf("don't know how to revoke %q access", access)
		}

	default:
		return errors.Errorf("unknown action %q", action)
	}
}

// ModelDefaults returns the default config values used when creating a new model.
func (m *ModelManagerAPI) ModelDefaults() (params.ModelDefaultsResult, error) {
	result := params.ModelDefaultsResult{}
	if !m.isAdmin {
		return result, common.ErrPerm
	}

	values, err := m.state.ModelConfigDefaultValues()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Config = make(map[string]params.ModelDefaults)
	for attr, val := range values {
		settings := params.ModelDefaults{
			Controller: val.Controller,
			Default:    val.Default,
		}
		for _, v := range val.Regions {
			settings.Regions = append(
				settings.Regions, params.RegionDefaults{
					RegionName: v.Name,
					Value:      v.Value})
		}
		result.Config[attr] = settings
	}
	return result, nil
}

// SetModelDefaults writes new values for the specified default model settings.
func (m *ModelManagerAPI) SetModelDefaults(args params.SetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Config))}
	if err := m.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Config {
		results.Results[i].Error = common.ServerError(
			m.setModelDefaults(arg),
		)
	}
	return results, nil
}

func (m *ModelManagerAPI) setModelDefaults(args params.ModelDefaultValues) error {
	if !m.isAdmin {
		return common.ErrPerm
	}

	if err := m.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	if _, found := args.Config["agent-version"]; found {
		return errors.New("agent-version cannot have a default value")
	}

	var rspec *environs.RegionSpec
	if args.CloudRegion != "" {
		spec, err := m.makeRegionSpec(args.CloudTag, args.CloudRegion)
		if err != nil {
			return errors.Trace(err)
		}
		rspec = spec
	}
	return m.state.UpdateModelConfigDefaultValues(args.Config, nil, rspec)
}

// UnsetModelDefaults removes the specified default model settings.
func (m *ModelManagerAPI) UnsetModelDefaults(args params.UnsetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Keys))}
	if !m.isAdmin {
		return results, common.ErrPerm
	}

	if err := m.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}

	for i, arg := range args.Keys {
		var rspec *environs.RegionSpec
		if arg.CloudRegion != "" {
			spec, err := m.makeRegionSpec(arg.CloudTag, arg.CloudRegion)
			if err != nil {
				results.Results[i].Error = common.ServerError(
					errors.Trace(err))
				continue
			}
			rspec = spec
		}
		results.Results[i].Error = common.ServerError(
			m.state.UpdateModelConfigDefaultValues(nil, arg.Keys, rspec),
		)
	}
	return results, nil
}

// makeRegionSpec is a helper method for methods that call
// state.UpdateModelConfigDefaultValues.
func (m *ModelManagerAPI) makeRegionSpec(cloudTag, r string) (*environs.RegionSpec, error) {
	cTag, err := names.ParseCloudTag(cloudTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rspec, err := environs.NewRegionSpec(cTag.Id(), r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rspec, nil
}
