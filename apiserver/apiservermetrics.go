// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	apiserverMetricsNamespace = "juju"
	apiserverMetricsSubsystem = "apiserver"
)

// Collector is a prometheus.Collector that collects metrics based
// on apiserver status.
type Collector struct {
	TotalConnections   prometheus.Counter
	APIConnections     prometheus.Gauge
	LogsinkConnections prometheus.Gauge
	LoginAttempts      prometheus.Gauge
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		TotalConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverMetricsSubsystem,
			Name:      "connections_total",
			Help:      "Total number of apiserver connections ever made",
		}),
		APIConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverMetricsSubsystem,
			Name:      "connection_count",
			Help:      "Current number of active apiserver connections",
		}),
		LogsinkConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverMetricsSubsystem,
			Name:      "connection_count_logsink",
			Help:      "Current number of active apiserver connections for logsink",
		}),
		LoginAttempts: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverMetricsSubsystem,
			Name:      "active_login_attempts",
			Help:      "Current number of active agent login attempts",
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.TotalConnections.Describe(ch)
	c.APIConnections.Describe(ch)
	c.LogsinkConnections.Describe(ch)
	c.LoginAttempts.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.TotalConnections.Collect(ch)
	c.APIConnections.Collect(ch)
	c.LogsinkConnections.Collect(ch)
	c.LoginAttempts.Collect(ch)
}
