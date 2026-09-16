// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremotecaller

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/api"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
)

// APIRemoteCallers is an interface that represents the remote API callers.
type APIRemoteCallers interface {
	// GetAPIRemotes returns the current API connections. It is expected that
	// the caller will call this method just before making an API call to ensure
	// that the connection is still valid. The caller must not cache the
	// connections as they may change over time.
	GetAPIRemotes() ([]RemoteConnection, error)
}

// APIRemoteSubscriber is an interface that represents a subscriber to changes
// in the set of API remotes.
type APIRemoteSubscriber interface {
	APIRemoteCallers

	// Subscribe subscribes to changes in the set of API remotes.
	Subscribe() (Subscription, error)
}

// Subscription represents a subscription to changes in the set of API remotes.
type Subscription interface {
	// Changes returns a channel that signals when the set of API remotes has
	// changed.
	Changes() <-chan struct{}

	// Close closes the subscription.
	Close()
}

// APIInfoProvider returns the current API connection details. It is
// called each time the worker starts so that bounced workers do not
// keep stale values.
type APIInfoProvider interface {
	APIInfo() (*api.Info, error)
}

// RemoteCallerServices provides the controller-model services used to build
// direct controller remote endpoints.
type RemoteCallerServices interface {
	ControllerConfig(context.Context) (controller.Config, error)
	Network() ControllerNetworkService
}

// GetRemoteCallerServices retrieves the services required by the remote
// caller from the object store service factory.
type GetRemoteCallerServices func(dependency.Getter, string) (RemoteCallerServices, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	ObjectStoreServicesName string

	// APIInfo returns the current API connection details. It is called
	// each time the worker starts so that bounced workers do not keep
	// stale values.
	APIInfo APIInfoProvider

	// Origin is the tag identifying the calling controller. It is
	// passed directly because it never changes during the lifetime
	// of the agent.
	Origin names.Tag

	Clock  clock.Clock
	Logger logger.Logger

	GetRemoteCallerServices GetRemoteCallerServices
	NewWorker               func(WorkerConfig) (worker.Worker, error)
}

func (config ManifoldConfig) Validate() error {
	if config.APIInfo == nil {
		return errors.NotValidf("missing APIInfo")
	}
	if config.Origin == nil {
		return errors.NotValidf("missing Origin")
	}
	if config.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	if config.GetRemoteCallerServices == nil {
		config.GetRemoteCallerServices = getRemoteCallerServices
	}
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			services, err := config.GetRemoteCallerServices(getter, config.ObjectStoreServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			controllerConfig, err := services.ControllerConfig(ctx)
			if err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				ControllerNetworkService: services.Network(),
				APIPort:                  controllerConfig.APIPort(),
				APIInfo:                  config.APIInfo,
				APIOpener:                api.Open,
				Origin:                   config.Origin,
				NewRemote:                NewRemoteServer,
				Logger:                   config.Logger,
				Clock:                    config.Clock,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
		Output: remoteOutput,
	}
}

type remoteCallerServices struct {
	services services.ObjectStoreServices
}

func getRemoteCallerServices(getter dependency.Getter, name string) (RemoteCallerServices, error) {
	var objectStoreServices services.ObjectStoreServices
	if err := getter.Get(name, &objectStoreServices); err != nil {
		return nil, errors.Trace(err)
	}
	return remoteCallerServices{services: objectStoreServices}, nil
}

func (s remoteCallerServices) ControllerConfig(ctx context.Context) (controller.Config, error) {
	return s.services.ControllerConfig().ControllerConfig(ctx)
}

func (s remoteCallerServices) Network() ControllerNetworkService {
	return s.services.Network()
}

func remoteOutput(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*remoteWorker)
	if !ok {
		return errors.Errorf("expected input of type remoteWorker, got %T", in)
	}

	switch out := out.(type) {
	case *APIRemoteCallers:
		var target APIRemoteCallers = w
		*out = target
	case *APIRemoteSubscriber:
		var target APIRemoteSubscriber = w
		*out = target
	default:
		return errors.Errorf("expected output of APIRemoteCallers or APIRemoteSubscriber, got %T", out)
	}
	return nil
}
