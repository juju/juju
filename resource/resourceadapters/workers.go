// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"time"

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
func (wf WorkerFactory) NewModelWorker(st *state.State) func() (worker.Worker, error) {
	wfs := &workerFactoryState{st: st}
	csOpener := charmstoreOpener{}
	poller := workers.NewCharmStorePoller(wfs, func() (workers.CharmStoreClient, error) {
		return csOpener.NewClient()
	})
	newWorker := func() (worker.Worker, error) {
		return poller.NewWorker(), nil
	}
	return newWorker
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

// SetCharmStoreResources sets the "polled from the charm store"
// resources for the service to the provided values.
func (wfs *workerFactoryState) SetCharmStoreResources(serviceID string, info []charmresource.Resource, lastPolled time.Time) error {
	resources, err := wfs.st.Resources()
	if err != nil {
		return errors.Trace(err)
	}
	return resources.SetCharmStoreResources(serviceID, info, lastPolled)
}
