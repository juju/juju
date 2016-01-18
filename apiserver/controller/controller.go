// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The controller package defines an API end point for functions dealing
// with controllers as a whole.
package controller

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.controller")

func init() {
	common.RegisterStandardFacade("Controller", 1, NewControllerAPI)
}

// Controller defines the methods on the controller API end point.
type Controller interface {
	AllEnvironments() (params.UserEnvironmentList, error)
	DestroyController(args params.DestroyControllerArgs) error
	EnvironmentConfig() (params.EnvironmentConfigResults, error)
	ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error)
	RemoveBlocks(args params.RemoveBlocksArgs) error
	WatchAllEnvs() (params.AllWatcherId, error)
	EnvironmentStatus(req params.Entities) (params.EnvironmentStatusResults, error)
}

// ControllerAPI implements the environment manager interface and is
// the concrete implementation of the api end point.
type ControllerAPI struct {
	state      *state.State
	authorizer common.Authorizer
	apiUser    names.UserTag
	resources  *common.Resources
}

var _ Controller = (*ControllerAPI)(nil)

// NewControllerAPI creates a new api server endpoint for managing
// environments.
func NewControllerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ControllerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := st.IsControllerAdministrator(apiUser)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The entire end point is only accessible to controller administrators.
	if !isAdmin {
		return nil, errors.Trace(common.ErrPerm)
	}

	return &ControllerAPI{
		state:      st,
		authorizer: authorizer,
		apiUser:    apiUser,
		resources:  resources,
	}, nil
}

// AllEnvironments allows controller administrators to get the list of all the
// environments in the controller.
func (s *ControllerAPI) AllEnvironments() (params.UserEnvironmentList, error) {
	result := params.UserEnvironmentList{}

	// Get all the environments that the authenticated user can see, and
	// supplement that with the other environments that exist that the user
	// cannot see. The reason we do this is to get the LastConnection time for
	// the environments that the user is able to see, so we have consistent
	// output when listing with or without --all when an admin user.
	environments, err := s.state.EnvironmentsForUser(s.apiUser)
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
		result.UserEnvironments = append(result.UserEnvironments, params.UserEnvironment{
			Environment: params.Environment{
				Name:     env.Name(),
				UUID:     env.UUID(),
				OwnerTag: env.Owner().String(),
			},
			LastConnection: &lastConn,
		})
	}

	allEnvs, err := s.state.AllEnvironments()
	if err != nil {
		return result, errors.Trace(err)
	}

	for _, env := range allEnvs {
		if !visibleEnvironments.Contains(env.UUID()) {
			result.UserEnvironments = append(result.UserEnvironments, params.UserEnvironment{
				Environment: params.Environment{
					Name:     env.Name(),
					UUID:     env.UUID(),
					OwnerTag: env.Owner().String(),
				},
				// No LastConnection as this user hasn't.
			})
		}
	}

	// Sort the resulting sequence by environment name, then owner.
	sort.Sort(orderedUserEnvironments(result.UserEnvironments))

	return result, nil
}

// ListBlockedEnvironments returns a list of all environments on the controller
// which have a block in place.  The resulting slice is sorted by environment
// name, then owner. Callers must be controller administrators to retrieve the
// list.
func (s *ControllerAPI) ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error) {
	results := params.EnvironmentBlockInfoList{}

	blocks, err := s.state.AllBlocksForController()
	if err != nil {
		return results, errors.Trace(err)
	}

	envBlocks := make(map[string][]string)
	for _, block := range blocks {
		uuid := block.EnvUUID()
		types, ok := envBlocks[uuid]
		if !ok {
			types = []string{block.Type().String()}
		} else {
			types = append(types, block.Type().String())
		}
		envBlocks[uuid] = types
	}

	for uuid, blocks := range envBlocks {
		envInfo, err := s.state.GetEnvironment(names.NewEnvironTag(uuid))
		if err != nil {
			logger.Debugf("Unable to get name for environment: %s", uuid)
			continue
		}
		results.Environments = append(results.Environments, params.EnvironmentBlockInfo{
			UUID:     envInfo.UUID(),
			Name:     envInfo.Name(),
			OwnerTag: envInfo.Owner().String(),
			Blocks:   blocks,
		})
	}

	// Sort the resulting sequence by environment name, then owner.
	sort.Sort(orderedBlockInfo(results.Environments))

	return results, nil
}

// EnvironmentConfig returns the environment config for the controller
// environment.  For information on the current environment, use
// client.EnvironmentGet
func (s *ControllerAPI) EnvironmentConfig() (params.EnvironmentConfigResults, error) {
	result := params.EnvironmentConfigResults{}

	stateServerEnv, err := s.state.ControllerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}

	config, err := stateServerEnv.Config()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config.AllAttrs()
	return result, nil
}

// RemoveBlocks removes all the blocks in the controller.
func (s *ControllerAPI) RemoveBlocks(args params.RemoveBlocksArgs) error {
	if !args.All {
		return errors.New("not supported")
	}
	return errors.Trace(s.state.RemoveAllBlocksForController())
}

// WatchAllEnvs starts watching events for all environments in the
// controller. The returned AllWatcherId should be used with Next on the
// AllEnvWatcher endpoint to receive deltas.
func (c *ControllerAPI) WatchAllEnvs() (params.AllWatcherId, error) {
	w := c.state.WatchAllEnvs()
	return params.AllWatcherId{
		AllWatcherId: c.resources.Register(w),
	}, nil
}

type orderedBlockInfo []params.EnvironmentBlockInfo

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

// EnvironmentStatus returns a summary of the environment.
func (c *ControllerAPI) EnvironmentStatus(req params.Entities) (params.EnvironmentStatusResults, error) {
	envs := req.Entities
	results := params.EnvironmentStatusResults{}
	status := make([]params.EnvironmentStatus, len(envs))
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

func (c *ControllerAPI) environStatus(tag string) (params.EnvironmentStatus, error) {
	var status params.EnvironmentStatus
	envTag, err := names.ParseEnvironTag(tag)
	if err != nil {
		return status, errors.Trace(err)
	}
	st, err := c.state.ForEnviron(envTag)
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

	services, err := st.AllServices()
	if err != nil {
		return status, errors.Trace(err)
	}

	env, err := st.Environment()
	if err != nil {
		return status, errors.Trace(err)
	}
	if err != nil {
		return status, errors.Trace(err)
	}

	return params.EnvironmentStatus{
		EnvironTag:         tag,
		OwnerTag:           env.Owner().String(),
		Life:               params.Life(env.Life().String()),
		HostedMachineCount: len(hostedMachines),
		ServiceCount:       len(services),
	}, nil
}

func (o orderedBlockInfo) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

type orderedUserEnvironments []params.UserEnvironment

func (o orderedUserEnvironments) Len() int {
	return len(o)
}

func (o orderedUserEnvironments) Less(i, j int) bool {
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

func (o orderedUserEnvironments) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}
