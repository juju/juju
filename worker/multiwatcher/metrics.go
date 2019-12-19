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
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.watcherCount.Describe(ch)
	c.restartCount.Describe(ch)
	c.storeSize.Describe(ch)
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

	c.watcherCount.Collect(ch)
	c.restartCount.Collect(ch)
	c.storeSize.Collect(ch)
}
