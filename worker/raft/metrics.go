// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"time"

	"github.com/armon/go-metrics"
	pmetrics "github.com/armon/go-metrics/prometheus"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
)

// newMetricsCollector returns a collector for the metrics gathered in the
// hashicorp/raft library code.
func newMetricsCollector() (prometheus.Collector, error) {
	sink, err := pmetrics.NewPrometheusSink()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// go-metrics always registers the sink it returns in the default
	// registry, which we don't collect metrics from - unregister it
	// so subsequent calls don't fail because it's already registered
	// there.
	prometheus.DefaultRegisterer.Unregister(sink)
	config := metrics.DefaultConfig("juju")
	config.EnableRuntimeMetrics = false
	_, err = metrics.NewGlobal(config, sink)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return sink, nil
}

func registerRaftMetrics(registry prometheus.Registerer, logger Logger) {
	collector, err := newMetricsCollector()
	if err != nil {
		// It isn't a fatal error to fail to set up metrics, so
		// log and continue
		logger.Warningf("creating a raft metrics collector failed: %v", err)
		return
	}

	if err := registerMetrics(registry, collector); err != nil {
		logger.Warningf("registering raft metrics collector failed: %v", err)
	}
}

func registerApplierMetrics(registry prometheus.Registerer, collector prometheus.Collector, logger Logger) {
	if err := registerMetrics(registry, collector); err != nil {
		logger.Warningf("registering raft applier metrics collector failed: %v", err)
	}
}

func registerMetrics(registry prometheus.Registerer, collector prometheus.Collector) error {
	// We use unregister/register rather than
	// register/defer-unregister to avoid this scenario:
	// * raft.newWorker is called, which starts loop in a
	//   goroutine.
	// * loop registers the collector and defers unregistering.
	// * loop gets delayed starting raft (possibly it's taking a
	//   long time for the peergrouper to publish the api addresses).
	// * newWorker times out waiting for loop to be ready, kills the
	//   catacomb and returns a timeout error - at this point loop
	//   hasn't finished, so the collector hasn't been unregistered.
	// * The dep-engine calls newWorker again, it starts a new loop
	//   goroutine.
	// * The new run of loop can't register the collector, and we
	//   never see raft metrics for this controller.
	registry.Unregister(collector)
	err := registry.Register(collector)
	return errors.Trace(err)
}

const (
	metricsNamespace = "juju_raftapply"
)

type applierMetrics struct {
	applications *prometheus.SummaryVec
	clock        clock.Clock
}

func newApplierMetrics(clock clock.Clock) *applierMetrics {
	return &applierMetrics{
		applications: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: metricsNamespace,
			Name:      "applications",
			Help:      "Application times for applying to the raft log in ms",
			Objectives: map[float64]float64{
				0.5:  0.05,
				0.9:  0.01,
				0.99: 0.001,
			},
		}, []string{
			"result", // success, failure
		}),
		clock: clock,
	}
}

// Record times how long a apply operation took, along with if it failed or
// not. This can be used to understand if we're hitting issues with the
// underlying raft instance.
func (a *applierMetrics) Record(start time.Time, result string) {
	elapsedMS := float64(a.clock.Now().Sub(start)) / float64(time.Millisecond)
	a.applications.With(prometheus.Labels{
		"result": result,
	}).Observe(elapsedMS)
}

// RecordLeaderError calls out that there was a leader error, so didn't
// follow the usual flow.
func (a *applierMetrics) RecordLeaderError(start time.Time) {
	elapsedMS := float64(a.clock.Now().Sub(start)) / float64(time.Millisecond)
	a.applications.With(prometheus.Labels{
		"result": "leader-error",
	}).Observe(elapsedMS)
}

// Describe is part of the prometheus.Collector interface.
func (a *applierMetrics) Describe(ch chan<- *prometheus.Desc) {
	a.applications.Describe(ch)
}

// Collect is part of the prometheus.Collector interface.
func (a *applierMetrics) Collect(ch chan<- prometheus.Metric) {
	a.applications.Collect(ch)
}
