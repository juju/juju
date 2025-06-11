// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

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
	instanceStatusLabel   = "instance_status"
	workloadStatusLabel   = "workload_status"
	baseLabel             = "base"
	archLabel             = "arch"
)

var (
	machineLabelNames = []string{
		agentStatusLabel,
		lifeLabel,
		instanceStatusLabel,
		baseLabel,
		archLabel,
	}

	applicationLabelNames = []string{
		lifeLabel,
	}

	unitLabelNames = []string{
		agentStatusLabel,
		lifeLabel,
		workloadStatusLabel,
		baseLabel,
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

	ApplicationConfigReads   prometheus.Gauge
	ApplicationHashCacheHit  prometheus.Gauge
	ApplicationHashCacheMiss prometheus.Gauge

	CharmConfigHashCacheHit  prometheus.Gauge
	CharmConfigHashCacheMiss prometheus.Gauge
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
		CharmConfigHashCacheHit: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "charm_config_watcher_hash_hit",
				Help:      "The number of times a change in master or branch config required no notification to config watcher(s)",
			},
		),
		CharmConfigHashCacheMiss: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "charm_config_watcher_hash_miss",
				Help:      "The number of times a change in master or branch config required notification to config watcher(s)",
			},
		),
		ApplicationConfigReads: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "application_config_reads",
				Help:      "The number of times the application config is read.",
			},
		),
		ApplicationHashCacheHit: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "application_hash_cache_hit",
				Help:      "The number of times the application config change hash was determined using the cached value.",
			},
		),
		ApplicationHashCacheMiss: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "application_hash_cache_miss",
				Help:      "The number of times the application config change hash was generated.",
			},
		),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *ControllerGauges) Describe(ch chan<- *prometheus.Desc) {
	c.ModelConfigReads.Describe(ch)
	c.ModelHashCacheHit.Describe(ch)
	c.ModelHashCacheMiss.Describe(ch)

	c.CharmConfigHashCacheHit.Describe(ch)
	c.CharmConfigHashCacheMiss.Describe(ch)

	c.ApplicationConfigReads.Describe(ch)
	c.ApplicationHashCacheHit.Describe(ch)
	c.ApplicationHashCacheMiss.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *ControllerGauges) Collect(ch chan<- prometheus.Metric) {
	c.ModelConfigReads.Collect(ch)
	c.ModelHashCacheHit.Collect(ch)
	c.ModelHashCacheMiss.Collect(ch)

	c.CharmConfigHashCacheHit.Collect(ch)
	c.CharmConfigHashCacheMiss.Collect(ch)

	c.ApplicationConfigReads.Collect(ch)
	c.ApplicationHashCacheHit.Collect(ch)
	c.ApplicationHashCacheMiss.Collect(ch)
}

// Collector is a prometheus.Collector that collects metrics about
// the Juju global state.
type Collector struct {
	controller *Controller

	scrapeDuration prometheus.Gauge
	scrapeErrors   prometheus.Gauge

	models       *prometheus.GaugeVec
	machines     *prometheus.GaugeVec
	applications *prometheus.GaugeVec
	units        *prometheus.GaugeVec
	users        *prometheus.GaugeVec

	// Since the collector resets the GaugeVecs and iterates the model cache,
	// we need to ensure that we don't have overlapping collect calls.
	mu sync.Mutex
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
		applications: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "applications",
				Help:      "Number of applications managed by the controller.",
			},
			applicationLabelNames,
		),
		units: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: metricsNamespace,
				Name:      "units",
				Help:      "Number of units managed by the controller.",
			},
			unitLabelNames,
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
	c.controller.metrics.Describe(ch)

	c.models.Describe(ch)
	c.machines.Describe(ch)
	c.applications.Describe(ch)
	c.units.Describe(ch)
	c.users.Describe(ch)

	c.scrapeErrors.Describe(ch)
	c.scrapeDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	timer := prometheus.NewTimer(prometheus.ObserverFunc(c.scrapeDuration.Set))
	defer c.scrapeDuration.Collect(ch)
	defer timer.ObserveDuration()
	c.scrapeErrors.Set(0)
	defer c.scrapeErrors.Collect(ch)

	c.models.Reset()
	c.machines.Reset()
	c.applications.Reset()
	c.units.Reset()
	c.users.Reset()

	c.updateMetrics()

	c.controller.metrics.Collect(ch)
	c.models.Collect(ch)
	c.machines.Collect(ch)
	c.applications.Collect(ch)
	c.units.Collect(ch)
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
	logger.Tracef("updating cache metrics for %s", modelUUID)
	model, err := c.controller.Model(modelUUID)
	if err != nil {
		logger.Debugf("error getting model: %v", err)
		return
	}
	model.mu.Lock()
	defer model.mu.Unlock()

	for _, machine := range model.machines {
		arch := "unknown"
		if machine.details.HardwareCharacteristics != nil && machine.details.HardwareCharacteristics.Arch != nil {
			arch = *machine.details.HardwareCharacteristics.Arch
		}
		c.machines.With(prometheus.Labels{
			agentStatusLabel:    string(machine.details.AgentStatus.Status),
			lifeLabel:           string(machine.details.Life),
			instanceStatusLabel: string(machine.details.InstanceStatus.Status),
			baseLabel:           machine.details.Base,
			archLabel:           arch,
		}).Inc()
	}
	for _, app := range model.applications {
		c.applications.With(prometheus.Labels{
			lifeLabel: string(app.details.Life),
		}).Inc()
	}
	for _, unit := range model.units {
		c.units.With(prometheus.Labels{
			agentStatusLabel:    string(unit.details.AgentStatus.Status),
			lifeLabel:           string(unit.details.Life),
			workloadStatusLabel: string(unit.details.WorkloadStatus.Status),
			baseLabel:           unit.details.Base,
		}).Inc()
	}

	c.models.With(prometheus.Labels{
		lifeLabel:   string(model.details.Life),
		statusLabel: string(model.details.Status.Status),
	}).Inc()
}
