// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.metricworker.cleanup")

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
// If a notify channel is provided it will be signalled everytime the call is made.
func NewCleanup(client *metricsmanager.Client, notify chan struct{}) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			logger.Warningf("failed to cleanup %v - will retry later", err)
			return nil
		}
		select {
		case notify <- struct{}{}:
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, time.Second)
}
