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
	senderLogger = loggo.GetLogger("juju.worker.metricworker.sender")
)

const (
	senderPeriod = 5 * time.Minute
)

// NewSender creates a new periodic worker that sends metrics
// to a collection service.
func NewSender(client metricsmanager.MetricsManagerClient) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.SendMetrics()
		if err != nil {
			senderLogger.Warningf("failed to send metrics %v - will retry later", err)
			return nil
		}
		select {
		case notify <- "senderCalled":
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, senderPeriod, worker.NewTimer)
}
