// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

var (
	workloadUpdateLogger = loggo.GetLogger("juju.workload.worker.status.update")
	workloadUpdatePeriod = time.Minute * 5
	notify               chan string
)

// StatusEventHandler handles and dispatches workload.Events based on their kind.
func StatusEventHandler(events []workload.Event, api context.APIClient, runner Runner) error {
	workloadUpdateLogger.Debugf("handling events")
	for _, e := range events {
		workloadUpdateLogger.Debugf("handling %#v event", e)
		switch e.Kind {
		case workload.EventKindTracked:
			if err := statusTracked(e, api, runner); err != nil {
				return errors.Trace(err)
			}
		case workload.EventKindUntracked:
			if err := statusUntracked(e, api, runner); err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

// NewStatusWorkerFunc returns a function that you can use with a PeriodicWorker.
func NewStatusWorkerFunc(event workload.Event, api context.APIClient) func(<-chan struct{}) error {
	//TODO(wwitzel3) Should this be set based on the plugin response?
	// Or is this Juju state of the workload, which is Running
	var status workload.Status
	status.State = workload.StateRunning
	status.Message = fmt.Sprintf("%s is being tracked", event.ID)

	call := func(stopCh <-chan struct{}) error {
		pluginStatus, err := event.Plugin.Status(event.PluginID)
		if err != nil {
			workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
		}

		err = api.SetStatus(status, pluginStatus, event.ID)
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
func NewStatusWorker(event workload.Event, api context.APIClient) worker.Worker {
	workloadUpdateLogger.Debugf("starting status worker")
	f := NewStatusWorkerFunc(event, api)
	return worker.NewPeriodicWorker(f, workloadUpdatePeriod, worker.NewTimer)
}

func statusTracked(event workload.Event, api context.APIClient, runner Runner) error {
	worker := func() (worker.Worker, error) {
		return NewStatusWorker(event, api), nil
	}

	workloadUpdateLogger.Infof("%s is being tracked", event.ID)
	return runner.StartWorker(event.ID, worker)
}

func statusUntracked(event workload.Event, api context.APIClient, runner Runner) error {
	pluginStatus, err := event.Plugin.Status(event.PluginID)
	if err != nil {
		workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
	}

	//TODO(wwitzel3) Is this the right status to set?
	var status workload.Status
	status.State = workload.StateStopping
	status.Message = fmt.Sprintf("%s is no longer being tracked", event.ID)

	if err := api.SetStatus(status, pluginStatus, event.ID); err != nil {
		workloadUpdateLogger.Warningf("failed to update - %v while stopping", err)
	}
	workloadUpdateLogger.Infof("%s is no longer being tracked", event.ID)

	return runner.StopWorker(event.ID)
}
