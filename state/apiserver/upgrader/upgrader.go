// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"errors"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.state.apiserver.upgrader")

type Upgrader interface {
	WatchAPIVersion(args params.Entities) (params.NotifyWatchResults, error)
	DesiredVersion(args params.Entities) (params.VersionResults, error)
	Tools(args params.Entities) (params.ToolsResults, error)
	SetTools(args params.EntitiesVersion) (params.ErrorResults, error)
}

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
	*common.ToolsGetter
	*common.ToolsSetter

	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewUpgraderAPI creates a new client-side UpgraderAPI facade.
func NewUpgraderAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*UpgraderAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}
	getCanReadWrite := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	return &UpgraderAPI{
		ToolsGetter: common.NewToolsGetter(st, getCanReadWrite),
		ToolsSetter: common.NewToolsSetter(st, getCanReadWrite),
		st:          st,
		resources:   resources,
		authorizer:  authorizer,
	}, nil
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade to
func (u *UpgraderAPI) WatchAPIVersion(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(agent.Tag) {
			watch := u.st.WatchForEnvironConfigChanges()
			// Consume the initial event. Technically, API
			// calls to Watch 'transmit' the initial event
			// in the Watch response. But NotifyWatchers
			// have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result.Results[i].NotifyWatcherId = u.resources.Register(watch)
				err = nil
			} else {
				err = watcher.MustErr(watch)
			}
		}
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (u *UpgraderAPI) getGlobalAgentVersion() (version.Number, *config.Config, error) {
	// Get the Agent Version requested in the Environment Config
	cfg, err := u.st.EnvironConfig()
	if err != nil {
		return version.Number{}, nil, err
	}
	agentVersion, ok := cfg.AgentVersion()
	if !ok {
		return version.Number{}, nil, errors.New("agent version not set in environment config")
	}
	return agentVersion, cfg, nil
}

type hasIsManager interface {
	IsManager() bool
}

func (u *UpgraderAPI) entityIsManager(tag string) bool {
	entity, err := u.st.FindEntity(tag)
	if err != nil {
		return false
	}
	if m, ok := entity.(hasIsManager); !ok {
		return false
	} else {
		return m.IsManager()
	}
}

// DesiredVersion reports the Agent Version that we want that agent to be running
func (u *UpgraderAPI) DesiredVersion(args params.Entities) (params.VersionResults, error) {
	results := make([]params.VersionResult, len(args.Entities))
	if len(args.Entities) == 0 {
		return params.VersionResults{}, nil
	}
	agentVersion, _, err := u.getGlobalAgentVersion()
	if err != nil {
		return params.VersionResults{}, common.ServerError(err)
	}
	// Is the desired version greater than the current API server version?
	isNewerVersion := agentVersion.Compare(version.Current.Number) > 0
	for i, entity := range args.Entities {
		err := common.ErrPerm
		if u.authorizer.AuthOwner(entity.Tag) {
			if !isNewerVersion || u.entityIsManager(entity.Tag) {
				results[i].Version = &agentVersion
			} else {
				logger.Debugf("desired version is %s, but current version is %s and agent is not a manager node", agentVersion, version.Current.Number)
				results[i].Version = &version.Current.Number
			}
			err = nil
		}
		results[i].Error = common.ServerError(err)
	}
	return params.VersionResults{Results: results}, nil
}
