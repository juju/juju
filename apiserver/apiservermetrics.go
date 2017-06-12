// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	apiserverMetricsNamespace = "juju_apiserver"
)

// ServerMetricsSource implementations provide apiserver metrics.
type ServerMetricsSource interface {
	TotalConnections() int64
	ConnectionCount() int64
	ConcurrentLoginAttempts() int64
	ConnectionPauseTime() time.Duration
}

// Collector is a prometheus.Collector that collects metrics based
// on apiserver status.
type Collector struct {
	src ServerMetricsSource

	connectionCounter        prometheus.Counter
	connectionCountGauge     prometheus.Gauge
	connectionPauseTimeGauge prometheus.Gauge
	concurrentLoginsGauge    prometheus.Gauge
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector(src ServerMetricsSource) *Collector {
	return &Collector{
		src: src,
		connectionCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connections_total",
			Help:      "Total number of apiserver connections ever made",
		}),
		connectionCountGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connection_count",
			Help:      "Current number of active apiserver connections",
		}),
		connectionPauseTimeGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connection_pause_seconds",
			Help:      "Current wait time in before accepting incoming connections",
		}),
		concurrentLoginsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "active_login_attempts",
			Help:      "Current number of active agent login attempts",
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.connectionCounter.Describe(ch)
	c.connectionCountGauge.Describe(ch)
	c.connectionPauseTimeGauge.Describe(ch)
	c.concurrentLoginsGauge.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.connectionCountGauge.Set(float64(c.src.ConnectionCount()))
	c.connectionPauseTimeGauge.Set(float64(c.src.ConnectionPauseTime()) / float64(time.Second))
	c.concurrentLoginsGauge.Set(float64(c.src.ConcurrentLoginAttempts()))

	ch <- prometheus.MustNewConstMetric(
		c.connectionCounter.Desc(),
		prometheus.CounterValue,
		float64(c.src.TotalConnections()),
	)
	c.connectionCountGauge.Collect(ch)
	c.connectionPauseTimeGauge.Collect(ch)
	c.concurrentLoginsGauge.Collect(ch)
}
