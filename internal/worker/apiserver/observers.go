// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/metricobserver"
	"github.com/juju/juju/core/model"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/services"
)

func newObserverFn(
	agentConfig agent.Config,
	domainServicesGetter services.DomainServicesGetter,
	clock clock.Clock,
	metricsCollector *apiserver.Collector,
) (observer.ObserverFactory, error) {
	// Metrics observer.
	metricObserver, err := metricobserver.NewObserverFactory(metricobserver.Config{
		Clock:            clock,
		MetricsCollector: metricCollector{collector: metricsCollector},
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating metric observer factory")
	}

	return observer.ObserverFactoryMultiplexer(
		requestLogger(clock),
		metricObserver,
		agentPresence(domainServicesGetter),
	), nil
}

// requestLogger is common logging of RPC requests and responses.
func requestLogger(clock clock.Clock) func() observer.Observer {
	return func() observer.Observer {
		return observer.NewRequestLogger(observer.RequestLoggerConfig{
			Clock:  clock,
			Logger: internallogger.GetLogger("juju.apiserver"),
		})
	}
}

func agentPresence(servicesGetter services.DomainServicesGetter) func() observer.Observer {
	return func() observer.Observer {
		return observer.NewAgentPresence(observer.AgentPresenceConfig{
			DomainServicesGetter: domainServicesGetter{
				servicesGetter: servicesGetter,
			},
			Logger: internallogger.GetLogger("juju.apiserver"),
		})
	}
}

type metricCollector struct {
	collector *apiserver.Collector
}

func (o metricCollector) APIRequestDuration() metricobserver.SummaryVec {
	return o.collector.APIRequestDuration
}

type domainServicesGetter struct {
	servicesGetter services.DomainServicesGetter
}

func (d domainServicesGetter) ServicesForModel(ctx context.Context, uuid model.UUID) (observer.ModelService, error) {
	services, err := d.servicesGetter.ServicesForModel(ctx, uuid)
	if err != nil {
		return nil, err
	}
	return modelService{services: services}, nil
}

type modelService struct {
	services services.DomainServices
}

func (m modelService) StatusService() observer.StatusService {
	return m.services.Status()
}
