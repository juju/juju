// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	changestreamMetricsNamespace   = "juju"
	changestreamSubsystemNamespace = "db"
)

var (
	labelNames = []string{"namespace"}
)

// Metrics is a wrapper for the Collector interface of the prometheus package,
// extended with a ForNamespace(string) method that returns a collector of
// changestream metrics for a given namespace.
type Metrics interface {
	ForNamespace(string) *NamespaceCollector
	Describe(ch chan<- *prometheus.Desc)
	Collect(ch chan<- prometheus.Metric)
}

// NamespaceMetrics is a set of methods to be used in changestream to collect
// prometheus metrics for a given namespace.
type NamespaceMetrics interface {
	// Stream metrics.
	WatermarkInsertsInc()
	WatermarkRetriesInc()
	ChangesRequestDurationObserve(val float64)
	ChangesCountObserve(val int)
	// EventMultiplexer metrics.
	SubscriptionsInc()
	SubscriptionsDec()
	DispatchDurationObserve(val float64, failed bool)
}

// NamespaceCollector is a prometheus collector extended with a Namespace
// argument used in the metric labels.
type NamespaceCollector struct {
	*Collector
	Namespace string
}

// WatermarkInsertsInc increments the watermark inserts counter.
func (c *NamespaceCollector) WatermarkInsertsInc() {
	c.WatermarkInserts.WithLabelValues(c.Namespace).Inc()
}

// WatermarkRetriesInc increments the watermark insertion retries counter.
func (c *NamespaceCollector) WatermarkRetriesInc() {
	c.WatermarkRetries.WithLabelValues(c.Namespace).Inc()
}

// ChangesRequestDurationObserve records a duration of the changes request (see
// worker/changestream/stream/stream.go readChanges()).
func (c *NamespaceCollector) ChangesRequestDurationObserve(val float64) {
	c.ChangesRequestDuration.WithLabelValues(c.Namespace).Observe(val)
}

// ChangesCountObserve records the number of changes returned by the changes
// request (see worker/changestream/stream/stream.go readChanges()).
func (c *NamespaceCollector) ChangesCountObserve(val int) {
	c.ChangesCount.WithLabelValues(c.Namespace).Observe(float64(val))
}

// SubscriptionsInc increments the number of current subscriptions.
func (c *NamespaceCollector) SubscriptionsInc() {
	c.Subscriptions.WithLabelValues(c.Namespace).Inc()
}

// SubscriptionsDec decrements the number of current subscriptions.
func (c *NamespaceCollector) SubscriptionsDec() {
	c.Subscriptions.WithLabelValues(c.Namespace).Dec()
}

// DispatchDurationObsere records the duration of the events dispatch method
// (see worker/changestream/eventmultiplexer/eventmultiplexer.go dispatchSet()).
func (c *NamespaceCollector) DispatchDurationObserve(val float64, failed bool) {
	c.DispatchDuration.WithLabelValues(c.Namespace, strconv.FormatBool(failed)).Observe(val)
}

// Collector defines a prometheus collector for the dbaccessor.
type Collector struct {
	// Stream metrics.
	WatermarkInserts       *prometheus.CounterVec
	WatermarkRetries       *prometheus.CounterVec
	ChangesRequestDuration *prometheus.HistogramVec
	ChangesCount           *prometheus.HistogramVec
	// EventMultiplexer metrics.
	Subscriptions    *prometheus.GaugeVec
	DispatchDuration *prometheus.HistogramVec
}

func (c *Collector) ForNamespace(namespace string) *NamespaceCollector {
	return &NamespaceCollector{
		Collector: c,
		Namespace: namespace,
	}
}

// Describe is part of the prometheus.Collector interface.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	// Stream metrics.
	c.WatermarkInserts.Describe(ch)
	c.WatermarkRetries.Describe(ch)
	c.ChangesRequestDuration.Describe(ch)
	c.ChangesCount.Describe(ch)
	// EventMultiplexer metrics.
	c.Subscriptions.Describe(ch)
	c.DispatchDuration.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// Stream metrics.
	c.WatermarkInserts.Collect(ch)
	c.WatermarkRetries.Collect(ch)
	c.ChangesRequestDuration.Collect(ch)
	c.ChangesCount.Collect(ch)
	// EventMultiplexer metrics.
	c.Subscriptions.Collect(ch)
	c.DispatchDuration.Collect(ch)
}

// NewMetricsCollector returns a new Collector.
func NewMetricsCollector() *Collector {
	return &Collector{
		// Stream metrics.
		WatermarkInserts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "watermark_inserts_total",
			Help:      "Total number of watermark insertions on the changelog witness table.",
		}, labelNames),
		WatermarkRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "watermark_retries_total",
			Help:      "Total number of watermark retries on the changelog witness table.",
		}, labelNames),
		ChangesRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "changestream_duration_seconds",
			Help:      "Total time spent in changestream requests.",
		}, labelNames),
		ChangesCount: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "changestream_count",
			Help:      "Total number of changes returned by the changestream requests.",
		}, labelNames),
		// EventMultiplexer metrics.
		Subscriptions: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "subscription_count",
			Help:      "The total number of subscriptions, labeled per model.",
		}, labelNames),
		DispatchDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: changestreamMetricsNamespace,
			Subsystem: changestreamSubsystemNamespace,
			Name:      "dispatch_duration_seconds",
			Help:      "Total time spent dispatching event multiplexer events.",
		}, []string{"namespace", "failed"}),
	}
}
