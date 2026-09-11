// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "juju_sshtunnel"

// Collector collects SSH tunnel and relay upgrade connection metrics.
type Collector struct {
	connectionCount *prometheus.GaugeVec
}

// NewMetricsCollector returns a collector for SSH tunnel and relay
// endpoints.
func NewMetricsCollector() *Collector {
	return &Collector{
		connectionCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "connection_count",
			Help:      "The number of active SSH tunnel or relay upgrade connections.",
		}, []string{"endpoint"}),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.connectionCount.Describe(ch)
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.connectionCount.Collect(ch)
}

// IncConnectionCount increments the active connection count for the
// given endpoint.
func (c *Collector) IncConnectionCount(endpoint string) {
	c.connectionCount.WithLabelValues(endpoint).Inc()
}

// DecConnectionCount decrements the active connection count for the
// given endpoint.
func (c *Collector) DecConnectionCount(endpoint string) {
	c.connectionCount.WithLabelValues(endpoint).Dec()
}
