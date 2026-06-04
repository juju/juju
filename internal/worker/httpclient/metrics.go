// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju"
	metricsSubsystem = "http_client_worker"

	// MetricLabelEndpoint defines a constant for the APIConnections and
	// PingFailureCount Labels
	MetricLabelEndpoint = "endpoint"

	// MetricLabelHost defines a host constant for the Requests Label
	MetricLabelHost = "host"

	// MetricLabelStatus defines a status constant for the Requests Label
	MetricLabelStatus = "status"
)

// MetricTotalRequestsWithStatusLabelNames defines a series of labels for the
// TotalRequests metric.
var MetricTotalRequestsWithStatusLabelNames = []string{
	MetricLabelHost,
	MetricLabelStatus,
}

// MetricTotalRequestsLabelNames defines a series of labels for the
// TotalRequests metric.
var MetricTotalRequestsLabelNames = []string{
	MetricLabelHost,
}

// MetricTotalRequestsErrorLabelNames defines a series of labels for the
// TotalRequestErrors metric.
var MetricTotalRequestsErrorLabelNames = []string{
	MetricLabelHost,
	MetricLabelStatus,
}

// Collector defines a prometheus collector for the http client worker.
type Collector struct {
	TotalRequests         *prometheus.CounterVec
	TotalRequestErrors    *prometheus.CounterVec
	TotalRequestsDuration *prometheus.SummaryVec
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		TotalRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "outbound_requests_total",
			Help:      "Total number of http requests to outbound APIs",
		}, MetricTotalRequestsWithStatusLabelNames),
		TotalRequestErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "outbound_request_errors_total",
			Help:      "Total number of http request errors to outbound APIs",
		}, MetricTotalRequestsErrorLabelNames),
		TotalRequestsDuration: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "outbound_request_duration_seconds",
			Help:      "Latency of outbound API requests in seconds.",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, MetricTotalRequestsLabelNames),
	}
}

// Describe implements the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	c.TotalRequests.Describe(ch)
	c.TotalRequestErrors.Describe(ch)
	c.TotalRequestsDuration.Describe(ch)
}

// Collect implements the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.TotalRequests.Collect(ch)
	c.TotalRequestErrors.Collect(ch)
	c.TotalRequestsDuration.Collect(ch)
}

// Record an outgoing request which produced an http.Response.
func (c *Collector) Record(method string, url *url.URL, res *http.Response, rtt time.Duration) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	statusCode := strconv.FormatInt(int64(res.StatusCode), 10)
	c.TotalRequests.WithLabelValues(url.Host, statusCode).Inc()
	if res.StatusCode >= 400 {
		c.TotalRequestErrors.WithLabelValues(url.Host, statusCode).Inc()
	}
	c.TotalRequestsDuration.WithLabelValues(url.Host).Observe(rtt.Seconds())
}

// Record an outgoing request which returned back an error.
func (c *Collector) RecordError(method string, url *url.URL, err error) {
	// Note: Do not log url.Path as REST queries _can_ include the name of the
	// entities (charms, architectures, etc).
	c.TotalRequests.WithLabelValues(url.Host, "unknown").Inc()
	c.TotalRequestErrors.WithLabelValues(url.Host).Inc()
}
