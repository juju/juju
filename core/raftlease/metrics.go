// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju_raftlease"
)

// metricsCollector is a prometheus.Collector that collects metrics
// about lease store operations.
type metricsCollector struct {
	requests *prometheus.SummaryVec
}

func newMetricsCollector() *metricsCollector {
	return &metricsCollector{
		requests: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Name:      "request",
			Help:      "Request times for lease store operations in ms",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, []string{
			"operation", // claim, extend, pin, unpin or settime
			"result",    // success, failure, timeout or error
		}),
	}
}

// Describe is part of prometheus.Collector.
func (c *metricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.requests.Describe(ch)
}

// Collect is part of prometheus.Collector.
func (c *metricsCollector) Collect(ch chan<- prometheus.Metric) {
	c.requests.Collect(ch)
}
