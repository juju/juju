// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.charmrevisionupdater")

func init() {
	common.RegisterStandardFacade("CharmRevisionUpdater", 2, NewCharmRevisionUpdaterAPI)
}

// CharmRevisionUpdater defines the methods on the charmrevisionupdater API end point.
type CharmRevisionUpdater interface {
	UpdateLatestRevisions() (params.ErrorResult, error)
}

// CharmRevisionUpdaterAPI implements the CharmRevisionUpdater interface and is the concrete
// implementation of the api end point.
type CharmRevisionUpdaterAPI struct {
	state      *state.State
	resources  facade.Resources
	authorizer facade.Authorizer
}

var _ CharmRevisionUpdater = (*CharmRevisionUpdaterAPI)(nil)

// NewCharmRevisionUpdaterAPI creates a new server-side charmrevisionupdater API end point.
func NewCharmRevisionUpdaterAPI(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
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
	// Get the handlers to use.
	handlers, err := createHandlers(api.state)
	if err != nil {
		return err
	}

	// Look up the information for all the deployed charms. This is the
	// "expensive" part.
	latest, err := retrieveLatestCharmInfo(api.state)
	if err != nil {
		return err
	}

	// Process the resulting info for each charm.
	for _, info := range latest {
		// First, add a charm placeholder to the model for each.
		if err = api.state.AddStoreCharmPlaceholder(info.LatestURL()); err != nil {
			return err
		}

		// Then run through the handlers.
		serviceID := info.service.ApplicationTag()
		for _, handler := range handlers {
			if err := handler.HandleLatest(serviceID, info.CharmInfo); err != nil {
				return err
			}
		}
	}

	return nil
}

// NewCharmStoreClient instantiates a new charm store repository.  Exported so
// we can change it during testing.
var NewCharmStoreClient = func(st *state.State) (charmstore.Client, error) {
	return charmstore.NewCachingClient(state.MacaroonCache{st}, nil)
}

type latestCharmInfo struct {
	charmstore.CharmInfo
	service *state.Application
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func retrieveLatestCharmInfo(st *state.State) ([]latestCharmInfo, error) {
	// First get the uuid for the environment to use when querying the charm store.
	env, err := st.Model()
	if err != nil {
		return nil, err
	}

	services, err := st.AllApplications()
	if err != nil {
		return nil, err
	}

	client, err := NewCharmStoreClient(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var charms []charmstore.CharmID
	var resultsIndexedServices []*state.Application
	for _, service := range services {
		curl, _ := service.CharmURL()
		if curl.Schema == "local" {
			// Version checking for charms from local repositories is not
			// currently supported, since we don't yet support passing in
			// a path to the local repo. This may change if the need arises.
			continue
		}

		cid := charmstore.CharmID{
			URL:     curl,
			Channel: service.Channel(),
		}
		charms = append(charms, cid)
		resultsIndexedServices = append(resultsIndexedServices, service)
	}

	metadata := map[string]string{
		"environment_uuid": env.UUID(),
		"cloud":            env.Cloud(),
		"cloud_region":     env.CloudRegion(),
	}
	cloud, err := st.Cloud(env.Cloud())
	if err != nil {
		metadata["provider"] = "unknown"
	} else {
		metadata["provider"] = cloud.Type
	}
	results, err := charmstore.LatestCharmInfo(client, charms, metadata)
	if err != nil {
		return nil, err
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if result.Error != nil {
			logger.Errorf("retrieving charm info for %s: %v", charms[i].URL, result.Error)
			continue
		}
		service := resultsIndexedServices[i]
		latest = append(latest, latestCharmInfo{
			CharmInfo: result.CharmInfo,
			service:   service,
		})
	}
	return latest, nil
}
