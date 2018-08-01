// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package monitoring

import (
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	serverLabel  = "server"
	timeoutLabel = "timeout"
)

var dialLabels = []string{serverLabel, failedLabel, timeoutLabel}

// DialCollector is a prometheus.Collector that collects MongoDB
// connection dialing metrics.
type DialCollector struct {
	dialsTotal   *prometheus.CounterVec
	dialDuration *prometheus.SummaryVec
}

// NewDialCollector returns a new DialCollector.
func NewDialCollector(namespace string) *DialCollector {
	return &DialCollector{
		dialsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "mongo_dials_total",
			Help:      "Total number of MongoDB server dials.",
		}, dialLabels),

		dialDuration: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: namespace,
			Name:      "mongo_dial_duration_seconds",
			Help:      "Time taken dialng MongoDB server.",
		}, dialLabels),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *DialCollector) Describe(ch chan<- *prometheus.Desc) {
	c.dialsTotal.Describe(ch)
	c.dialDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *DialCollector) Collect(ch chan<- prometheus.Metric) {
	c.dialsTotal.Collect(ch)
	c.dialDuration.Collect(ch)
}

// PostDialServer is a function that may be used in
// mongo.DialOpts.PostDialServer, to update metrics.
func (c *DialCollector) PostDialServer(server string, duration time.Duration, dialErr error) {
	var failedValue, timeoutValue string
	if dialErr != nil {
		// TODO(axw) attempt to distinguish more types of
		// errors, e.g. failure due to TLS handshake vs. net
		// dial.
		failedValue = "failed"
		if err, ok := dialErr.(*net.OpError); ok {
			failedValue = err.Op
		}
		if err, ok := dialErr.(net.Error); ok {
			if err.Timeout() {
				timeoutValue = "timed out"
			}
		}
	}
	labels := prometheus.Labels{
		serverLabel:  server,
		failedLabel:  failedValue,
		timeoutLabel: timeoutValue,
	}
	c.dialsTotal.With(labels).Inc()
	c.dialDuration.With(labels).Observe(duration.Seconds())
}
