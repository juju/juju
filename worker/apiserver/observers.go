// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/controller"
)

func newObserverFn(
	agentConfig agent.Config,
	controllerConfig controller.Config,
	clock clock.Clock,
	prometheusRegisterer prometheus.Registerer,
	hub *pubsub.StructuredHub,
) (observer.ObserverFactory, error) {

	var observerFactories []observer.ObserverFactory

	// Common logging of RPC requests
	observerFactories = append(observerFactories, func() observer.Observer {
		logger := loggo.GetLogger("juju.apiserver")
		ctx := observer.RequestObserverContext{
			Clock:  clock,
			Logger: logger,
			Hub:    hub,
		}
		return observer.NewRequestObserver(ctx)
	})

	// Metrics observer.
	metricObserver, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:                clock,
		PrometheusRegisterer: prometheusRegisterer,
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating metric observer factory")
	}
	observerFactories = append(observerFactories, metricObserver)

	return observer.ObserverFactoryMultiplexer(observerFactories...), nil
}
