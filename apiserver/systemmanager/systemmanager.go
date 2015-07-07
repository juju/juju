// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The systemmanager package defines an API end point for functions
// dealing with systems.

package systemmanager

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/environmentmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.systemmanager")

func init() {
	common.RegisterStandardFacadeForFeature("SystemManager", 0, NewSystemManagerAPI, feature.JES)
}

// SystemManager defines the methods on the systemmanager API endpoint.
type SystemManager interface {
	DestroySystem(args params.DestroySystemArgs) error
	EnvironmentGet() (params.EnvironmentConfigResults, error)
	ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error)
}

// SystemManagerAPI implements the system manager interface and is
// the concrete implementation of the api endpoint.
type SystemManagerAPI struct {
	state      *state.State
	authorizer common.Authorizer
	resources  *common.Resources
}

var _ SystemManager = (*SystemManagerAPI)(nil)

// NewSystemManagerAPI creates a new api server endpoint for managing
// systems.
func NewSystemManagerAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*SystemManagerAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &SystemManagerAPI{
		state:      st,
		authorizer: authorizer,
	}, nil
}

// isSystemAdministrator determines if the api user is a system administrator
func (sm *SystemManagerAPI) isSystemAdministrator() (bool, error) {
	authTag := sm.authorizer.GetAuthTag()
	apiUser, ok := authTag.(names.UserTag)
	if !ok {
		return false, errors.Errorf("auth tag should be a user, but isn't: %q", authTag.String())
	}

	isAdmin, err := sm.state.IsSystemAdministrator(apiUser)
	if err != nil {
		return false, errors.Trace(err)
	}

	return isAdmin, nil
}

// ListBlockedEnvironments returns a list of all environments on the system which
// have a block in place.  Callers must be a system administrator to retrieve the list.
func (sm *SystemManagerAPI) ListBlockedEnvironments() (params.EnvironmentBlockInfoList, error) {
	results := params.EnvironmentBlockInfoList{}

	// Check that we are authorized
	isAdmin, err := sm.isSystemAdministrator()
	if err != nil {
		return results, errors.Trace(err)
	}

	if !isAdmin {
		return results, common.ErrPerm
	}

	blocks, err := sm.state.AllBlocksForSystem()
	if err != nil {
		return results, errors.Trace(err)
	}

	envBlocks := make(map[string][]string)
	for _, block := range blocks {
		uuid := block.EnvUUID()
		envBlocks[uuid] = append(envBlocks[uuid], block.Type().String())
	}

	for uuid, blocks := range envBlocks {
		envInfo, err := sm.state.GetEnvironment(names.NewEnvironTag(uuid))
		if err != nil {
			// Environment no longer exists, don't list it.
			logger.Debugf("Unable to get name for environment: %s", uuid)
			continue
		}
		results.Environments = append(results.Environments, params.EnvironmentBlockInfo{
			Environment: params.Environment{
				UUID:     envInfo.UUID(),
				Name:     envInfo.Name(),
				OwnerTag: envInfo.Owner().String(),
			},
			Blocks: blocks,
		})
	}

	return results, nil
}

func (sm *SystemManagerAPI) DestroySystem(args params.DestroySystemArgs) error {
	// Check we're destroying the system env
	st := sm.state

	envTag, err := names.ParseEnvironTag(args.EnvTag)
	if err != nil {
		return errors.Trace(err)
	}

	stateServerEnv, err := st.StateServerEnvironment()
	if err != nil {
		return errors.Trace(err)
	}

	if envTag.Id() != stateServerEnv.ServerUUID() {
		return errors.Errorf("%q is not a system", envTag.Id())
	}

	// Check that we are authorized
	isAdmin, err := sm.isSystemAdministrator()
	if err != nil {
		return errors.Trace(err)
	}

	if !isAdmin {
		return common.ErrPerm
	}

	// Make sure we can get an EnvironmentManagerAPI connection to destroy
	// the environments
	envManager, err := environmentmanager.NewEnvironmentManagerAPI(sm.state, sm.resources, sm.authorizer)
	if err != nil {
		return errors.Trace(err)
	}

	// Get list of all environments in the system.
	allEnvs, err := st.AllEnvironments()
	if err != nil {
		return errors.Trace(err)
	}

	// If there are hosted environments and DestroyEnvs was not specified, don't
	// bother trying to destroy the system, as it will fail.
	if len(allEnvs) > 1 && !args.DestroyEnvs {
		return errors.Errorf("state server environment cannot be destroyed before all other environments are destroyed")
	}

	// Check for blocks
	blocks, err := st.AllBlocksForSystem()
	if err != nil {
		// If we're ignoring blocks, we're trying to kill the system.  Don't fail
		// here and attempt to destroy environments to clean up as much as possible.
		logger.Warningf("Unable to get blocks for system: %s", err)
		if !args.IgnoreBlocks {
			return errors.Trace(err)
		}
	}
	if len(blocks) > 0 {
		if !args.IgnoreBlocks {
			return common.ErrOperationBlocked("found blocks in system environments")
		}

		err := st.RemoveAllBlocksForSystem()
		if err != nil {
			// If we're ignoring blocks, we're trying to kill the system.  Don't fail
			// here and attempt to destroy environments to clean up as much as possible.
			logger.Warningf("Unable to remove all blocks for system: %s", err)
		}
	}

	if args.DestroyEnvs {
		for _, env := range allEnvs {
			if env.UUID() != envTag.Id() {
				tag := names.NewEnvironTag(env.UUID())
				err = envManager.DestroyEnvironment(params.DestroyEnvironmentArgs{tag.String()})
				if err != nil {
					logger.Warningf("unable to destroy environment %q: %s", env.UUID(), err)
				}
			}
		}
	}

	return envManager.DestroyEnvironment(params.DestroyEnvironmentArgs{envTag.String()})
}

// EnvironmentGet returns the environment config for the system
// environment.  For information on the current environment, use
// client.EnvironmentGet
func (sm *SystemManagerAPI) EnvironmentGet() (_ params.EnvironmentConfigResults, err error) {
	result := params.EnvironmentConfigResults{}

	stateServerEnv, err := sm.state.StateServerEnvironment()
	if err != nil {
		return result, errors.Trace(err)
	}

	// Check that we are authorized
	isAdmin, err := sm.isSystemAdministrator()
	if err != nil {
		return result, errors.Trace(err)
	}

	if !isAdmin {
		return result, common.ErrPerm
	}

	config, err := stateServerEnv.Config()
	if err != nil {
		return result, errors.Trace(err)
	}

	result.Config = config.AllAttrs()
	return result, nil
}
