// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statemetrics

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju_state"

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

	logger = loggo.GetLogger("juju.state.statemetrics")
)

// Collector is a prometheus.Collector that collects metrics about
// the Juju global state.
type Collector struct {
	pool StatePool

	scrapeDuration prometheus.Gauge
	scrapeErrors   prometheus.Gauge

	models   *prometheus.GaugeVec
	machines *prometheus.GaugeVec
	users    *prometheus.GaugeVec
}

// New returns a new Collector.
func New(pool StatePool) *Collector {
	return &Collector{
		pool: pool,
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

	c.machines.Collect(ch)
	c.models.Collect(ch)
	c.users.Collect(ch)
}

func (c *Collector) updateMetrics() {
	logger.Tracef("updating state metrics")
	defer logger.Tracef("updated state metrics")

	st := c.pool.SystemState()
	modelUUIDs, err := st.AllModelUUIDs()
	if err != nil {
		logger.Debugf("error getting models: %v", err)
		c.scrapeErrors.Inc()
	}
	for _, m := range modelUUIDs {
		c.updateModelMetrics(m)
	}

	// TODO(axw) AllUsers only returns *local* users. We do not have User
	// records for external users. To obtain external users, we will need
	// to get all of the controller and model-level access documents.
	controllerTag := st.ControllerTag()
	localUsers, err := st.AllUsers()
	if err != nil {
		logger.Debugf("error getting local users: %v", err)
		c.scrapeErrors.Inc()
		localUsers = nil
	}
	for _, u := range localUsers {
		userTag := u.UserTag()
		access, err := st.UserAccess(userTag, controllerTag)
		if err != nil && !errors.IsNotFound(err) {
			logger.Debugf("error getting controller user access: %v", err)
			c.scrapeErrors.Inc()
			continue
		}
		var deleted, disabled string
		if u.IsDeleted() {
			deleted = "true"
		}
		if u.IsDisabled() {
			disabled = "true"
		}
		c.users.With(prometheus.Labels{
			controllerAccessLabel: string(access.Access),
			deletedLabel:          deleted,
			disabledLabel:         disabled,
			domainLabel:           userTag.Domain(),
		}).Inc()
	}
}

func (c *Collector) updateModelMetrics(modelUUID string) {
	model, release, err := c.pool.GetModel(modelUUID)
	if err != nil {
		logger.Debugf("error getting model: %v", err)
		return
	}
	defer release()

	modelStatus, err := model.Status()
	if err != nil {
		if errors.IsNotFound(err) {
			return // Model removed
		}
		c.scrapeErrors.Inc()
		logger.Debugf("error getting model status: %v", err)
		return
	}

	modelTag := model.ModelTag()
	st, releaseState, err := c.pool.Get(modelTag.Id())
	if err != nil {
		if errors.IsNotFound(err) {
			return // Model removed
		}
		c.scrapeErrors.Inc()
		logger.Debugf("error getting model state: %v", err)
		return
	}
	defer releaseState()

	machines, err := st.AllMachines()
	if err != nil {
		c.scrapeErrors.Inc()
		logger.Debugf("error getting machines: %v", err)
		machines = nil
	}
	for _, m := range machines {
		agentStatus, err := m.Status()
		if errors.IsNotFound(err) {
			continue // Machine removed
		} else if err != nil {
			c.scrapeErrors.Inc()
			logger.Debugf("error getting machine status: %v", err)
			continue
		}

		machineStatus, err := m.InstanceStatus()
		if errors.IsNotFound(err) {
			continue // Machine removed
		} else if errors.IsNotProvisioned(err) {
			machineStatus.Status = ""
		} else if err != nil {
			c.scrapeErrors.Inc()
			logger.Debugf("error getting machine status: %v", err)
			continue
		}

		c.machines.With(prometheus.Labels{
			agentStatusLabel:   string(agentStatus.Status),
			lifeLabel:          m.Life().String(),
			machineStatusLabel: string(machineStatus.Status),
		}).Inc()
	}

	c.models.With(prometheus.Labels{
		lifeLabel:   model.Life().String(),
		statusLabel: string(modelStatus.Status),
	}).Inc()
}
