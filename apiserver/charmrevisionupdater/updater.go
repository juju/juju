// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.charmrevisionupdater")

func init() {
	common.RegisterStandardFacade("CharmRevisionUpdater", 0, NewCharmRevisionUpdaterAPI)
}

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
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	uuid := env.UUID()

	deployedCharms, err := fetchAllDeployedCharms(api.state)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	// Look up the revision information for all the deployed charms.
	curls, err := retrieveLatestCharmInfo(deployedCharms, uuid)
	if err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	// Add the charms and latest revision info to state as charm placeholders.
	for _, curl := range curls {
		if err = api.state.AddStoreCharmPlaceholder(curl); err != nil {
			return params.ErrorResult{Error: common.ServerError(err)}, nil
		}
	}
	return params.ErrorResult{}, nil
}

// fetchAllDeployedCharms returns a map from service name to service
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

// NewCharmStore instantiates a new charm store repository.
// It is defined at top level for testing purposes.
var NewCharmStore = charmrepo.NewCharmStore

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
	repo := NewCharmStore(charmrepo.NewCharmStoreParams{})
	repo = repo.(*charmrepo.CharmStore).WithJujuAttrs(map[string]string{
		"environment_uuid": uuid,
	})
	revInfo, err := repo.Latest(curls...)
	if err != nil {
		err = errors.Annotate(err, "finding charm revision info")
		logger.Infof(err.Error())
		return nil, err
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
