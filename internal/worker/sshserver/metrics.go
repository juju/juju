// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import "github.com/prometheus/client_golang/prometheus"

const metricsNamespace = "juju_sshserver"

// Collector collects SSH server connection and authentication metrics.
type Collector struct {
	connectionCount        prometheus.Gauge
	connectionDuration     prometheus.Histogram
	timeToSession          *prometheus.HistogramVec
	authenticationFailures *prometheus.CounterVec
}

// NewMetricsCollector returns a collector for an SSH server worker.
func NewMetricsCollector() *Collector {
	return &Collector{
		connectionCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "connection_count",
			Help:      "The number of active connections to the SSH server.",
		}),
		connectionDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "connection_duration_seconds",
			Help:      "The time an SSH connection remains open.",
			Buckets:   []float64{1, 10, 60, 300, 600, 3600},
		}),
		timeToSession: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "time_to_session_seconds",
			Help:      "The time taken to establish an SSH session.",
			Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30},
		}, []string{"model_type"}),
		authenticationFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "authentication_failures",
			Help:      "The number of rejected SSH authentication attempts.",
		}, []string{"auth_method"}),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.connectionCount.Describe(ch)
	c.connectionDuration.Describe(ch)
	c.timeToSession.Describe(ch)
	c.authenticationFailures.Describe(ch)
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.connectionCount.Collect(ch)
	c.connectionDuration.Collect(ch)
	c.timeToSession.Collect(ch)
	c.authenticationFailures.Collect(ch)
}

// IncConnectionCount increments the active connection count.
func (c *Collector) IncConnectionCount() {
	c.connectionCount.Inc()
}

// DecConnectionCount decrements the active connection count and records
// the connection duration.
func (c *Collector) DecConnectionCount() {
	c.connectionCount.Dec()
}
