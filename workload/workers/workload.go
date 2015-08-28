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
	wh := newWorkloadHandler(apiClient, runner)
	if err := wh.handleEvents(events); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type workloadHandler struct {
	apiClient context.APIClient
	runner    Runner
}

func newWorkloadHandler(apiClient context.APIClient, runner Runner) *workloadHandler {
	wh := &workloadHandler{
		apiClient: apiClient,
		runner:    runner,
	}
	return wh
}

func (wh *workloadHandler) handleEvents(events []workload.Event) error {
	for _, event := range events {
		if err := wh.handleEvent(event); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (wh *workloadHandler) handleEvent(event workload.Event) error {
	name := "workload <" + event.ID + ">"
	switch event.Kind {
	case workload.EventKindTracked:
		if err := wh.runner.StartWorker(name, wh.newWorker); err != nil {
			return errors.Trace(err)
		}
	case workload.EventKindUntracked:
		if err := wh.runner.StopWorker(name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (wh *workloadHandler) newWorker() (worker.Worker, error) {
	// TODO(ericsnow) Start up a runner or an engine, and run
	// workload-specific workers under it?
	return worker.NewNoOpWorker(), nil
}
