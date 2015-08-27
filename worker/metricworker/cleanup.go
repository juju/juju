// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/loggo"

	"github.com/juju/juju/api/metricsmanager"
	"github.com/juju/juju/worker"
)

var (
	cleanupLogger = loggo.GetLogger("juju.worker.metricworker.cleanup")
	notify        chan string
)

const (
	cleanupPeriod = time.Hour
)

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
func NewCleanup(client metricsmanager.MetricsManagerClient) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			cleanupLogger.Warningf("failed to cleanup %v - will retry later", err)
			return nil
		}
		select {
		case notify <- "cleanupCalled":
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, cleanupPeriod, worker.NewTimer)
}
