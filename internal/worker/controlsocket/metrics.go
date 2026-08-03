// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	controlSocketMetricsNamespace   = "juju"
	controlSocketSubsystemNamespace = "control_socket"
)

// Collector defines a prometheus collector for the controlsocket worker.
type Collector struct {
	Requests        *prometheus.CounterVec
	RequestErrors   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		Requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: controlSocketMetricsNamespace,
			Subsystem: controlSocketSubsystemNamespace,
			Name:      "requests_total",
			Help:      "Total number of control socket requests.",
		}, []string{"endpoint", "method", "status"}),
		RequestErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: controlSocketMetricsNamespace,
			Subsystem: controlSocketSubsystemNamespace,
			Name:      "request_errors_total",
			Help:      "Total number of failed control socket requests.",
		}, []string{"endpoint", "method", "status"}),
		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: controlSocketMetricsNamespace,
			Subsystem: controlSocketSubsystemNamespace,
			Name:      "request_duration_seconds",
			Help:      "Control socket request duration in seconds.",
		}, []string{"endpoint", "method"}),
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.Requests.Describe(ch)
	c.RequestErrors.Describe(ch)
	c.RequestDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.Requests.Collect(ch)
	c.RequestErrors.Collect(ch)
	c.RequestDuration.Collect(ch)
}

func (c *Collector) recordRequest(endpoint, method string, status int, durationSeconds float64) {
	if c == nil {
		return
	}
	statusString := strconv.Itoa(status)
	c.Requests.WithLabelValues(endpoint, method, statusString).Inc()
	if status >= 400 {
		c.RequestErrors.WithLabelValues(endpoint, method, statusString).Inc()
	}
	c.RequestDuration.WithLabelValues(endpoint, method).Observe(durationSeconds)
}
