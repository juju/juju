// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/metricsmanager"
	jworker "github.com/juju/juju/worker"
)

const cleanupPeriod = time.Hour

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
func newCleanup(client metricsmanager.MetricsManagerClient, notify chan string, logger Logger) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			logger.Warningf("failed to cleanup %v - will retry later", err)
			return nil
		}
		select {
		case notify <- "cleanupCalled":
		default:
		}
		return nil
	}
	return jworker.NewPeriodicWorker(f, cleanupPeriod, jworker.NewTimer)
}
