// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package centralhub

import (
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "juju"
	metricsSubsytem  = "pubsub"
)

// GaugeVec defines in place gauge used for testing.
type GaugeVec interface {
	prometheus.Collector
	With(prometheus.Labels) prometheus.Gauge
}

// PubsubMetrics implements pubsub.Metrics for gaining information about the
// current work hub and it's subscribers are undertaking.
type PubsubMetrics struct {
	subscriptions prometheus.Gauge
	published     GaugeVec
	queue         GaugeVec
	consumed      *prometheus.SummaryVec
}

// NewPubsubMetrics creates a new set of pubsub metrics for collecting
// information about the ongoing work for a given central hub.
func NewPubsubMetrics() *PubsubMetrics {
	return &PubsubMetrics{
		subscriptions: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsytem,
			Name:      "subscriptions",
			Help:      "Number of subscriptions on a hub",
		}),
		published: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsytem,
			Name:      "published",
			Help:      "Number of published message per topic",
		}, []string{
			"topic",
		}),
		queue: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsytem,
			Name:      "queue",
			Help:      "Queue length for a given callback identifier",
		}, []string{
			"ident",
		}),
		consumed: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsytem,
			Name:      "consumed",
			Help:      "Consumed times for a pubsub message",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, []string{
			"ident",
		}),
	}
}

func (m *PubsubMetrics) Subscribed() {
	m.subscriptions.Inc()
}

func (m *PubsubMetrics) Unsubscribed() {
	m.subscriptions.Dec()
}

var (
	leaseRequestRegex  = regexp.MustCompile("lease.request.[0-9a-f]+.[0-9]+")
	callbackTopicRegex = regexp.MustCompile("lease.request.callback.[a-z0-9]{8}-([a-z0-9]{4}-){3}[a-z0-9]{12}")
)

const (
	// Store the leaseCallbackNamespace as a constant.
	leaseCallbackNamespace = "lease.request.callback"
)

func (m *PubsubMetrics) Published(topic string) {
	// Pubsub synchronous callback hack needs to be worked around, otherwise
	// we explode the cardinality of the metrics.
	if leaseRequestRegex.MatchString(topic) {
		if index := strings.LastIndex(topic, "."); index > 0 {
			topic = topic[:index]
		}
	} else if callbackTopicRegex.MatchString(topic) {
		topic = leaseCallbackNamespace
	}

	m.published.With(prometheus.Labels{
		"topic": topic,
	}).Inc()
}

func (m *PubsubMetrics) Enqueued(ident string) {
	m.queue.With(prometheus.Labels{
		"ident": ident,
	}).Inc()
}

func (m *PubsubMetrics) Dequeued(ident string) {
	m.queue.With(prometheus.Labels{
		"ident": ident,
	}).Dec()
}

func (m *PubsubMetrics) Consumed(ident string, duration time.Duration) {
	elapsedMS := float64(duration) / float64(time.Millisecond)
	m.consumed.With(prometheus.Labels{
		"ident": ident,
	}).Observe(elapsedMS)
}

// Describe is part of prometheus.Collector.
func (m *PubsubMetrics) Describe(ch chan<- *prometheus.Desc) {
	m.subscriptions.Describe(ch)
	m.published.Describe(ch)
	m.queue.Describe(ch)
	m.consumed.Describe(ch)
}

// Collect is part of prometheus.Collector.
func (m *PubsubMetrics) Collect(ch chan<- prometheus.Metric) {
	m.subscriptions.Collect(ch)
	m.published.Collect(ch)
	m.queue.Collect(ch)
	m.consumed.Collect(ch)
}

type PubsubNoOpMetrics struct{}

func (PubsubNoOpMetrics) Subscribed()                                   {}
func (PubsubNoOpMetrics) Unsubscribed()                                 {}
func (PubsubNoOpMetrics) Published(topic string)                        {}
func (PubsubNoOpMetrics) Enqueued(ident string)                         {}
func (PubsubNoOpMetrics) Dequeued(ident string)                         {}
func (PubsubNoOpMetrics) Consumed(ident string, duration time.Duration) {}
