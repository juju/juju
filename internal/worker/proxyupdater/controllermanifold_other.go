//go:build !dqlite || !linux

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"

	"github.com/juju/proxy"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
)

// GetControllerDomainServicesFunc extracts controller domain services from a
// dependency getter.
type GetControllerDomainServicesFunc func(dependency.Getter, string) (ControllerDomainServices, error)

// GetDomainServicesFunc extracts model domain services for the supplied model
// UUID from a dependency getter.
type GetDomainServicesFunc func(context.Context, dependency.Getter, string, coremodel.UUID) (DomainServices, error)

// ControllerManifoldConfig defines a proxy updater manifold backed directly by
// domain services instead of the API facade.
type ControllerManifoldConfig struct {
	DomainServicesName          string
	ProxyReadyGateName          string
	Logger                      logger.Logger
	WorkerFunc                  func(Config) (worker.Worker, error)
	GetControllerDomainServices GetControllerDomainServicesFunc
	GetDomainServices           GetDomainServicesFunc
	SupportLegacyValues         bool
	ExternalUpdate              func(proxy.Settings) error
	InProcessUpdate             func(proxy.Settings) error
	RunFunc                     func(string, string, ...string) (string, error)
}

func ControllerManifold(config ControllerManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
			config.ProxyReadyGateName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			return newNoopWorker(), nil
		},
	}
}

// ControllerDomainServices exposes controller services used by this worker.
type ControllerDomainServices any

// DomainServices exposes model services used by this worker.
type DomainServices any

// ModelConfigService provides access to the model's configuration.
type ModelConfigService any

// ControllerNodeService provides API address information for no-proxy values.
type ControllerNodeService any

// ModelService provides controller model information.
type ModelService any

// GetControllerDomainServices retrieves controller services from the
// dependency getter.
func GetControllerDomainServices(getter dependency.Getter, name string) (ControllerDomainServices, error) {
	return nil, nil
}

// GetDomainServices retrieves model services from the dependency getter.
func GetDomainServices(ctx context.Context, getter dependency.Getter, name string, modelUUID coremodel.UUID) (DomainServices, error) {
	return nil, nil
}

type noopWorker struct {
	tomb tomb.Tomb
}

func newNoopWorker() worker.Worker {
	w := &noopWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return tomb.ErrDying
	})
	return w
}

func (w *noopWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *noopWorker) Wait() error {
	return w.tomb.Wait()
}
