// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import "github.com/prometheus/client_golang/prometheus"

const (
	dbaccessorMetricsNamespace   = "juju"
	dbaccessorSubsystemNamespace = "db"
)

// Collector defines a prometheus collector for the dbaccessor.
type Collector struct {
	DBRequests  *prometheus.GaugeVec
	DBDuration  *prometheus.HistogramVec
	TxnRequests *prometheus.CounterVec
	TxnRetries  *prometheus.CounterVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		DBRequests: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: dbaccessorMetricsNamespace,
			Subsystem: dbaccessorSubsystemNamespace,
			Name:      "requests_total",
			Help:      "Number of active db requests.",
		}, []string{"namespace"}),
		DBDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: dbaccessorMetricsNamespace,
			Subsystem: dbaccessorSubsystemNamespace,
			Name:      "duration_seconds",
			Help:      "Total time spent in db requests.",
		}, []string{"namespace", "result"}),
		TxnRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: dbaccessorMetricsNamespace,
			Subsystem: dbaccessorSubsystemNamespace,
			Name:      "txn_requests_total",
			Help:      "Total number of txn requests including retries.",
		}, []string{"namespace"}),
		TxnRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: dbaccessorMetricsNamespace,
			Subsystem: dbaccessorSubsystemNamespace,
			Name:      "txn_retries_total",
			Help:      "Total number of txn retries.",
		}, []string{"namespace"}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.DBRequests.Describe(ch)
	c.DBDuration.Describe(ch)
	c.TxnRequests.Describe(ch)
	c.TxnRetries.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.DBRequests.Collect(ch)
	c.DBDuration.Collect(ch)
	c.TxnRequests.Collect(ch)
	c.TxnRetries.Collect(ch)
}
