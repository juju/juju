// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"
	"crypto/tls"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/acme/autocert"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pki"
	pkitls "github.com/juju/juju/internal/pki/tls"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// ManifoldConfig holds the information necessary to run an HTTP server
// in a dependency.Engine.
type ManifoldConfig struct {
	AuthorityName      string
	MuxName            string
	StateName          string
	DomainServicesName string

	// We don't use these in the worker, but we want to prevent the
	// httpserver from starting until they're running so that all of
	// their handlers are registered.
	APIServerName string

	AgentName            string
	Clock                clock.Clock
	MuxShutdownWait      time.Duration
	LogDir               string
	PrometheusRegisterer prometheus.Registerer

	Logger logger.Logger

	GetControllerConfig func(context.Context, ControllerConfigGetter) (controller.Config, error)
	NewTLSConfig        func(string, string, autocert.Cache, SNIGetterFunc, logger.Logger) *tls.Config
	NewWorker           func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.APIServerName == "" {
		return errors.NotValidf("empty APIServerName")
	}
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.GetControllerConfig == nil {
		return errors.NotValidf("nil GetControllerConfig")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil logger")
	}
	if config.NewTLSConfig == nil {
		return errors.NotValidf("nil NewTLSConfig")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.MuxShutdownWait < 1*time.Minute {
		return errors.NotValidf("MuxShutdownWait %v", config.MuxShutdownWait)
	}
	if config.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AuthorityName,
			config.StateName,
			config.DomainServicesName,
			config.MuxName,
			config.APIServerName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := getter.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := getter.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	// We don't actually need anything from these workers, but we
	// shouldn't start until they're available.
	if err := getter.Get(config.APIServerName, nil); err != nil {
		return nil, errors.Trace(err)
	}

	var controllerDomainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	newCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	controllerConfig, err := config.GetControllerConfig(newCtx, controllerDomainServices.ControllerConfig())
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	tlsConfig := config.NewTLSConfig(
		controllerConfig.AutocertDNSName(),
		controllerConfig.AutocertURL(),
		controllerDomainServices.AutocertCache(),
		pkitls.AuthoritySNITLSGetter(authority, config.Logger),
		config.Logger,
	)

	w, err := config.NewWorker(Config{
		AgentName:            config.AgentName,
		Clock:                config.Clock,
		PrometheusRegisterer: config.PrometheusRegisterer,
		TLSConfig:            tlsConfig,
		Mux:                  mux,
		MuxShutdownWait:      config.MuxShutdownWait,
		LogDir:               config.LogDir,
		Logger:               config.Logger,
		APIPort:              controllerConfig.APIPort(),
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
