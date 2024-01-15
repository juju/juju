// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	dependencyMetricsNamespace   = "juju"
	dependencySubsystemNamespace = "dependency_engine"
)

const (
	// metricsLabelWorkerName defines a constant for the MetricsWorkerStartNames
	// Labels.
	metricsLabelWorkerName = "worker"

	// metricsLabelModelUUID defines a constant for the MetricsWorkerStartNames
	// Labels.
	metricsLabelModelUUID = "model_uuid"
)

var metricsWorkerStartNames = []string{
	metricsLabelWorkerName,
}

var metricsModelWorkerStartNames = []string{
	metricsLabelModelUUID,
}

// Collector defines a new metrics collector. This allows the collection of
// models for one model.
type Collector struct {
	workerStarts      *prometheus.GaugeVec
	modelWorkerStarts *prometheus.GaugeVec
}

// NewMetrics creates a new collector for a model.
func NewMetrics() *Collector {
	return &Collector{
		workerStarts: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: dependencyMetricsNamespace,
			Subsystem: dependencySubsystemNamespace,
			Name:      "worker_start",
			Help:      "Current number of worker starts in the dependency engine by worker",
		}, metricsWorkerStartNames),
		modelWorkerStarts: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: dependencyMetricsNamespace,
			Subsystem: dependencySubsystemNamespace,
			Name:      "worker_starts_for_model",
			Help:      "Current number of worker starts in the dependency engine by model",
		}, metricsModelWorkerStartNames),
	}
}

// MetricSink describes a way to unregister a model metrics collector. This
// ensures that we correctly tidy up after the removal of a model.
type MetricSink interface {
	dependency.Metrics
	Unregister() bool
}

// ForModel returns a metrics collector for a given model.
func (c *Collector) ForModel(model names.ModelTag) MetricSink {
	return &modelCollector{
		collector: c,
		modelUUID: model.Id(),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.workerStarts.Describe(ch)
	c.modelWorkerStarts.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.workerStarts.Collect(ch)
	c.modelWorkerStarts.Collect(ch)
}

type modelCollector struct {
	modelUUID string
	collector *Collector
}

// RecordStart records the number of starts a given worker has started.
// This is over the lifetime of a model, even after restarts.
func (c *modelCollector) RecordStart(worker string) {
	c.collector.workerStarts.WithLabelValues(worker).Inc()
	c.collector.modelWorkerStarts.WithLabelValues(c.modelUUID).Inc()
}

// Unregister removes any associated model worker starts from the sink if
// the model is removed.
func (c *modelCollector) Unregister() bool {
	return c.collector.modelWorkerStarts.DeleteLabelValues(c.modelUUID)
}
