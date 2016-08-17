// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The controller package defines an API end point for functions dealing
// with controllers as a whole.
package controller

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/txn"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/description"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.controller")

func init() {
	common.RegisterStandardFacade("Controller", 3, NewControllerAPI)
}

// Controller defines the methods on the controller API end point.
type Controller interface {
	AllModels() (params.UserModelList, error)
	DestroyController(args params.DestroyControllerArgs) error
	ModelConfig() (params.ModelConfigResults, error)
	ControllerConfig() (params.ControllerConfigResult, error)
	ListBlockedModels() (params.ModelBlockInfoList, error)
	RemoveBlocks(args params.RemoveBlocksArgs) error
	WatchAllModels() (params.AllWatcherId, error)
	ModelStatus(params.Entities) (params.ModelStatusResults, error)
	InitiateModelMigration(params.InitiateModelMigrationArgs) (params.InitiateModelMigrationResults, error)
	ModifyControllerAccess(params.ModifyControllerAccessRequest) (params.ErrorResults, error)
}

// ControllerAPI implements the environment manager interface and is
// the concrete implementation of the api end point.
type ControllerAPI struct {
	*common.ControllerConfigAPI
	cloudspec.CloudSpecAPI

	state      *state.State
	authorizer facade.Authorizer
	apiUser    names.UserTag
	resources  facade.Resources
}

var _ Controller = (*ControllerAPI)(nil)

// NewControllerAPI creates a new api server endpoint for managing
// environments.
func NewControllerAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*ControllerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	isAdmin, err := authorizer.HasPermission(description.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The entire end point is only accessible to controller administrators.
	if !isAdmin {
		return nil, errors.Trace(common.ErrPerm)
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	environConfigGetter := stateenvirons.EnvironConfigGetter{st}
	return &ControllerAPI{
		ControllerConfigAPI: common.NewControllerConfig(st),
		CloudSpecAPI:        cloudspec.NewCloudSpec(environConfigGetter.CloudSpec, common.AuthFuncForTag(st.ModelTag())),
		state:               st,
		authorizer:          authorizer,
		apiUser:             apiUser,
		resources:           resources,
	}, nil
}

func (s *ControllerAPI) hasReadAccess() (bool, error) {
	canRead, err := s.authorizer.HasPermission(description.ReadAccess, s.state.ModelTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canRead, err

}

func (s *ControllerAPI) hasWriteAccess() (bool, error) {
	canWrite, err := s.authorizer.HasPermission(description.WriteAccess, s.state.ModelTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

func (s *ControllerAPI) hasAdminAccess() (bool, error) {
	isAdmin, err := s.authorizer.HasPermission(description.SuperuserAccess, s.state.ControllerTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return isAdmin, err
}

// AllModels allows controller administrators to get the list of all the
// environments in the controller.
func (s *ControllerAPI) AllModels() (params.UserModelList, error) {
	result := params.UserModelList{}

	admin, err := s.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !admin {
		return result, common.ServerError(common.ErrPerm)
	}

	// Get all the environments that the authenticated user can see, and
	// supplement that with the other environments that exist that the user
	// cannot see. The reason we do this is to get the LastConnection time for
	// the environments that the user is able to see, so we have consistent
	// output when listing with or without --all when an admin user.
	environments, err := s.state.ModelsForUser(s.apiUser)
	if err != nil {
		return result, errors.Trace(err)
	}
	visibleEnvironments := set.NewStrings()
	for _, env := range environments {
		lastConn, err := env.LastConnection()
		if err != nil && !state.IsNeverConnectedError(err) {
			return result, errors.Trace(err)
		}
		visibleEnvironments.Add(env.UUID())
		result.UserModels = append(result.UserModels, params.UserModel{
			Model: params.Model{
				Name:     env.Name(),
				UUID:     env.UUID(),
				OwnerTag: env.Owner().String(),
			},
			LastConnection: &lastConn,
		})
	}

	allEnvs, err := s.state.AllModels()
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range allEnvs {
		if !visibleEnvironments.Contains(env.UUID()) {
			result.UserModels = append(result.UserModels, params.UserModel{
				Model: params.Model{
					Name:     env.Name(),
					UUID:     env.UUID(),
					OwnerTag: env.Owner().String(),
				},
				// No LastConnection as this user hasn't.
			})
		}
	}

	// Sort the resulting sequence by environment name, then owner.
	sort.Sort(orderedUserModels(result.UserModels))

	return result, nil
}

// ListBlockedModels returns a list of all environments on the controller
// which have a block in place.  The resulting slice is sorted by environment
// name, then owner. Callers must be controller administrators to retrieve the
// list.
func (s *ControllerAPI) ListBlockedModels() (params.ModelBlockInfoList, error) {
	results := params.ModelBlockInfoList{}
	admin, err := s.hasAdminAccess()
	if err != nil {
		return results, errors.Trace(err)
	}
	if !admin {
		return results, common.ServerError(common.ErrPerm)
	}
	blocks, err := s.state.AllBlocksForController()
	if err != nil {
		return results, errors.Trace(err)
	}

	envBlocks := make(map[string][]string)
	for _, block := range blocks {
		uuid := block.ModelUUID()
		types, ok := envBlocks[uuid]
		if !ok {
			types = []string{block.Type().String()}
		} else {
			types = append(types, block.Type().String())
		}
		envBlocks[uuid] = types
	}

	for uuid, blocks := range envBlocks {
		envInfo, err := s.state.GetModel(names.NewModelTag(uuid))
		if err != nil {
			logger.Debugf("Unable to get name for model: %s", uuid)
			continue
		}
		results.Models = append(results.Models, params.ModelBlockInfo{
			UUID:     envInfo.UUID(),
			Name:     envInfo.Name(),
			OwnerTag: envInfo.Owner().String(),
			Blocks:   blocks,
		})
	}

	// Sort the resulting sequence by environment name, then owner.
	sort.Sort(orderedBlockInfo(results.Models))

	return results, nil
}

// ModelConfig returns the environment config for the controller
// environment.  For information on the current environment, use
// client.ModelGet
func (s *ControllerAPI) ModelConfig() (params.ModelConfigResults, error) {
	result := params.ModelConfigResults{}
	admin, err := s.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !admin {
		return result, common.ServerError(common.ErrPerm)
	}

	controllerModel, err := s.state.ControllerModel()
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

// RemoveBlocks removes all the blocks in the controller.
func (s *ControllerAPI) RemoveBlocks(args params.RemoveBlocksArgs) error {
	admin, err := s.hasAdminAccess()
	if err != nil {
		return errors.Trace(err)
	}
	if !admin {
		return common.ServerError(common.ErrPerm)
	}

	if !args.All {
		return errors.New("not supported")
	}
	return errors.Trace(s.state.RemoveAllBlocksForController())
}

// WatchAllModels starts watching events for all models in the
// controller. The returned AllWatcherId should be used with Next on the
// AllModelWatcher endpoint to receive deltas.
func (c *ControllerAPI) WatchAllModels() (params.AllWatcherId, error) {
	w := c.state.WatchAllModels()
	return params.AllWatcherId{
		AllWatcherId: c.resources.Register(w),
	}, nil
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
	// environments of the same name for the same owner, but return false
	// instead of panicing.
	return false
}

// ModelStatus returns a summary of the environment.
func (c *ControllerAPI) ModelStatus(req params.Entities) (params.ModelStatusResults, error) {
	envs := req.Entities
	results := params.ModelStatusResults{}
	admin, err := c.hasAdminAccess()
	if err != nil {
		return results, errors.Trace(err)
	}
	if !admin {
		return results, common.ServerError(common.ErrPerm)
	}

	status := make([]params.ModelStatus, len(envs))
	for i, env := range envs {
		envStatus, err := c.environStatus(env.Tag)
		if err != nil {
			return results, errors.Trace(err)
		}
		status[i] = envStatus
	}
	results.Results = status
	return results, nil
}

// InitiateModelMigration attempts to begin the migration of one or
// more models to other controllers.
func (c *ControllerAPI) InitiateModelMigration(reqArgs params.InitiateModelMigrationArgs) (
	params.InitiateModelMigrationResults, error,
) {
	out := params.InitiateModelMigrationResults{
		Results: make([]params.InitiateModelMigrationResult, len(reqArgs.Specs)),
	}
	admin, err := c.hasAdminAccess()
	if err != nil {
		return out, errors.Trace(err)
	}
	if !admin {
		return out, common.ServerError(common.ErrPerm)
	}

	for i, spec := range reqArgs.Specs {
		result := &out.Results[i]
		result.ModelTag = spec.ModelTag
		id, err := c.initiateOneModelMigration(spec)
		if err != nil {
			result.Error = common.ServerError(err)
		} else {
			result.MigrationId = id
		}
	}
	return out, nil
}

func (c *ControllerAPI) initiateOneModelMigration(spec params.ModelMigrationSpec) (string, error) {
	modelTag, err := names.ParseModelTag(spec.ModelTag)
	if err != nil {
		return "", errors.Annotate(err, "model tag")
	}

	// Ensure the model exists.
	if _, err := c.state.GetModel(modelTag); err != nil {
		return "", errors.Annotate(err, "unable to read model")
	}

	// Get State for model.
	hostedState, err := c.state.ForModel(modelTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer hostedState.Close()

	// Start the migration.
	targetInfo := spec.TargetInfo

	controllerTag, err := names.ParseModelTag(targetInfo.ControllerTag)
	if err != nil {
		return "", errors.Annotate(err, "controller tag")
	}
	authTag, err := names.ParseUserTag(targetInfo.AuthTag)
	if err != nil {
		return "", errors.Annotate(err, "auth tag")
	}

	args := state.ModelMigrationSpec{
		InitiatedBy: c.apiUser,
		TargetInfo: migration.TargetInfo{
			ControllerTag: controllerTag,
			Addrs:         targetInfo.Addrs,
			CACert:        targetInfo.CACert,
			AuthTag:       authTag,
			Password:      targetInfo.Password,
		},
	}
	mig, err := hostedState.CreateModelMigration(args)
	if err != nil {
		return "", errors.Trace(err)
	}
	return mig.Id(), nil
}

func (c *ControllerAPI) environStatus(tag string) (params.ModelStatus, error) {
	var status params.ModelStatus
	modelTag, err := names.ParseModelTag(tag)
	if err != nil {
		return status, errors.Trace(err)
	}
	st, err := c.state.ForModel(modelTag)
	if err != nil {
		return status, errors.Trace(err)
	}
	defer st.Close()

	machines, err := st.AllMachines()
	if err != nil {
		return status, errors.Trace(err)
	}

	var hostedMachines []*state.Machine
	for _, m := range machines {
		if !m.IsManager() {
			hostedMachines = append(hostedMachines, m)
		}
	}

	services, err := st.AllApplications()
	if err != nil {
		return status, errors.Trace(err)
	}

	env, err := st.Model()
	if err != nil {
		return status, errors.Trace(err)
	}
	if err != nil {
		return status, errors.Trace(err)
	}

	return params.ModelStatus{
		ModelTag:           tag,
		OwnerTag:           env.Owner().String(),
		Life:               params.Life(env.Life().String()),
		HostedMachineCount: len(hostedMachines),
		ApplicationCount:   len(services),
	}, nil
}

// ModifyControllerAccess changes the model access granted to users.
func (c *ControllerAPI) ModifyControllerAccess(args params.ModifyControllerAccessRequest) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	hasPermission, err := c.authorizer.HasPermission(description.SuperuserAccess, c.state.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Changes {
		if !hasPermission {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		controllerAccess := description.Access(arg.Access)
		if err := description.ValidateControllerAccess(controllerAccess); err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}

		targetUserTag, err := names.ParseUserTag(arg.UserTag)
		if err != nil {
			result.Results[i].Error = common.ServerError(errors.Annotate(err, "could not modify controller access"))
			continue
		}

		result.Results[i].Error = common.ServerError(
			ChangeControllerAccess(c.state, c.apiUser, targetUserTag, arg.Action, controllerAccess))
	}
	return result, nil
}

func grantControllerAccess(accessor *state.State, targetUserTag, apiUser names.UserTag, access description.Access) error {
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

func revokeControllerAccess(accessor *state.State, targetUserTag, apiUser names.UserTag, access description.Access) error {
	controllerTag := accessor.ControllerTag()
	switch access {
	case description.LoginAccess:
		// Revoking login access removes all access.
		err := accessor.RemoveUserAccess(targetUserTag, controllerTag)
		return errors.Annotate(err, "could not revoke controller access")
	case description.AddModelAccess:
		// Revoking add-model access sets login.
		controllerUser, err := accessor.UserAccess(targetUserTag, controllerTag)
		if err != nil {
			return errors.Annotate(err, "could not look up controller access for user")
		}
		_, err = accessor.SetUserAccess(controllerUser.UserTag, controllerUser.Object, description.LoginAccess)
		return errors.Annotate(err, "could not set controller access to read-only")
	case description.SuperuserAccess:
		// Revoking superuser sets add-model.
		controllerUser, err := accessor.UserAccess(targetUserTag, controllerTag)
		if err != nil {
			return errors.Annotate(err, "could not look up controller access for user")
		}
		_, err = accessor.SetUserAccess(controllerUser.UserTag, controllerUser.Object, description.AddModelAccess)
		return errors.Annotate(err, "could not set controller access to add-model")

	default:
		return errors.Errorf("don't know how to revoke %q access", access)
	}

}

// ChangeControllerAccess performs the requested access grant or revoke action for the
// specified user on the controller.
func ChangeControllerAccess(accessor *state.State, apiUser, targetUserTag names.UserTag, action params.ControllerAction, access description.Access) error {
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

func (o orderedBlockInfo) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

type orderedUserModels []params.UserModel

func (o orderedUserModels) Len() int {
	return len(o)
}

func (o orderedUserModels) Less(i, j int) bool {
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
	// environments of the same name for the same owner, but return false
	// instead of panicing.
	return false
}

func (o orderedUserModels) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}
