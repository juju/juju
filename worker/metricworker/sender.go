// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricworker

import (
	"time"

	"github.com/juju/worker/v2"

	"github.com/juju/juju/api/metricsmanager"
	jworker "github.com/juju/juju/worker"
)

const senderPeriod = 5 * time.Minute

// NewSender creates a new periodic worker that sends metrics
// to a collection service.
func newSender(client metricsmanager.MetricsManagerClient, notify chan string, logger Logger) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.SendMetrics()
		if err != nil {
			logger.Warningf("failed to send metrics %v - will retry later", err)
			return nil
		}
		select {
		case notify <- "senderCalled":
		default:
		}
		return nil
	}
	return jworker.NewPeriodicWorker(f, senderPeriod, jworker.NewTimer)
}
