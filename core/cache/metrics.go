// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju_cache"

	statusLabel           = "status"
	lifeLabel             = "life"
	disabledLabel         = "disabled"
	deletedLabel          = "deleted"
	controllerAccessLabel = "controller_access"
	domainLabel           = "domain"
	agentStatusLabel      = "agent_status"
	machineStatusLabel    = "machine_status"
)

var (
	machineLabelNames = []string{
		agentStatusLabel,
		lifeLabel,
		machineStatusLabel,
	}

	modelLabelNames = []string{
		lifeLabel,
		statusLabel,
	}

	userLabelNames = []string{
		controllerAccessLabel,
		deletedLabel,
		disabledLabel,
		domainLabel,
	}

	logger = loggo.GetLogger("juju.core.cache")
)

// ControllerGauges holds the prometheus gauges for ever increasing
// values used by the controller.
type ControllerGauges struct {
	ModelConfigReads   prometheus.Gauge
	ModelHashCacheHit  prometheus.Gauge
	ModelHashCacheMiss prometheus.Gauge
}

func createControllerGauges() *ControllerGauges {
	return &ControllerGauges{
		ModelConfigReads: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "model_config_reads",
				Help:      "The number of times the model config is read.",
			},
		),
		ModelHashCacheHit: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "model_hash_cache_hit",
				Help:      "The number of times the model config change hash was determined using the cached value.",
			},
		),
		ModelHashCacheMiss: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "model_hash_cache_miss",
				Help:      "The number of times the model config change hash was generated.",
			},
		),
	}
}

// Collect is part of the prometheus.Collector interface.
func (c *ControllerGauges) Collect(ch chan<- prometheus.Metric) {
	c.ModelConfigReads.Collect(ch)
	c.ModelHashCacheHit.Collect(ch)
	c.ModelHashCacheMiss.Collect(ch)
}

// Collector is a prometheus.Collector that collects metrics about
// the Juju global state.
type Collector struct {
	controller *Controller

	scrapeDuration prometheus.Gauge
	scrapeErrors   prometheus.Gauge

	models   *prometheus.GaugeVec
	machines *prometheus.GaugeVec
	users    *prometheus.GaugeVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector(controller *Controller) *Collector {
	return &Collector{
		controller: controller,
		scrapeDuration: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "scrape_duration_seconds",
				Help:      "Amount of time taken to collect state metrics.",
			},
		),
		scrapeErrors: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "scrape_errors",
				Help:      "Number of errors observed while collecting state metrics.",
			},
		),

		models: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "models",
				Help:      "Number of models in the controller.",
			},
			modelLabelNames,
		),
		machines: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "machines",
				Help:      "Number of machines managed by the controller.",
			},
			machineLabelNames,
		),
		users: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "users",
				Help:      "Number of local users in the controller.",
			},
			userLabelNames,
		),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.machines.Describe(ch)
	c.models.Describe(ch)
	c.users.Describe(ch)

	c.scrapeErrors.Describe(ch)
	c.scrapeDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.scrapeDuration.Set))
	defer c.scrapeDuration.Collect(ch)
	defer timer.ObserveDuration()
	c.scrapeErrors.Set(0)
	defer c.scrapeErrors.Collect(ch)

	c.machines.Reset()
	c.models.Reset()
	c.users.Reset()

	c.updateMetrics()

	c.controller.metrics.Collect(ch)
	c.machines.Collect(ch)
	c.models.Collect(ch)
	c.users.Collect(ch)
}

func (c *Collector) updateMetrics() {
	logger.Tracef("updating cache metrics")
	defer logger.Tracef("updated cache metrics")

	modelUUIDs := c.controller.ModelUUIDs()
	for _, m := range modelUUIDs {
		c.updateModelMetrics(m)
	}

	// TODO: add user metrics.
}

func (c *Collector) updateModelMetrics(modelUUID string) {
	model, err := c.controller.Model(modelUUID)
	if err != nil {
		logger.Debugf("error getting model: %v", err)
		return
	}

	// TODO: add machines, applications and units.

	c.models.With(prometheus.Labels{
		lifeLabel:   string(model.details.Life),
		statusLabel: string(model.details.Status.Status),
	}).Inc()
}
