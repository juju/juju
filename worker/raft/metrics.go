// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raft

import (
	"github.com/armon/go-metrics"
	pmetrics "github.com/armon/go-metrics/prometheus"
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

func registerMetrics(registry prometheus.Registerer, logger Logger) {
	collector, err := newMetricsCollector()
	if err != nil {
		// It isn't a fatal error to fail to set up metrics, so
		// log and continue
		logger.Warningf("creating a raft metrics collector failed: %v", err)
		return
	}

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
	err = registry.Register(collector)
	if err != nil {
		logger.Warningf("registering metrics collector failed: %v", err)
	}

}
