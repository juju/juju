// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker"
)

// ProcessHandler returns an event handler that starts a worker for each
// tracked workload process. The worker waits until it is stopped.
func ProcessHandler(events []process.Event, apiClient context.APIClient, runner Runner) error {
	ph := newProcessHandler(apiClient, runner)
	if err := ph.handleEvents(events); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type processHandler struct {
	apiClient context.APIClient
	runner    Runner

	newWorker func() (worker.Worker, error)
}

func newProcessHandler(apiClient context.APIClient, runner Runner) *processHandler {
	ph := &processHandler{
		apiClient: apiClient,
		runner:    runner,
		newWorker: func() (worker.Worker, error) {
			return worker.NewNoOpWorker(), nil
		},
	}
	return ph
}

func (ph *processHandler) handleEvents(events []process.Event) error {
	for _, event := range events {
		if err := ph.handleEvent(event); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (ph *processHandler) handleEvent(event process.Event) error {
	switch event.Kind {
	case process.EventKindTracked:
		if err := ph.runner.StartWorker(event.ID, ph.newWorker); err != nil {
			return errors.Trace(err)
		}
	case process.EventKindUntracked:
		if err := ph.runner.StopWorker(event.ID); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
