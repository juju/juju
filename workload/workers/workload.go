// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

// WorkloadHandler returns an event handler that starts a worker for each
// tracked workload workload. The worker waits until it is stopped.
func WorkloadHandler(events []workload.Event, apiClient context.APIClient, runner Runner) error {
	ph := newWorkloadHandler(apiClient, runner)
	if err := ph.handleEvents(events); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type workloadHandler struct {
	apiClient context.APIClient
	runner    Runner

	newWorker func() (worker.Worker, error)
}

func newWorkloadHandler(apiClient context.APIClient, runner Runner) *workloadHandler {
	ph := &workloadHandler{
		apiClient: apiClient,
		runner:    runner,
		newWorker: func() (worker.Worker, error) {
			return worker.NewNoOpWorker(), nil
		},
	}
	return ph
}

func (ph *workloadHandler) handleEvents(events []workload.Event) error {
	for _, event := range events {
		if err := ph.handleEvent(event); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (ph *workloadHandler) handleEvent(event workload.Event) error {
	switch event.Kind {
	case workload.EventKindTracked:
		if err := ph.runner.StartWorker(event.ID, ph.newWorker); err != nil {
			return errors.Trace(err)
		}
	case workload.EventKindUntracked:
		if err := ph.runner.StopWorker(event.ID); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
