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
	ConnectionCount() int64
	ConcurrentLoginAttempts() int64
	ConnectionPauseTime() time.Duration
	ConnectionRate() int64
}

// Collector is a prometheus.Collector that collects metrics based
// on apiserver status.
type Collector struct {
	src ServerMetricsSource

	connectionCountGauge     prometheus.Gauge
	connectionRateGauge      prometheus.Gauge
	connectionPauseTimeGauge prometheus.Gauge
	concurrentLoginsGauge    prometheus.Gauge
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector(src ServerMetricsSource) *Collector {
	return &Collector{
		src: src,
		connectionCountGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connection_count",
			Help:      "Current number of apiserver connections",
		}),
		connectionRateGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connection_rate",
			Help:      "Rate per second of web socket connections",
		}),
		connectionPauseTimeGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "connection_pause_time",
			Help:      "Current wait time in milliseconds before accepting incoming connections",
		}),
		concurrentLoginsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: apiserverMetricsNamespace,
			Name:      "concurrent_login_attempts",
			Help:      "Current number of concurrent agent login attempts",
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.connectionCountGauge.Describe(ch)
	c.connectionRateGauge.Describe(ch)
	c.connectionPauseTimeGauge.Describe(ch)
	c.concurrentLoginsGauge.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.connectionCountGauge.Set(float64(c.src.ConnectionCount()))
	c.connectionRateGauge.Set(float64(c.src.ConnectionRate()))
	c.connectionPauseTimeGauge.Set(float64(c.src.ConnectionPauseTime() / time.Millisecond))
	c.concurrentLoginsGauge.Set(float64(c.src.ConcurrentLoginAttempts()))

	c.connectionCountGauge.Collect(ch)
	c.connectionRateGauge.Collect(ch)
	c.connectionPauseTimeGauge.Collect(ch)
	c.concurrentLoginsGauge.Collect(ch)
}
