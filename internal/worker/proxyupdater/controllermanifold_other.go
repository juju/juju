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
)

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
