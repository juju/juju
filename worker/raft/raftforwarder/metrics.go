// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftforwarder

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju_raftforwarder"
)

// metricsCollector is a prometheus.Collector that collects metrics
// about how long it's taking to forward requests to raft and get
// responses.
type metricsCollector struct {
	requests *prometheus.SummaryVec
}

func newMetricsCollector() *metricsCollector {
	return &metricsCollector{
		requests: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Name:      "request",
			Help:      "Request times for raft forwarder operations in ms",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, []string{
			// section can be "apply" (just the time to apply req and
			// get a response from raft) or "full" (total time from
			// request received to response sent)
			"section",
		}),
	}
}

func (m *metricsCollector) record(start time.Time, section string) {
	elapsedMS := float64(time.Now().Sub(start)) / float64(time.Millisecond)
	m.requests.With(prometheus.Labels{
		"section": section,
	}).Observe(elapsedMS)
}

// Describe is part of prometheus.Collector.
func (c *metricsCollector) Describe(ch chan<- *prometheus.Desc) {
	c.requests.Describe(ch)
}

// Collect is part of prometheus.Collector.
func (c *metricsCollector) Collect(ch chan<- prometheus.Metric) {
	c.requests.Collect(ch)
}
