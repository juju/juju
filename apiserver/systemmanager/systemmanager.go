// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The systemmanager package defines an API end point for functions dealing
// with systems as a whole. Primarily the destruction of systems.
package systemmanager

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.systemmanager")

func init() {
	common.RegisterStandardFacadeForFeature("SystemManager", 1, NewSystemManagerAPI, feature.JES)
}

// SystemManager defines the methods on the systemmanager API end point.
type SystemManager interface {
	AllEnvironments() (params.UserEnvironmentList, error)
	DestroySystem(args params.DestroySystemArgs) error
	EnvironmentConfig() (params.EnvironmentConfigResults, error)
	ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error)
	RemoveBlocks(args params.RemoveBlocksArgs) error
	WatchAllEnvs() (params.AllWatcherId, error)
}

// SystemManagerAPI implements the environment manager interface and is
// the concrete implementation of the api end point.
type SystemManagerAPI struct {
	state      *state.State
	authorizer common.Authorizer
	apiUser    names.UserTag
	resources  *common.Resources
}

var _ SystemManager = (*SystemManagerAPI)(nil)

// NewSystemManagerAPI creates a new api server endpoint for managing
// environments.
func NewSystemManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*SystemManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, errors.Trace(common.ErrPerm)
	}

	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	isAdmin, err := st.IsSystemAdministrator(apiUser)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// The entire end point is only accessible to system administrators.
	if !isAdmin {
		return nil, errors.Trace(common.ErrPerm)
	}

	return &SystemManagerAPI{
		state:      st,
		authorizer: authorizer,
		apiUser:    apiUser,
		resources:  resources,
	}, nil
}

// AllEnvironments allows system administrators to get the list of all the
// environments in the system.
func (s *SystemManagerAPI) AllEnvironments() (params.UserEnvironmentList, error) {
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

// ListBlockedEnvironments returns a list of all environments on the system
// which have a block in place.  The resulting slice is sorted by environment
// name, then owner. Callers must be system administrators to retrieve the
// list.
func (s *SystemManagerAPI) ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error) {
	results := params.EnvironmentBlockInfoList{}

	blocks, err := s.state.AllBlocksForSystem()
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

// DestroySystem will attempt to destroy the system. If the args specify the
// removal of blocks or the destruction of the environments, this method will
// attempt to do so.
func (s *SystemManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	// Get list of all environments in the system.
	allEnvs, err := s.state.AllEnvironments()
	if err != nil {
		return errors.Trace(err)
	}

	// If there are hosted environments and DestroyEnvironments was not
	// specified, don't bother trying to destroy the system, as it will fail.
	if len(allEnvs) > 1 && !args.DestroyEnvironments {
		return errors.Errorf("state server environment cannot be destroyed before all other environments are destroyed")
	}

	// If there are blocks, and we aren't being told to ignore them, let the
	// user know.
	blocks, err := s.state.AllBlocksForSystem()
	if err != nil {
		logger.Debugf("Unable to get blocks for system: %s", err)
		if !args.IgnoreBlocks {
			return errors.Trace(err)
		}
	}
	if len(blocks) > 0 {
		if !args.IgnoreBlocks {
			return common.ErrOperationBlocked("found blocks in system environments")
		}

		err := s.state.RemoveAllBlocksForSystem()
		if err != nil {
			return errors.Trace(err)
		}
	}

	systemEnv, err := s.state.StateServerEnvironment()
	if err != nil {
		return errors.Trace(err)
	}
	systemTag := systemEnv.EnvironTag()

	if args.DestroyEnvironments {
		for _, env := range allEnvs {
			environTag := env.EnvironTag()
			if environTag != systemTag {
				if err := common.DestroyEnvironment(s.state, environTag); err != nil {
					logger.Errorf("unable to destroy environment %q: %s", env.UUID(), err)
				}
			}
		}
	}

	return errors.Trace(common.DestroyEnvironment(s.state, systemTag))
}

// EnvironmentConfig returns the environment config for the system
// environment.  For information on the current environment, use
// client.EnvironmentGet
func (s *SystemManagerAPI) EnvironmentConfig() (params.EnvironmentConfigResults, error) {
	result := params.EnvironmentConfigResults{}

	stateServerEnv, err := s.state.StateServerEnvironment()
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

// RemoveBlocks removes all the blocks in the system.
func (s *SystemManagerAPI) RemoveBlocks(args params.RemoveBlocksArgs) error {
	if !args.All {
		return errors.New("not supported")
	}
	return errors.Trace(s.state.RemoveAllBlocksForSystem())
}

// WatchAllEnvs starts watching events for all environments in the
// system. The returned AllWatcherId should be used with Next on the
// AllEnvWatcher endpoint to receive deltas.
func (c *SystemManagerAPI) WatchAllEnvs() (params.AllWatcherId, error) {
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
