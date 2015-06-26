// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"time"

	corecharm "gopkg.in/juju/charm.v5"
)

const (
	// interval at which the unit's metrics should be collected
	metricsPollInterval = 5 * time.Minute

	// interval at which the uniter sends metrics to the state server.
	metricsSendInterval = 5 * time.Minute
)

// activeCollectMetricsTimer returns a channel that will signal the collect metrics hook
// as close to interval after the last run as possible.
var activeCollectMetricsTimer = func(now, lastRun time.Time, interval time.Duration) <-chan time.Time {
	waitDuration := interval - now.Sub(lastRun)
	logger.Debugf("metrics waiting for %v", waitDuration)
	return time.After(waitDuration)
}

var activeSendMetricsTimer = activeCollectMetricsTimer

// inactiveMetricsTimer is the default metrics signal generation function, that
// returns no signal. It will be used in charms that do not declare metrics.
func inactiveMetricsTimer(_, _ time.Time, _ time.Duration) <-chan time.Time {
	return nil
}

// timerChooser allows modeAbide to choose a proper timer for metrics
// depending on the charm.
type timerChooser struct {
	collector TimedSignal
	sender    TimedSignal
	inactive  TimedSignal
}

// getCollectMetricsTimer returns a timer used to trigger the collect-metrics hook,
// given the supplied charm.
func (t *timerChooser) getCollectMetricsTimer(ch corecharm.Charm) TimedSignal {
	metrics := ch.Metrics()
	if metrics != nil && len(metrics.Metrics) > 0 {
		return t.collector
	}
	return t.inactive
}

// getSendMetricsTimer returns a timer used to trigger sending metrics
// to the state server, given the supplied charm.
func (t *timerChooser) getSendMetricsTimer(ch corecharm.Charm) TimedSignal {
	metrics := ch.Metrics()
	if metrics != nil && len(metrics.Metrics) > 0 {
		return t.sender
	}
	return t.inactive
}

// NewMetricsTimerChooser returns a timerChooser for
// collect-metrics hook and the send-metrics operation.
func NewMetricsTimerChooser() *timerChooser {
	return &timerChooser{
		collector: activeCollectMetricsTimer,
		sender:    activeSendMetricsTimer,
		inactive:  inactiveMetricsTimer,
	}
}
