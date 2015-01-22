// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.upgrader")

func init() {
	common.RegisterStandardFacade("Upgrader", 0, upgraderFacade)
}

// upgraderFacade is a bit unique vs the other API Facades, as it has two
// implementations that actually expose the same API and which one gets
// returned depends on who is calling.
// Both of them conform to the exact Upgrader API, so the actual calls that are
// available do not depend on who is currently connected.
func upgraderFacade(st *state.State, resources *common.Resources, auth common.Authorizer) (Upgrader, error) {
	// The type of upgrader we return depends on who is asking.
	// Machines get an UpgraderAPI, units get a UnitUpgraderAPI.
	// This is tested in the api/upgrader package since there
	// are currently no direct srvRoot tests.
	// TODO(dfc) this is redundant
	tag, err := names.ParseTag(auth.GetAuthTag().String())
	if err != nil {
		return nil, common.ErrPerm
	}
	switch tag.(type) {
	case names.MachineTag:
		return NewUpgraderAPI(st, resources, auth)
	case names.UnitTag:
		return NewUnitUpgraderAPI(st, resources, auth)
	}
	// Not a machine or unit.
	return nil, common.ErrPerm
}

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

// NewUpgraderAPI creates a new server-side UpgraderAPI facade.
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
	env, err := st.Environment()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(env.UUID(), st)
	return &UpgraderAPI{
		ToolsGetter: common.NewToolsGetter(st, st, st, urlGetter, getCanReadWrite),
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
		tag, err := names.ParseTag(agent.Tag)
		if err != nil {
			return params.NotifyWatchResults{}, errors.Trace(err)
		}
		err = common.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			watch := u.st.WatchForEnvironConfigChanges()
			// Consume the initial event. Technically, API
			// calls to Watch 'transmit' the initial event
			// in the Watch response. But NotifyWatchers
			// have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result.Results[i].NotifyWatcherId = u.resources.Register(watch)
				err = nil
			} else {
				err = watcher.EnsureErr(watch)
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

func (u *UpgraderAPI) entityIsManager(tag names.Tag) bool {
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
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		err = common.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			// Only return the globally desired agent version if the
			// asking entity is a machine agent with JobManageEnviron or
			// if this API server is running the globally desired agent
			// version. Otherwise report this API server's current
			// agent version.
			//
			// This ensures that state machine agents will upgrade
			// first - once they have restarted and are running the
			// new version other agents will start to see the new
			// agent version.
			if !isNewerVersion || u.entityIsManager(tag) {
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
