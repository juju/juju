// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/state/api/metricsmanager"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.metricworker.cleanup")

type MetricCleanupWorker struct {
	work worker.Worker
}

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
// If a notify channel is provided it will be signalled everytime the call is made.
func NewCleanup(client *metricsmanager.Client, notify chan struct{}) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			logger.Errorf("failed to cleanup %v", err)
			return nil // We failed this time, but we'll retry later
		}
		select {
		case notify <- struct{}{}:
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, time.Second)
}
