// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"

	corecharm "gopkg.in/juju/charm.v5-unstable"
)

const (
	// interval at which the unit's metrics should be collected
	metricsPollInterval = 5 * time.Minute
)

// CollectMetricsSignal is the signature of the function used to generate a
// collect-metrics signal.
type CollectMetricsSignal func(now, lastSignal time.Time, interval time.Duration) <-chan time.Time

// activeMetricsTimer returns a channel that will signal the collect metrics hook
// as close to interval after the last run as possible.
var activeMetricsTimer = func(now, lastRun time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastRun)
	logger.Debugf("waiting for %v", waitDuration)
	return time.After(waitDuration)
}

// inactiveMetricsTimer is the default metrics signal generation function, that
// returns no signal. It will be used in charms that do not declare metrics.
func inactiveMetricsTimer(_, _ time.Time, _ time.Duration) <-chan time.Time {
	return nil
}

// getMetricsTimer returns the metrics timer we should be using, given the supplied
// charm.
func getMetricsTimer(ch corecharm.Charm) CollectMetricsSignal {
	metrics := ch.Metrics()
	if metrics != nil && len(metrics.Metrics) > 0 {
		return activeMetricsTimer
	}
	return inactiveMetricsTimer
}
