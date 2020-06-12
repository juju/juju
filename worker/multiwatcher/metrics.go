// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "juju_multiwatcher"

// Collector is a prometheus.Collector that collects metrics about
// multiwatcher worker.
type Collector struct {
	worker *Worker

	watcherCount prometheus.Gauge
	restartCount prometheus.Gauge
	storeSize    prometheus.Gauge
	queueSize    prometheus.Gauge
	queueAge     prometheus.Gauge
	append       prometheus.Summary
	dupe         prometheus.Counter
	process      prometheus.Summary
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector(worker *Worker) *Collector {
	return &Collector{
		worker: worker,
		watcherCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "watcher_count",
				Help:      "The number of multiwatcher type watchers there are.",
			},
		),
		restartCount: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "restart_count",
				Help:      "The number of times the all watcher has been restarted.",
			},
		),
		storeSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "store_size",
				Help:      "The number of entities in the store.",
			},
		),
		queueSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "queue_size",
				Help:      "The number of entries in the queue to process.",
			},
		),
		queueAge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "queue_age",
				Help:      "The age in seconds of the oldest queue entry.",
			},
		),
		append: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Name:      "append",
			Help:      "Time to append a queue entry in ms.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}),
		dupe: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "dupe",
			Help:      "Count of duplicate watcher notifications already in queue.",
		}),
		process: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Name:      "process",
			Help:      "Time to process a queue entry in ms.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.watcherCount.Describe(ch)
	c.restartCount.Describe(ch)
	c.storeSize.Describe(ch)
	c.queueSize.Describe(ch)
	c.queueAge.Describe(ch)
	c.append.Describe(ch)
	c.dupe.Describe(ch)
	c.process.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// The report deals with the synchronization requirements.
	report := c.worker.Report()

	floatValue := func(key string) float64 {
		return float64(report[key].(int))
	}

	c.watcherCount.Set(floatValue(reportWatcherKey))
	c.restartCount.Set(floatValue(reportRestartKey))
	c.storeSize.Set(floatValue(reportStoreKey))
	c.queueSize.Set(floatValue(reportQueueSizeKey))
	c.queueAge.Set(report[reportQueueAgeKey].(float64))

	c.watcherCount.Collect(ch)
	c.restartCount.Collect(ch)
	c.storeSize.Collect(ch)
	c.queueSize.Collect(ch)
	c.queueAge.Collect(ch)
	c.append.Collect(ch)
	c.dupe.Collect(ch)
	c.process.Collect(ch)
}
