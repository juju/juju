// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlsocket

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/socketlistener"
)

// ManifoldConfig describes the dependencies required by the controlsocket worker.
type ManifoldConfig struct {
	DomainServicesName string
	Logger             logger.Logger
	NewWorker          func(Config) (worker.Worker, error)
	NewSocketListener  func(socketlistener.Config) (SocketListener, error)
	SocketName         string
}

// Manifold returns a Manifold that encapsulates the controlsocket worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker func")
	}
	if cfg.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	if cfg.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (_ worker.Worker, err error) {
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServices services.ControllerDomainServices
	if err = getter.Get(cfg.DomainServicesName, &domainServices); err != nil {
		return nil, errors.Trace(err)
	}

	controllerSerivce := domainServices.Controller()
	controllerModelUUID, err := controllerSerivce.ControllerModelUUID(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var w worker.Worker
	w, err = cfg.NewWorker(Config{
		AccessService:       domainServices.Access(),
		Logger:              cfg.Logger,
		SocketName:          cfg.SocketName,
		NewSocketListener:   cfg.NewSocketListener,
		ControllerModelUUID: controllerModelUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// SocketListener describes a worker that listens on a unix socket.
type SocketListener interface {
	worker.Worker
}

// NewSocketListener is a function that creates a new socket listener.
func NewSocketListener(config socketlistener.Config) (SocketListener, error) {
	return socketlistener.NewSocketListener(config)
}
