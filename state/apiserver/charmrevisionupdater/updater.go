// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver.charmrevisionupdater")

// CharmRevisionUpdater defines the methods on the charmrevisionupdater API end point.
type CharmRevisionUpdater interface {
	UpdateLatestRevisions() (params.ErrorResult, error)
}

// CharmRevisionUpdaterAPI implements the CharmRevisionUpdater interface and is the concrete
// implementation of the api end point.
type CharmRevisionUpdaterAPI struct {
	state      *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

var _ CharmRevisionUpdater = (*CharmRevisionUpdaterAPI)(nil)

// NewCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func NewCharmRevisionUpdaterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*CharmRevisionUpdaterAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthEnvironManager() {
		return nil, common.ErrPerm
	}
	return &CharmRevisionUpdaterAPI{
		state: st, resources: resources, authorizer: authorizer}, nil
}

// UpdateLatestRevisions retrieves the latest revision information from the charm store for all deployed charms
// and records this information in state.
func (api *CharmRevisionUpdaterAPI) UpdateLatestRevisions() (params.ErrorResult, error) {
	// First get the uuid for the environment to use when querying the charm store.
	env, err := api.state.Environment()
	if err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	uuid := env.UUID()

	deployedCharms, err := fetchAllDeployedCharms(api.state)
	if err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	// Look up the revision information for all the deployed charms.
	curls, err := retrieveLatestCharmInfo(deployedCharms, uuid)
	if err != nil {
		return params.ErrorResult{common.ServerError(err)}, nil
	}
	// Add the charms and latest revision info to state as charm placeholders.
	for _, curl := range curls {
		if err = api.state.AddStoreCharmPlaceholder(curl); err != nil {
			return params.ErrorResult{common.ServerError(err)}, nil
		}
	}
	return params.ErrorResult{}, nil
}

// fetchAllServicesAndUnits returns a map from service name to service
// and a map from service name to unit name to unit.
func fetchAllDeployedCharms(st *state.State) (map[string]*charm.URL, error) {
	deployedCharms := make(map[string]*charm.URL)
	services, err := st.AllServices()
	if err != nil {
		return nil, err
	}
	for _, s := range services {
		url, _ := s.CharmURL()
		// Record the basic charm information so it can be bulk processed later to
		// get the available revision numbers from the repo.
		baseCharm := url.WithRevision(-1)
		deployedCharms[baseCharm.String()] = baseCharm
	}
	return deployedCharms, nil
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func retrieveLatestCharmInfo(deployedCharms map[string]*charm.URL, uuid string) ([]*charm.URL, error) {
	var curls []*charm.URL
	for _, curl := range deployedCharms {
		if curl.Schema == "local" {
			// Version checking for charms from local repositories is not
			// currently supported, since we don't yet support passing in
			// a path to the local repo. This may change if the need arises.
			continue
		}
		curls = append(curls, curl)
	}

	// Do a bulk call to get the revision info for all charms.
	logger.Infof("retrieving revision information for %d charms", len(curls))
	store := charm.Store.WithJujuAttrs("environment_uuid=" + uuid)
	revInfo, err := store.Latest(curls...)
	if err != nil {
		return nil, log.LoggedErrorf(logger, "finding charm revision info: %v", err)
	}
	var latestCurls []*charm.URL
	for i, info := range revInfo {
		curl := curls[i]
		if info.Err == nil {
			latestCurls = append(latestCurls, curl.WithRevision(info.Revision))
		} else {
			logger.Errorf("retrieving charm info for %s: %v", curl, info.Err)
		}
	}
	return latestCurls, nil
}
