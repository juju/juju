// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	apiserverMetricsNamespace   = "juju"
	apiserverSubsystemNamespace = "apiserver"
	// TODO (stickupkid): remove this deprecated subsystem in 2.6+
	deprecatedSubsystemNamespace = "api"
)

// Collector is a prometheus.Collector that collects metrics based
// on apiserver status.
type Collector struct {
	TotalConnections   prometheus.Counter
	APIConnections     prometheus.Gauge
	LogsinkConnections prometheus.Gauge
	LoginAttempts      prometheus.Gauge
	APIRequestDuration *prometheus.SummaryVec

	DeprecatedAPIRequestsTotal   *prometheus.CounterVec
	DeprecatedAPIRequestDuration *prometheus.SummaryVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		TotalConnections: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "connections_total",
			Help:      "Total number of apiserver connections ever made",
		}),
		APIConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "connection_count",
			Help:      "Current number of active apiserver connections",
		}),
		LogsinkConnections: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "connection_count_logsink",
			Help:      "Current number of active apiserver connections for logsink",
		}),
		LoginAttempts: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "active_login_attempts",
			Help:      "Current number of active agent login attempts",
		}),
		APIRequestDuration: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "request_duration_seconds",
			Help:      "Latency of Juju API requests in seconds.",
		}, metricobserver.MetricLabelNames),

		// TODO (stickupkid): remove post 2.6 release
		DeprecatedAPIRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: deprecatedSubsystemNamespace,
			Name:      "requests_total",
			Help:      "Number of Juju API requests served.",
		}, metricobserver.MetricLabelNames),
		DeprecatedAPIRequestDuration: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: deprecatedSubsystemNamespace,
			Name:      "request_duration_seconds",
			Help:      "Latency of Juju API requests in seconds.",
		}, metricobserver.MetricLabelNames),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.TotalConnections.Describe(ch)
	c.APIConnections.Describe(ch)
	c.LogsinkConnections.Describe(ch)
	c.LoginAttempts.Describe(ch)
	c.APIRequestDuration.Describe(ch)

	// TODO (stickupkid): remove post 2.6 release
	c.DeprecatedAPIRequestsTotal.Describe(ch)
	c.DeprecatedAPIRequestDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.TotalConnections.Collect(ch)
	c.APIConnections.Collect(ch)
	c.LogsinkConnections.Collect(ch)
	c.LoginAttempts.Collect(ch)
	c.APIRequestDuration.Collect(ch)

	// TODO (stickupkid): remove post 2.6 release
	c.DeprecatedAPIRequestsTotal.Collect(ch)
	c.DeprecatedAPIRequestDuration.Collect(ch)
}
