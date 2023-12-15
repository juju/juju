// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsendermetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/internal/worker/logsender"
)

var (
	jujuLogsenderCapacityDesc = prometheus.NewDesc(
		"juju_logsender_capacity",
		"Total number of log messages that can be enqueued before being dropped.",
		[]string{},
		prometheus.Labels{},
	)
	jujuLogsenderEnqueuedTotalDesc = prometheus.NewDesc(
		"juju_logsender_enqueued_total",
		"Total number of log messages enqueued.",
		[]string{},
		prometheus.Labels{},
	)
	jujuLogsenderSentTotalDesc = prometheus.NewDesc(
		"juju_logsender_sent_total",
		"Total number of log messages sent.",
		[]string{},
		prometheus.Labels{},
	)
	jujuLogsenderDroppedTotalDesc = prometheus.NewDesc(
		"juju_logsender_dropped_total",
		"Total number of log messages dropped.",
		[]string{},
		prometheus.Labels{},
	)
)

// BufferedLogWriterMetrics is a prometheus.Collector that collects metrics
// from a BufferedLogWriter.
type BufferedLogWriterMetrics struct {
	*logsender.BufferedLogWriter
}

// Describe is part of the prometheus.Collector interface.
func (BufferedLogWriterMetrics) Describe(ch chan<- *prometheus.Desc) {
	ch <- jujuLogsenderCapacityDesc
	ch <- jujuLogsenderEnqueuedTotalDesc
	ch <- jujuLogsenderSentTotalDesc
	ch <- jujuLogsenderDroppedTotalDesc
}

// Collect is part of the prometheus.Collector interface.
func (b BufferedLogWriterMetrics) Collect(ch chan<- prometheus.Metric) {
	stats := b.Stats()
	ch <- prometheus.MustNewConstMetric(
		jujuLogsenderCapacityDesc,
		prometheus.CounterValue,
		float64(b.Capacity()),
	)
	ch <- prometheus.MustNewConstMetric(
		jujuLogsenderEnqueuedTotalDesc,
		prometheus.CounterValue,
		float64(stats.Enqueued),
	)
	ch <- prometheus.MustNewConstMetric(
		jujuLogsenderSentTotalDesc,
		prometheus.CounterValue,
		float64(stats.Sent),
	)
	ch <- prometheus.MustNewConstMetric(
		jujuLogsenderDroppedTotalDesc,
		prometheus.CounterValue,
		float64(stats.Dropped),
	)
}
