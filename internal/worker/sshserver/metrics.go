// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "juju_sshserver"

// Collector is a prometheus.Collector that collects metrics about
// sshserver worker.
type Collector struct {
	connectionCount        prometheus.Gauge
	timeToSession          *prometheus.HistogramVec
	connectionDuration     prometheus.Histogram
	authenticationFailures *prometheus.CounterVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		connectionCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "connection_count",
				Help:      "The number of active connections to the SSH server.",
			},
		),
		timeToSession: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Name:      "time_to_session",
				Help:      "The time taken to establish a SSH session.",
				Buckets:   []float64{0.1, 0.5, 1, 2, 5, 10, 20, 60},
			}, []string{"model_type"},
		),
		connectionDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: metricsNamespace,
				Name:      "session_time",
				Help:      "The duration a user keeps an SSH connection open.",
				Buckets:   []float64{1, 10, 60, 300, 600, 3600},
			},
		),
		authenticationFailures: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: metricsNamespace,
				Name:      "authentication_failures",
				Help:      "The number of authentication failures.",
			}, []string{"auth_method"},
		),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.connectionCount.Describe(ch)
	c.authenticationFailures.Describe(ch)
	c.timeToSession.Describe(ch)
	c.connectionDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.connectionCount.Collect(ch)
	c.authenticationFailures.Collect(ch)
	c.timeToSession.Collect(ch)
	c.connectionDuration.Collect(ch)
}
