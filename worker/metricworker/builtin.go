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
	unitTimeLogger = loggo.GetLogger("juju.worker.metricworker.unittime")
	unitTimeMetric = "juju-unit-time"
)

const (
	bultinPeriod = 5 * time.Minute
)

// NewBuiltinMetricAdder creates a new periodic worker that adds builtin metrics.
func NewBuiltinMetricAdder(client metricsmanager.MetricsManagerClient) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		err := client.AddBuiltinMetrics()
		if err != nil {
			senderLogger.Warningf("failed to add builtin metrics %v - will retry later", err)
			return nil
		}
		select {
		case notify <- struct{}{}:
		default:
		}
		return nil
	}
	return worker.NewPeriodicWorker(f, bultinPeriod)
}
