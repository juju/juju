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
	notify               chan string
)

const (
	workloadUpdatePeriod = time.Minute * 5
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

func statusTracked(event process.Event, api context.APIClient, runner Runner) error {
	f := func(stopCh <-chan struct{}) error {
		pluginStatus, err := event.Plugin.Status(event.ID)
		if err != nil {
			workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
		}

		// TODO(wwitzel3) define this based on pluginStatus values
		var status process.Status

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
	worker := func() (worker.Worker, error) {
		return worker.NewPeriodicWorker(f, workloadUpdatePeriod), nil
	}
	return runner.StartWorker(event.ID, worker)
}

func statusUntracked(event process.Event, api context.APIClient, runner Runner) error {
	pluginStatus, err := event.Plugin.Status(event.ID)
	if err != nil {
		workloadUpdateLogger.Warningf("failed to get status %v - will retry later", err)
	}

	//TODO(wwitzel3) I figure we can just ignore the pluginStatus values?
	var status process.Status
	status.State = process.StateStopping
	status.Message = fmt.Sprintf("%s is no longer being tracked", event.ID)

	if err := api.SetProcessesStatus(status, pluginStatus, event.ID); err != nil {
		workloadUpdateLogger.Warningf("failed to update - %v while stopping", err)
	}

	return runner.StopWorker(event.ID)
}
