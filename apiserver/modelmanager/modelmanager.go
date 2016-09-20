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

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.apiserver.modelmanager")

func init() {
	common.RegisterStandardFacade("ModelManager", 2, newFacade)
}

// ModelManager defines the methods on the modelmanager API endpoint.
type ModelManager interface {
	CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error)
	DumpModels(args params.Entities) params.MapResults
	DumpModelsDB(args params.Entities) params.MapResults
	ListModels(user params.Entity) (params.UserModelList, error)
	DestroyModels(args params.Entities) (params.ErrorResults, error)
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	state       common.ModelManagerBackend
	check       *common.BlockChecker
	authorizer  facade.Authorizer
	toolsFinder *common.ToolsFinder
	apiUser     names.UserTag
	isAdmin     bool
}

var _ ModelManager = (*ModelManagerAPI)(nil)

func newFacade(st *state.State, _ facade.Resources, auth facade.Authorizer) (*ModelManagerAPI, error) {
	configGetter := stateenvirons.EnvironConfigGetter{st}
	return NewModelManagerAPI(common.NewModelManagerBackend(st), configGetter, auth)
}

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	st common.ModelManagerBackend,
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
		state:       st,
		check:       common.NewBlockChecker(st),
		authorizer:  authorizer,
		toolsFinder: common.NewToolsFinder(configGetter, st, urlGetter),
		apiUser:     apiUser,
		isAdmin:     isAdmin,
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (m *ModelManagerAPI) authCheck(user names.UserTag) error {
	if m.isAdmin {
		logger.Tracef("%q is a controller admin", m.apiUser.Canonical())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Canonical value of the user tag.
	if m.apiUser.Canonical() == user.Canonical() {
		return nil
	}
	return common.ErrPerm
}

func (s *ModelManagerAPI) hasWriteAccess(modelTag names.ModelTag) (bool, error) {
	canWrite, err := s.authorizer.HasPermission(permission.WriteAccess, modelTag)
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

func (mm *ModelManagerAPI) newModelConfig(
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
	if joint, err = mm.state.ComposeNewModelConfig(joint, regionSpec); err != nil {
		return nil, errors.Trace(err)
	}

	creator := modelmanager.ModelConfigCreator{
		Provider: environs.Provider,
		FindTools: func(n version.Number) (tools.List, error) {
			result, err := mm.toolsFinder.FindTools(params.FindToolsParams{
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
func (mm *ModelManagerAPI) CreateModel(args params.ModelCreateArgs) (params.ModelInfo, error) {
	result := params.ModelInfo{}
	canAddModel, err := mm.authorizer.HasPermission(permission.AddModelAccess, mm.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}
	if !canAddModel {
		return result, common.ErrPerm
	}

	// Get the controller model first. We need it both for the state
	// server owner and the ability to get the config.
	controllerModel, err := mm.state.ControllerModel()
	if err != nil {
		return result, errors.Trace(err)
	}

	ownerTag, err := names.ParseUserTag(args.OwnerTag)
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

	cloud, err := mm.state.Cloud(cloudTag.Id())
	if err != nil {
		if errors.IsNotFound(err) && args.CloudTag != "" {
			// A cloud was specified, and it was not found.
			// Annotate the error with the supported clouds.
			clouds, err := mm.state.Clouds()
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
		if ownerTag.Canonical() == controllerModel.Owner().Canonical() {
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
		credentialValue, err := mm.state.CloudCredential(cloudCredentialTag)
		if err != nil {
			return result, errors.Annotate(err, "getting credential")
		}
		credential = &credentialValue
	}

	cloudSpec, err := environs.MakeCloudSpec(cloud, cloudTag.Id(), cloudRegionName, credential)
	if err != nil {
		return result, errors.Trace(err)
	}

	controllerCfg, err := mm.state.ControllerConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	newConfig, err := mm.newModelConfig(cloudSpec, args, controllerModel)
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
	model, st, err := mm.state.NewModel(state.ModelArgs{
		CloudName:       cloudTag.Id(),
		CloudRegion:     cloudRegionName,
		CloudCredential: cloudCredentialTag,
		Config:          newConfig,
		Owner:           ownerTag,
		StorageProviderRegistry: storageProviderRegistry,
	})
	if err != nil {
		return result, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	return mm.getModelInfo(model.ModelTag())
}

func (mm *ModelManagerAPI) dumpModel(args params.Entity) (map[string]interface{}, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	isModelAdmin, err := mm.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isModelAdmin && !mm.isAdmin {
		return nil, common.ErrPerm
	}

	st := mm.state
	if st.ModelTag() != modelTag {
		st, err = mm.state.ForModel(modelTag)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, errors.Trace(common.ErrBadId)
			}
			return nil, errors.Trace(err)
		}
		defer st.Close()
	}

	bytes, err := migration.ExportModel(st)
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

func (mm *ModelManagerAPI) dumpModelDB(args params.Entity) (map[string]interface{}, error) {
	modelTag, err := names.ParseModelTag(args.Tag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	isModelAdmin, err := mm.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !isModelAdmin && !mm.isAdmin {
		return nil, common.ErrPerm
	}

	st := mm.state
	if st.ModelTag() != modelTag {
		st, err = mm.state.ForModel(modelTag)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, errors.Trace(common.ErrBadId)
			}
			return nil, errors.Trace(err)
		}
		defer st.Close()
	}

	return st.DumpAll()
}

// DumpModels will export the models into the database agnostic
// representation. The user needs to either be a controller admin, or have
// admin privileges on the model itself.
func (mm *ModelManagerAPI) DumpModels(args params.Entities) params.MapResults {
	results := params.MapResults{
		Results: make([]params.MapResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		dumped, err := mm.dumpModel(entity)
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
func (mm *ModelManagerAPI) DumpModelsDB(args params.Entities) params.MapResults {
	results := params.MapResults{
		Results: make([]params.MapResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		dumped, err := mm.dumpModelDB(entity)
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
func (mm *ModelManagerAPI) ListModels(user params.Entity) (params.UserModelList, error) {
	result := params.UserModelList{}

	userTag, err := names.ParseUserTag(user.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}

	err = mm.authCheck(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	models, err := mm.state.ModelsForUser(userTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, model := range models {
		var lastConn *time.Time
		userLastConn, err := model.LastConnection()
		if err != nil {
			if !state.IsNeverConnectedError(err) {
				return result, errors.Trace(err)
			}
		} else {
			lastConn = &userLastConn
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
func (m *ModelManagerAPI) DestroyModels(args params.Entities) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}

	destroyModel := func(tag names.ModelTag) error {
		model, err := m.state.GetModel(tag)
		if err != nil {
			return errors.Trace(err)
		}
		if err := m.authCheck(model.Owner()); err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(common.DestroyModel(m.state, model.ModelTag()))
	}

	for i, arg := range args.Entities {
		tag, err := names.ParseModelTag(arg.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if err := destroyModel(tag); err != nil {
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
	st, err := m.state.ForModel(tag)
	if errors.IsNotFound(err) {
		return params.ModelInfo{}, common.ErrPerm
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	defer st.Close()

	model, err := st.Model()
	if errors.IsNotFound(err) {
		return params.ModelInfo{}, common.ErrPerm
	} else if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	cfg, err := model.Config()
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	users, err := model.Users()
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}
	status, err := model.Status()
	if err != nil {
		return params.ModelInfo{}, errors.Trace(err)
	}

	owner := model.Owner()
	info := params.ModelInfo{
		Name:           cfg.Name(),
		UUID:           cfg.UUID(),
		ControllerUUID: controllerCfg.ControllerUUID(),
		OwnerTag:       owner.String(),
		Life:           params.Life(model.Life().String()),
		Status:         common.EntityStatusFromState(status),
		ProviderType:   cfg.Type(),
		DefaultSeries:  config.PreferredSeries(cfg),
		CloudTag:       names.NewCloudTag(model.Cloud()).String(),
		CloudRegion:    model.CloudRegion(),
	}

	if cloudCredentialTag, ok := model.CloudCredential(); ok {
		info.CloudCredentialTag = cloudCredentialTag.String()
	}

	authorizedOwner := m.authCheck(owner) == nil
	for _, user := range users {
		if !authorizedOwner && m.authCheck(user.UserTag) != nil {
			// The authenticated user is neither the owner
			// nor administrator, nor the model user, so
			// has no business knowing about the model user.
			continue
		}

		userInfo, err := common.ModelUserInfo(user, st)
		if err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
		info.Users = append(info.Users, userInfo)
	}

	if len(info.Users) == 0 {
		// No users, which means the authenticated user doesn't
		// have access to the model.
		return params.ModelInfo{}, common.ErrPerm
	}

	canSeeMachines := authorizedOwner
	if !canSeeMachines {
		if canSeeMachines, err = m.hasWriteAccess(tag); err != nil {
			return params.ModelInfo{}, errors.Trace(err)
		}
	}
	if canSeeMachines {
		if info.Machines, err = common.ModelMachineInfo(st); err != nil {
			return params.ModelInfo{}, err
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
	}
	return result, nil
}

func userAuthorizedToChangeAccess(st common.ModelManagerBackend, userIsAdmin bool, userTag names.UserTag) error {
	if userIsAdmin {
		// Just confirm that the model that has been given is a valid model.
		_, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	// Get the current user's ModelUser for the Model to see if the user has
	// permission to grant or revoke permissions on the model.
	currentUser, err := st.UserAccess(userTag, st.ModelTag())
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
	st, err := accessor.ForModel(modelTag)
	if err != nil {
		return errors.Annotate(err, "could not lookup model")
	}
	defer st.Close()

	if err := userAuthorizedToChangeAccess(st, userIsAdmin, apiUser); err != nil {
		return errors.Trace(err)
	}

	switch action {
	case params.GrantModelAccess:
		_, err = st.AddModelUser(modelTag.Id(), state.UserAccessSpec{User: targetUserTag, CreatedBy: apiUser, Access: access})
		if errors.IsAlreadyExists(err) {
			modelUser, err := st.UserAccess(targetUserTag, modelTag)
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
			if _, err = st.SetUserAccess(modelUser.UserTag, modelUser.Object, access); err != nil {
				return errors.Annotate(err, "could not set model access for user")
			}
			return nil
		}
		return errors.Annotate(err, "could not grant model access")

	case params.RevokeModelAccess:
		switch access {
		case permission.ReadAccess:
			// Revoking read access removes all access.
			err := st.RemoveUserAccess(targetUserTag, modelTag)
			return errors.Annotate(err, "could not revoke model access")
		case permission.WriteAccess:
			// Revoking write access sets read-only.
			modelUser, err := st.UserAccess(targetUserTag, modelTag)
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}
			_, err = st.SetUserAccess(modelUser.UserTag, modelUser.Object, permission.ReadAccess)
			return errors.Annotate(err, "could not set model access to read-only")
		case permission.AdminAccess:
			// Revoking admin access sets read-write.
			modelUser, err := st.UserAccess(targetUserTag, modelTag)
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}
			_, err = st.SetUserAccess(modelUser.UserTag, modelUser.Object, permission.WriteAccess)
			return errors.Annotate(err, "could not set model access to read-write")

		default:
			return errors.Errorf("don't know how to revoke %q access", access)
		}

	default:
		return errors.Errorf("unknown action %q", action)
	}
}

// ModelDefaults returns the default config values used when creating a new model.
func (c *ModelManagerAPI) ModelDefaults() (params.ModelDefaultsResult, error) {
	result := params.ModelDefaultsResult{}
	if !c.isAdmin {
		return result, common.ErrPerm
	}

	values, err := c.state.ModelConfigDefaultValues()
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
func (c *ModelManagerAPI) SetModelDefaults(args params.SetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Config))}
	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Config {
		// TODO(wallyworld) - use arg.Cloud and arg.CloudRegion as appropriate
		results.Results[i].Error = common.ServerError(
			c.setModelDefaults(arg),
		)
	}
	return results, nil
}

func (c *ModelManagerAPI) setModelDefaults(args params.ModelDefaultValues) error {
	if !c.isAdmin {
		return common.ErrPerm
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return errors.Trace(err)
	}
	// Make sure we don't allow changing agent-version.
	if _, found := args.Config["agent-version"]; found {
		return errors.New("agent-version cannot have a default value")
	}
	return c.state.UpdateModelConfigDefaultValues(args.Config, nil)
}

// UnsetModelDefaults removes the specified default model settings.
func (c *ModelManagerAPI) UnsetModelDefaults(args params.UnsetModelDefaults) (params.ErrorResults, error) {
	results := params.ErrorResults{Results: make([]params.ErrorResult, len(args.Keys))}
	if !c.isAdmin {
		return results, common.ErrPerm
	}

	if err := c.check.ChangeAllowed(); err != nil {
		return results, errors.Trace(err)
	}
	for i, arg := range args.Keys {
		// TODO(wallyworld) - use arg.Cloud and arg.CloudRegion as appropriate
		results.Results[i].Error = common.ServerError(
			c.state.UpdateModelConfigDefaultValues(nil, arg.Keys),
		)
	}
	return results, nil
}
