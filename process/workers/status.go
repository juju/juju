// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/worker"
)

var (
	workloadUpdateLogger = loggo.GetLogger("juju.process.worker.status.update")
	workloadUpdatePeriod = time.Minute * 5
	notify               chan string
)

// StatusEventHandler handles and dispatches process.Events based on their kind.
func StatusEventHandler(events []process.Event, api context.APIClient, runner Runner) error {
	for _, e := range events {
		switch e.Kind {
		case process.EventKindTracked:
			if err := statusTracked(e, api, runner); err != nil {
				return errors.Trace(err)
			}
		case process.EventKindUntracked:
			if err := statusUntracked(e, api, runner); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// NewStatusWorkerFunc returns a function that you can use with a PeriodicWorker.
func NewStatusWorkerFunc(event process.Event, api context.APIClient) func(<-chan struct{}) error {
	//TODO(wwitzel3) Should this be set based on the plugin response?
	// Or is this Juju state of the workload, which is Running
	var status process.Status
	status.State = process.StateRunning
	status.Message = fmt.Sprintf("%s is being tracked", event.ID)

	call := func(stopCh <-chan struct{}) error {
		pluginStatus, err := event.Plugin.Status(event.ID)
		if err != nil {
			workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
		}

		err = api.SetProcessesStatus(status, pluginStatus, event.ID)
		if err != nil {
			workloadUpdateLogger.Warningf("failed to update %v - will retry later", err)
		}

		select {
		case notify <- "updateCalled":
		default:
		}
		return nil
	}
	return call
}

func SetStatusWorkerUpdatePeriod(t time.Duration) {
	workloadUpdatePeriod = t
}

// NewStatusWorker returns a new PeriodicWorker.
func NewStatusWorker(event process.Event, api context.APIClient) worker.Worker {
	f := NewStatusWorkerFunc(event, api)
	return worker.NewPeriodicWorker(f, workloadUpdatePeriod)
}

func statusTracked(event process.Event, api context.APIClient, runner Runner) error {
	worker := func() (worker.Worker, error) {
		return NewStatusWorker(event, api), nil
	}

	workloadUpdateLogger.Infof("%v is being tracked", event.ID)
	return runner.StartWorker(event.ID, worker)
}

func statusUntracked(event process.Event, api context.APIClient, runner Runner) error {
	pluginStatus, err := event.Plugin.Status(event.ID)
	if err != nil {
		workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
	}

	//TODO(wwitzel3) Is this the right status to set?
	var status process.Status
	status.State = process.StateStopping
	status.Message = fmt.Sprintf("%s is no longer being tracked", event.ID)

	if err := api.SetProcessesStatus(status, pluginStatus, event.ID); err != nil {
		workloadUpdateLogger.Warningf("failed to update - %v while stopping", err)
	}

	return runner.StopWorker(event.ID)
}
