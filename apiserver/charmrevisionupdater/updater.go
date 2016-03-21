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
		serviceID := info.service.ServiceTag()
		for _, handler := range handlers {
			if err := handler.HandleLatest(serviceID, info.CharmInfo); err != nil {
				return err
			}
		}
	}

	return nil
}

// NewCharmStoreClientConfig returns the client config to use.
// It is defined at top level for testing purposes.
var NewCharmStoreClientConfig = func() charmstore.ClientConfig {
	var config charmstore.ClientConfig
	return config
}

// newCharmStoreClient instantiates a new charm store repository.
func newCharmStoreClient(modelUUID string) (*charmstore.Client, error) {
	// TODO(ericsnow) Use the Juju "HTTP context" once we have one.
	config := NewCharmStoreClientConfig()
	client := charmstore.NewClient(config)
	return client.WithMetadata(charmstore.JujuMetadata{
		ModelUUID: modelUUID,
	})
}

type latestCharmInfo struct {
	charmstore.CharmInfo
	service *state.Service
}

// retrieveLatestCharmInfo looks up the charm store to return the charm URLs for the
// latest revision of the deployed charms.
func retrieveLatestCharmInfo(st *state.State) ([]latestCharmInfo, error) {
	// First get the uuid for the environment to use when querying the charm store.
	env, err := st.Model()
	if err != nil {
		return nil, err
	}
	modelUUID := env.UUID()

	services, err := st.AllServices()
	if err != nil {
		return nil, err
	}

	var curls []*charm.URL
	var resultsIndexedServices []*state.Service
	for _, service := range services {
		curl, _ := service.CharmURL()

		if curl.Schema == "local" {
			// Version checking for charms from local repositories is not
			// currently supported, since we don't yet support passing in
			// a path to the local repo. This may change if the need arises.
			continue
		}
		curls = append(curls, curl)
		resultsIndexedServices = append(resultsIndexedServices, service)
	}

	client, err := newCharmStoreClient(modelUUID)
	if err != nil {
		return nil, err
	}

	// TODO(natefinch): get the real channel when we have one.
	charms := make([]charmstore.CharmID, len(curls))
	for i, c := range curls {
		charms[i] = charmstore.CharmID{URL: c, Channel: "stable"}
	}

	results, err := charmstore.LatestCharmInfo(client, charms)
	if err != nil {
		return nil, err
	}

	var latest []latestCharmInfo
	for i, result := range results {
		if result.Error != nil {
			logger.Errorf("retrieving charm info for %s: %v", curls[i], result.Error)
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
