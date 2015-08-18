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
}

func newProcessHandler(apiClient context.APIClient, runner Runner) *processHandler {
	ph := &processHandler{
		apiClient: apiClient,
		runner:    runner,
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
	name := "proc <" + event.ID + ">"
	switch event.Kind {
	case process.EventKindTracked:
		if err := ph.runner.StartWorker(name, ph.newWorker); err != nil {
			return errors.Trace(err)
		}
	case process.EventKindUntracked:
		if err := ph.runner.StopWorker(name); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (ph *processHandler) newWorker() (worker.Worker, error) {
	// TODO(ericsnow) Start up a runner or an engine, and run
	// proc-specific workers under it?
	return worker.NewNoOpWorker(), nil
}
