// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.charmrevisionupdater")

func init() {
	common.RegisterStandardFacade("CharmRevisionUpdater", 1, NewCharmRevisionUpdaterAPI)
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
	if !authorizer.AuthMachineAgent() && !authorizer.AuthModelManager() {
		return nil, common.ErrPerm
	}
	return &CharmRevisionUpdaterAPI{
		state: st, resources: resources, authorizer: authorizer}, nil
}

// UpdateLatestRevisions retrieves the latest revision information from the charm store for all deployed charms
// and records this information in state.
func (api *CharmRevisionUpdaterAPI) UpdateLatestRevisions() (params.ErrorResult, error) {
	if err := api.updateLatestRevisions(); err != nil {
		return params.ErrorResult{Error: common.ServerError(err)}, nil
	}
	return params.ErrorResult{}, nil
}

func (api *CharmRevisionUpdaterAPI) updateLatestRevisions() error {
	// First get the uuid for the environment to use when querying the charm store.
	env, err := api.state.Model()
	if err != nil {
		return err
	}
	uuid := env.UUID()

	// Look up all the services in the model.
	services, err := api.state.AllServices()
	if err != nil {
		return err
	}

	// Get the handlers to use.
	handlers, err := listHandlers(api.state)
	if err != nil {
		return err
	}

	// Look up the information for all the deployed charms. This is the
	// "expensive" part.
	latest, err := retrieveLatestCharmInfo(services, uuid)
	if err != nil {
		return err
	}

	// Process the resulting info for each charm.
	for i, info := range latest {
		// First, add a charm placeholder to the model for each.
		if err = api.state.AddStoreCharmPlaceholder(info.LatestURL()); err != nil {
			return err
		}

		// Then run through the handlers.
		serviceID := services[i].ServiceTag()
		for _, handler := range handlers {
			if err := handler.HandleLatest(serviceID, info); err != nil {
				return err
			}
		}
	}

	return nil
}

// NewCharmStoreClient instantiates a new charm store repository.
// It is defined at top level for testing purposes.
var NewCharmStoreClient = func() charmstore.Client {
	// TODO(ericsnow) Use the Juju "HTTP context" once we have one.
	return charmstore.NewClient(nil)
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func retrieveLatestCharmInfo(services []*state.Service, uuid string) ([]charmstore.CharmInfo, error) {
	var curls []*charm.URL
	for _, service := range services {
		curl, _ := service.CharmURL()
		if curl.Schema == "local" {
			// Version checking for charms from local repositories is not
			// currently supported, since we don't yet support passing in
			// a path to the local repo. This may change if the need arises.
			continue
		}
		curls = append(curls, curl)
	}

	client := NewCharmStoreClient()

	results, err := charmstore.LatestCharmInfo(client, uuid, curls)
	if err != nil {
		return nil, err
	}

	var latest []charmstore.CharmInfo
	for i, result := range results {
		if result.Error != nil {
			logger.Errorf("retrieving charm info for %s: %v", curls[i], result.Error)
			continue
		}
		latest = append(latest, result.CharmInfo)
	}
	return latest, nil
}
