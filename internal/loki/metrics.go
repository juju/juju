// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package loki

import "github.com/prometheus/client_golang/prometheus"

const (
	lokiMetricsNamespace = "juju"
	lokiMetricsSubsystem = "loki_forwarder"
)

// MetricsSource provides the values exported by the Loki metrics collector.
type MetricsSource interface {
	Sent() uint64
	Dropped() uint64
	PushErrors() uint64
}

// Collector exposes Loki delivery metrics.
type Collector struct {
	sentDesc       *prometheus.Desc
	droppedDesc    *prometheus.Desc
	pushErrorsDesc *prometheus.Desc
	client         MetricsSource
}

// NewMetricsCollector returns a collector for the supplied client.
func NewMetricsCollector(client MetricsSource) *Collector {
	return &Collector{
		sentDesc: prometheus.NewDesc(
			prometheus.BuildFQName(
				lokiMetricsNamespace,
				lokiMetricsSubsystem,
				"sent_total",
			),
			"Total number of log records successfully sent to Loki.",
			nil,
			nil,
		),
		droppedDesc: prometheus.NewDesc(
			prometheus.BuildFQName(
				lokiMetricsNamespace,
				lokiMetricsSubsystem,
				"dropped_total",
			),
			"Total number of log records dropped from the local queue.",
			nil,
			nil,
		),
		pushErrorsDesc: prometheus.NewDesc(
			prometheus.BuildFQName(
				lokiMetricsNamespace,
				lokiMetricsSubsystem,
				"push_errors_total",
			),
			"Total number of Loki push attempts that failed after retries.",
			nil,
			nil,
		),
		client: client,
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.sentDesc
	ch <- c.droppedDesc
	ch <- c.pushErrorsDesc
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c == nil || c.client == nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		c.sentDesc,
		prometheus.CounterValue,
		float64(c.client.Sent()),
	)
	ch <- prometheus.MustNewConstMetric(
		c.droppedDesc,
		prometheus.CounterValue,
		float64(c.client.Dropped()),
	)
	ch <- prometheus.MustNewConstMetric(
		c.pushErrorsDesc,
		prometheus.CounterValue,
		float64(c.client.PushErrors()),
	)
}
