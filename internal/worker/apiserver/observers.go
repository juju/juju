// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	internallogger "github.com/juju/juju/internal/logger"
)

func newObserverFn(
	agentConfig agent.Config,
	clock clock.Clock,
	hub *pubsub.StructuredHub,
	metricsCollector *apiserver.Collector,
) (observer.ObserverFactory, error) {
	// Metrics observer.
	metricObserver, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            clock,
		MetricsCollector: metricCollectorWrapper{collector: metricsCollector},
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating metric observer factory")
	}

	return observer.ObserverFactoryMultiplexer([]observer.ObserverFactory{
		func() observer.Observer {
			logger := internallogger.GetLogger("juju.apiserver")
			ctx := observer.RequestObserverConfig{
				Clock:  clock,
				Logger: logger,
			}
			return observer.NewRequestObserver(ctx)
		},
		metricObserver,
	}...), nil
}

type metricCollectorWrapper struct {
	collector *apiserver.Collector
}

func (o metricCollectorWrapper) APIRequestDuration() metricobserver.SummaryVec {
	return o.collector.APIRequestDuration
}
