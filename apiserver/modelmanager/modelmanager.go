// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modelmanager defines an API end point for functions
// dealing with models.  Creating, listing and sharing models.
package modelmanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/txn"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/controller/modelmanager"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/juju/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/tools"
)

var logger = loggo.GetLogger("juju.apiserver.modelmanager")

func init() {
	common.RegisterStandardFacade("ModelManager", 2, NewModelManagerAPI)
}

// ModelManager defines the methods on the modelmanager API end
// point.
type ModelManager interface {
	ConfigSkeleton(args params.ModelSkeletonConfigArgs) (params.ModelConfigResult, error)
	CreateModel(args params.ModelCreateArgs) (params.Model, error)
	ListModels(user params.Entity) (params.UserModelList, error)
}

// ModelManagerAPI implements the model manager interface and is
// the concrete implementation of the api end point.
type ModelManagerAPI struct {
	state       stateInterface
	authorizer  common.Authorizer
	toolsFinder *common.ToolsFinder
	isAdmin     bool
}

var _ ModelManager = (*ModelManagerAPI)(nil)

// NewModelManagerAPI creates a new api server endpoint for managing
// models.
func NewModelManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ModelManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), st)
	return &ModelManagerAPI{
		state:       getState(st),
		authorizer:  authorizer,
		toolsFinder: common.NewToolsFinder(st, st, urlGetter),
	}, nil
}

// authCheck checks if the user is acting on their own behalf, or if they
// are an administrator acting on behalf of another user.
func (mm *ModelManagerAPI) authCheck(user names.UserTag) error {
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := mm.authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := mm.state.IsControllerAdministrator(apiUser)
	if err != nil {
		return errors.Trace(err)
	}
	mm.isAdmin = isAdmin
	if isAdmin {
		logger.Tracef("%q is a controller admin", apiUser.Canonical())
		return nil
	}

	// We can't just compare the UserTags themselves as the provider part
	// may be unset, and gets replaced with 'local'. We must compare against
	// the Username of the user tag.
	if apiUser.Canonical() == user.Canonical() {
		return nil
	}
	return common.ErrPerm
}

// ConfigSource describes a type that is able to provide config.
// Abstracted primarily for testing.
type ConfigSource interface {
	Config() (*config.Config, error)
}

// ConfigSkeleton returns config values to be used as a starting point for the
// API caller to construct a valid model specific config.  The provider
// and region params are there for future use, and current behaviour expects
// both of these to be empty.
func (mm *ModelManagerAPI) ConfigSkeleton(args params.ModelSkeletonConfigArgs) (params.ModelConfigResult, error) {
	var result params.ModelConfigResult
	if args.Region != "" {
		return result, errors.NotValidf("region value %q", args.Region)
	}

	controllerEnv, err := mm.state.ControllerModel()
	if err != nil {
		return result, errors.Trace(err)
	}
	config, err := mm.configSkeleton(controllerEnv, args.Provider)
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config
	return result, nil
}

func (mm *ModelManagerAPI) configSkeleton(source ConfigSource, requestedProviderType string) (map[string]interface{}, error) {
	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if requestedProviderType != "" && baseConfig.Type() != requestedProviderType {
		return nil, errors.Errorf(
			"cannot create new model with credentials for provider type %q on controller with provider type %q",
			requestedProviderType, baseConfig.Type())
	}
	baseMap := baseConfig.AllAttrs()

	fields, err := modelmanager.RestrictedProviderFields(baseConfig.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result = make(map[string]interface{})
	for _, field := range fields {
		if value, found := baseMap[field]; found {
			result[field] = value
		}
	}
	return result, nil
}

func (mm *ModelManagerAPI) newModelConfig(args params.ModelCreateArgs, source ConfigSource) (*config.Config, error) {
	// For now, we just smash to the two maps together as we store
	// the account values and the model config together in the
	// *config.Config instance.
	joint := make(map[string]interface{})
	for key, value := range args.Config {
		joint[key] = value
	}
	// Account info overrides any config values.
	for key, value := range args.Account {
		joint[key] = value
	}
	if _, ok := joint["uuid"]; ok {
		return nil, errors.New("uuid is generated, you cannot specify one")
	}
	baseConfig, err := source.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}
	creator := modelmanager.ModelConfigCreator{
		func(n version.Number) (tools.List, error) {
			result, err := mm.toolsFinder.FindTools(params.FindToolsParams{
				Number: n,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return result.List, nil
		},
	}
	return creator.NewModelConfig(mm.isAdmin, baseConfig, joint)
}

// CreateModel creates a new model using the account and
// model config specified in the args.
func (mm *ModelManagerAPI) CreateModel(args params.ModelCreateArgs) (params.Model, error) {
	result := params.Model{}
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

	// Any user is able to create themselves an model (until real fine
	// grain permissions are available), and admins (the creator of the state
	// server model) are able to create models for other people.
	err = mm.authCheck(ownerTag)
	if err != nil {
		return result, errors.Trace(err)
	}

	newConfig, err := mm.newModelConfig(args, controllerModel)
	if err != nil {
		return result, errors.Annotate(err, "failed to create config")
	}
	// NOTE: check the agent-version of the config, and if it is > the current
	// version, it is not supported, also check existing tools, and if we don't
	// have tools for that version, also die.
	model, st, err := mm.state.NewModel(newConfig, ownerTag)
	if err != nil {
		return result, errors.Annotate(err, "failed to create new model")
	}
	defer st.Close()

	result.Name = model.Name()
	result.UUID = model.UUID()
	result.OwnerTag = model.Owner().String()

	return result, nil
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
		logger.Debugf("list models: %s, %s, %s", model.Name(), model.UUID(), model.Owner())
	}

	return result, nil
}

// ModifyModelAccess changes the model access granted to users.
func (em *ModelManagerAPI) ModifyModelAccess(args params.ModifyModelAccessRequest) (result params.ErrorResults, err error) {
	// API user must be a controller admin.
	createdBy, _ := em.authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := em.state.IsControllerAdministrator(createdBy)
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isAdmin {
		return result, errors.New("only controller admins can grant or revoke model access")
	}

	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	for i, arg := range args.Changes {
		modelAccess, err := FromModelAccessParam(arg.Access)
		if err != nil {
			err = errors.Annotate(err, "could not modify model access")
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		userTagString := arg.UserTag
		user, err := names.ParseUserTag(userTagString)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}
		modelTagString := arg.ModelTag
		model, err := names.ParseModelTag(modelTagString)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify model access"))
			continue
		}

		result.Results[i].Error = common.ServerError(
			ChangeModelAccess(em.state, model, createdBy, user, arg.Action, modelAccess))
	}
	return result, nil
}

type stateAccessor interface {
	ForModel(tag names.ModelTag) (*state.State, error)
}

// resolveStateAccess returns the state representation of the logical model
// access type.
func resolveStateAccess(access permission.ModelAccess) (state.ModelAccess, error) {
	var fail state.ModelAccess
	switch access {
	case permission.ModelReadAccess:
		return state.ModelReadAccess, nil
	case permission.ModelWriteAccess:
		// TODO: Initially, we'll map "write" access to admin-level access.
		// Post Juju-2.0, support for more nuanced access will be added to the
		// permission business logic and state model.
		return state.ModelAdminAccess, nil
	}
	logger.Errorf("invalid access permission: %+v", access)
	return fail, errors.Errorf("invalid access permission")
}

// isGreaterAccess returns whether the new access provides more permissions
// than the current access.
// TODO(cmars): If/when more access types are implemented in state,
//   the implementation of this function will certainly need to change, and it
//   should be abstracted away to juju/permission as pure business logic
//   instead of operating on state values.
func isGreaterAccess(currentAccess, newAccess state.ModelAccess) bool {
	if currentAccess == state.ModelReadAccess && newAccess == state.ModelAdminAccess {
		return true
	}
	return false
}

// ChangeModelAccess performs the requested access grant or revoke action for the
// specified user on the specified model.
func ChangeModelAccess(accessor stateAccessor, modelTag names.ModelTag, createdBy, accessedBy names.UserTag, action params.ModelAction, access permission.ModelAccess) error {
	st, err := accessor.ForModel(modelTag)
	if err != nil {
		return errors.Annotate(err, "could not lookup model")
	}
	defer st.Close()

	_, err = st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	stateAccess, err := resolveStateAccess(access)
	if err != nil {
		return errors.Annotate(err, "could not resolve model access")
	}

	switch action {
	case params.GrantModelAccess:
		_, err = st.AddModelUser(state.ModelUserSpec{User: accessedBy, CreatedBy: createdBy, Access: stateAccess})
		if errors.IsAlreadyExists(err) {
			modelUser, err := st.ModelUser(accessedBy)
			if errors.IsNotFound(err) {
				// Conflicts with prior check, must be inconsistent state.
				err = txn.ErrExcessiveContention
			}
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}

			// Only set access if greater access is being granted.
			if isGreaterAccess(modelUser.Access(), stateAccess) {
				err = modelUser.SetAccess(stateAccess)
				if err != nil {
					return errors.Annotate(err, "could not set model access for user")
				}
			} else {
				return errors.Errorf("user already has %q access", modelUser.Access())
			}
			return nil
		}
		return errors.Annotate(err, "could not grant model access")

	case params.RevokeModelAccess:
		if stateAccess == state.ModelReadAccess {
			// Revoking read access removes all access.
			err := st.RemoveModelUser(accessedBy)
			return errors.Annotate(err, "could not revoke model access")

		} else if stateAccess == state.ModelAdminAccess {
			// Revoking admin access sets read-only.
			modelUser, err := st.ModelUser(accessedBy)
			if err != nil {
				return errors.Annotate(err, "could not look up model access for user")
			}
			err = modelUser.SetAccess(state.ModelReadAccess)
			return errors.Annotate(err, "could not set model access to read-only")

		} else {
			return errors.Errorf("don't know how to revoke %q access", stateAccess)
		}

	default:
		return errors.Errorf("unknown action %q", action)
	}
}

// FromModelAccessParam returns the logical model access type from the API wireformat type.
func FromModelAccessParam(paramAccess params.ModelAccessPermission) (permission.ModelAccess, error) {
	var fail permission.ModelAccess
	switch paramAccess {
	case params.ModelReadAccess:
		return permission.ModelReadAccess, nil
	case params.ModelWriteAccess:
		return permission.ModelWriteAccess, nil
	}
	return fail, errors.Errorf("invalid model access permission %q", paramAccess)
}
