// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/observer/metricobserver"
)

const (
	apiserverMetricsNamespace   = "juju"
	apiserverSubsystemNamespace = "apiserver"
)

// MetricLabelEndpoint defines a constant for the APIConnections abd
// PingFailureCount Labels
const MetricLabelEndpoint = "endpoint"

// MetricLabelModelUUID defines a constant for the PingFailureCount and
// LogWriteCount Labels
// Note: prometheus doesn't allow hyphens only underscores
const MetricLabelModelUUID = "model_uuid"

// MetricLabelState defines a constant for the LogWriteCount Label
const MetricLabelState = "state"

// MetricAPIConnectionsLabelNames defines a series of labels for the
// APIConnections metric.
var MetricAPIConnectionsLabelNames = []string{
	MetricLabelEndpoint,
}

// MetricPingFailureLabelNames defines a series of labels for the PingFailure
// metric.
var MetricPingFailureLabelNames = []string{
	MetricLabelModelUUID,
	MetricLabelEndpoint,
}

// MetricLogLabelNames defines a series of labels for the LogWrite and LogRead
// metric
var MetricLogLabelNames = []string{
	MetricLabelModelUUID,
	MetricLabelState,
}

// Collector is a prometheus.Collector that collects metrics based
// on apiserver status.
type Collector struct {
	TotalConnections   prometheus.Counter
	LoginAttempts      prometheus.Gauge
	APIConnections     *prometheus.GaugeVec
	APIRequestDuration *prometheus.SummaryVec
	PingFailureCount   *prometheus.CounterVec
	LogWriteCount      *prometheus.CounterVec
	LogReadCount       *prometheus.CounterVec
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

		APIConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "connections",
			Help:      "Current number of active apiserver connections for api handlers",
		}, MetricAPIConnectionsLabelNames),
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
		PingFailureCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "ping_failure_count",
			Help:      "Current number of ping failures",
		}, MetricPingFailureLabelNames),
		LogWriteCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "log_write_count",
			Help:      "Current number of log writes",
		}, MetricLogLabelNames),
		LogReadCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Subsystem: apiserverSubsystemNamespace,
			Name:      "log_read_count",
			Help:      "Current number of log reads",
		}, MetricLogLabelNames),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.TotalConnections.Describe(ch)
	c.APIConnections.Describe(ch)
	c.LoginAttempts.Describe(ch)
	c.APIRequestDuration.Describe(ch)
	c.PingFailureCount.Describe(ch)
	c.LogWriteCount.Describe(ch)
	c.LogReadCount.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.TotalConnections.Collect(ch)
	c.APIConnections.Collect(ch)
	c.LoginAttempts.Collect(ch)
	c.APIRequestDuration.Collect(ch)
	c.PingFailureCount.Collect(ch)
	c.LogWriteCount.Collect(ch)
	c.LogReadCount.Collect(ch)
}
