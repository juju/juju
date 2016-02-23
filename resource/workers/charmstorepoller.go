// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/worker"
)

const charmStorePollPeriod = 24 * time.Hour

// Service exposes the functionality of the Juju entity needed here.
type Service interface {
	// ID identifies the service in the model.
	ID() names.ServiceTag

	// CharmURL identifies the service's charm.
	CharmURL() *charm.URL
}

// DataStore exposes the functionality of Juju state needed here.
type DataStore interface {
	// ListAllServices returns all the services in the model.
	ListAllServices() ([]Service, error)

	// SetCharmStoreResources sets the "polled from the charm store"
	// resources for the service to the provided values.
	SetCharmStoreResources(serviceID string, info []charmresource.Resource, lastPolled time.Time) error
}

// CharmStoreClient exposes the functionality of the charm store
// needed here.
type CharmStoreClient interface {
	io.Closer

	// ListResources returns the resources info for each identified charm.
	ListResources([]*charm.URL) ([][]charmresource.Resource, error)
}

// CharmStorePoller provides the functionality to poll the charm store
// for changes in resources in the Juju model.
type CharmStorePoller struct {
	CharmStorePollerDeps

	// Period is the time between poll attempts.
	Period time.Duration
}

// NewCharmStorePoller returns a charm store poller that uses the
// provided data store.
func NewCharmStorePoller(st DataStore, newClient func() (CharmStoreClient, error)) *CharmStorePoller {
	deps := &csPollerDeps{
		DataStore: st,
		newClient: newClient,
	}
	return &CharmStorePoller{
		CharmStorePollerDeps: deps,
		Period:               charmStorePollPeriod,
	}
}

// NewWorker returns a new periodic worker for the poller.
func (csp CharmStorePoller) NewWorker() worker.Worker {
	// TODO(ericsnow) Wrap Do() in a retry? Log the error instead of
	// returning it?
	return csp.NewPeriodicWorker(csp.Do, csp.Period)
}

func shouldStop(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

// Do performs a single polling iteration.
func (csp CharmStorePoller) Do(stop <-chan struct{}) error {
	services, err := csp.ListAllServices()
	if err != nil {
		return errors.Trace(err)
	}
	if shouldStop(stop) {
		return nil
	}

	var cURLs []*charm.URL
	for _, service := range services {
		cURL := service.CharmURL()
		if cURL == nil {
			continue
		}
		cURLs = append(cURLs, cURL)
	}
	if shouldStop(stop) {
		return nil
	}

	chResources, err := csp.ListCharmStoreResources(cURLs)
	if err != nil {
		return errors.Trace(err)
	}

	lastPolled := time.Now().UTC()
	// Note: since we used "services" to compose the list of charm URL
	// s passed to the charm store client, there is a one-to-one
	// correspondence between "services" and "chResources".
	for i, service := range services {
		if shouldStop(stop) {
			return nil
		}

		serviceID := service.ID().Id()
		if err := csp.SetCharmStoreResources(serviceID, chResources[i], lastPolled); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

// CharmStorePollerDeps exposes the external dependencies of a charm
// store poller.
type CharmStorePollerDeps interface {
	DataStore

	// NewPeriodicWorker returns a new periodic worker.
	NewPeriodicWorker(func(stop <-chan struct{}) error, time.Duration) worker.Worker

	// ListCharmStoreResources returns the resources from the charm
	// store for each of the identified charms.
	ListCharmStoreResources([]*charm.URL) ([][]charmresource.Resource, error)
}

type csPollerDeps struct {
	DataStore
	newClient func() (CharmStoreClient, error)
}

// NewPeriodicWorker implements CharmStorePollerDeps.
func (csPollerDeps) NewPeriodicWorker(call func(stop <-chan struct{}) error, period time.Duration) worker.Worker {
	return worker.NewPeriodicWorker(call, period, worker.NewTimer)
}

// ListCharmStoreResources implements CharmStorePollerDeps.
func (deps csPollerDeps) ListCharmStoreResources(cURLs []*charm.URL) ([][]charmresource.Resource, error) {
	client, err := deps.newClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer client.Close()

	chResources, err := client.ListResources(cURLs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return chResources, nil
}
