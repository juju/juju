// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

	"github.com/juju/juju/resource/workers"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

// WorkerFactory is an implementation of cmd/jujud/agent.WorkerFactory
// for resources..
type WorkerFactory struct{}

// NewWorkerFactory returns a new worker factory for resources.
func NewWorkerFactory() *WorkerFactory {
	return &WorkerFactory{}
}

// NewModelWorker implements cmd/jujud/agent.WorkerFactory.
func (wf WorkerFactory) NewModelWorker(_ string, st *state.State) (newWorker func() (worker.Worker, error), supported bool) {
	wfs := &workerFactoryState{st: st}
	poller := workers.NewCharmStorePoller(wfs, newCharmStoreClient)
	newWorker = func() (worker.Worker, error) {
		return poller.NewWorker(), nil
	}
	return newWorker, true
}

func newCharmStoreClient() (workers.CharmStoreClient, error) {
	// TODO(ericsnow) Return an actual charm store client.
	return newFakeCharmStoreClient(nil), nil
}

type workerFactoryState struct {
	st *state.State
}

// ListAllServices returns all the services in the model.
func (wfs *workerFactoryState) ListAllServices() ([]workers.Service, error) {
	var services []workers.Service
	actual, err := wfs.st.AllServices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, svc := range actual {
		services = append(services, &service{svc})
	}
	return services, nil
}

// MarkOutdatedResources compares each of the service's resources
// against those provided and marks any outdated ones accordingly.
func (wfs *workerFactoryState) MarkOutdatedResources(serviceID string, info []charmresource.Resource) error {
	resources, err := wfs.st.Resources()
	if err != nil {
		return errors.Trace(err)
	}
	return resources.MarkOutdatedResources(serviceID, info)
}
