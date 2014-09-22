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
	notify        chan struct{}
)

// NewCleanup creates a new periodic worker that calls the CleanupOldMetrics api.
func NewCleanup(client *metricsmanager.Client) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.CleanupOldMetrics()
		if err != nil {
			cleanupLogger.Warningf("failed to cleanup %v - will retry later", err)
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
