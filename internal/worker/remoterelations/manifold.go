// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apiremoterelationcaller"
)

// RemoteRelationClientGetter defines the interface for a remote relation facade.
type RemoteRelationClientGetter interface {
	// GetRemoteRelationClient returns a RemoteModelRelationsClient for the
	// given model UUID.
	GetRemoteRelationClient(ctx context.Context, modelUUID string) (RemoteModelRelationsClient, error)
}

// NewRemoteRelationClientGetterFunc defines the function signature for creating
// a new RemoteRelationClient.
type NewRemoteRelationClientGetterFunc func(apiremoterelationcaller.APIRemoteCallerGetter) RemoteRelationClientGetter

// NewWorkerFunc defines the function signature for creating a new Worker.
type NewWorkerFunc func(Config) (worker.Worker, error)

// NewRemoteApplicationWorkerFunc defines the function signature for creating
// a new remote application worker.
type NewRemoteApplicationWorkerFunc func(RemoteApplicationConfig) (ReportableWorker, error)

// GetCrossModelServicesFunc defines the function signature for getting
// cross-model services.
type GetCrossModelServicesFunc func(getter dependency.Getter, domainServicesName string) (CrossModelRelationService, error)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName                   string
	APICallerName               string
	APIRemoteRelationCallerName string
	DomainServicesName          string

	NewLocalRemoteRelationFacade  func(base.APICaller) RemoteRelationsFacade
	NewRemoteRelationClientGetter NewRemoteRelationClientGetterFunc

	GetCrossModelServices GetCrossModelServicesFunc

	NewWorker                  NewWorkerFunc
	NewRemoteApplicationWorker NewRemoteApplicationWorkerFunc

	Logger logger.Logger
	Clock  clock.Clock

	// Active indicates if the worker should be started. This is only here so
	// that we can work on implementing cross-model relations behind a flag,
	// which prevents the dependency engine from starting the worker because
	// of other errors.
	Active bool
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.APIRemoteRelationCallerName == "" {
		return errors.NotValidf("empty APIRemoteRelationCallerName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.NewRemoteRelationClientGetter == nil {
		return errors.NotValidf("nil NewRemoteRelationClientGetter")
	}
	if config.GetCrossModelServices == nil {
		return errors.NotValidf("nil GetCrossModelServices")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewRemoteApplicationWorker == nil {
		return errors.NotValidf("nil NewRemoteApplicationWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.APIRemoteRelationCallerName,
			config.DomainServicesName,
		},
		Start: func(context context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			if !config.Active {
				return nil, dependency.ErrUninstall
			}

			var agent agent.Agent
			if err := getter.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var apiConn api.Connection
			if err := getter.Get(config.APICallerName, &apiConn); err != nil {
				return nil, errors.Trace(err)
			}

			var apiRemoteCallerGetter apiremoterelationcaller.APIRemoteCallerGetter
			if err := getter.Get(config.APIRemoteRelationCallerName, &apiRemoteCallerGetter); err != nil {
				return nil, errors.Trace(err)
			}

			crossModelRelationService, err := config.GetCrossModelServices(getter, config.DomainServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				ModelUUID: agent.CurrentConfig().Model().Id(),

				CrossModelRelationService: crossModelRelationService,

				RelationsFacade:            config.NewLocalRemoteRelationFacade(apiConn),
				RemoteRelationClientGetter: config.NewRemoteRelationClientGetter(apiRemoteCallerGetter),

				NewRemoteApplicationWorker: config.NewRemoteApplicationWorker,

				Clock:  config.Clock,
				Logger: config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// GetCrossModelServices returns the cross-model relation services
// from the dependency engine.
func GetCrossModelServices(getter dependency.Getter, domainServicesName string) (CrossModelRelationService, error) {
	var services services.DomainServices
	if err := getter.Get(domainServicesName, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return services.CrossModelRelation(), nil
}

// NewLocalRemoteRelationFacade creates a new RemoteRelationsFacade using the
// APICaller.
func NewLocalRemoteRelationFacade(apiCaller base.APICaller) RemoteRelationsFacade {
	return remoterelations.NewClient(apiCaller)
}

// NewRemoteRelationClientGetter creates a new RemoteRelationClientGetter
// using the provided APIRemoteCallerGetter.
func NewRemoteRelationClientGetter(getter apiremoterelationcaller.APIRemoteCallerGetter) RemoteRelationClientGetter {
	return remoteRelationClientGetter{
		getter: getter,
	}
}

type remoteRelationClientGetter struct {
	getter apiremoterelationcaller.APIRemoteCallerGetter
}

// GetRemoteRelationClient returns a RemoteModelRelationsClient for the given model UUID.
func (r remoteRelationClientGetter) GetRemoteRelationClient(ctx context.Context, modelUUID string) (RemoteModelRelationsClient, error) {
	client, err := r.getter.GetConnectionForModel(ctx, model.UUID(modelUUID))
	if err != nil {
		return nil, errors.Trace(err)
	}

	return crossmodelrelations.NewClient(client), nil
}
