// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	stdcontext "context"
	"crypto/tls"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/acme/autocert"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/internal/pki"
	pkitls "github.com/juju/juju/internal/pki/tls"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/servicefactory"
	workerstate "github.com/juju/juju/worker/state"
)

type Logger interface {
	Debugf(string, ...interface{})
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
	Logf(loggo.Level, string, ...interface{})
	Warningf(string, ...interface{})
}

// ManifoldConfig holds the information necessary to run an HTTP server
// in a dependency.Engine.
type ManifoldConfig struct {
	AuthorityName      string
	HubName            string
	MuxName            string
	StateName          string
	ServiceFactoryName string

	// We don't use these in the worker, but we want to prevent the
	// httpserver from starting until they're running so that all of
	// their handlers are registered.
	APIServerName string

	AgentName            string
	Clock                clock.Clock
	MuxShutdownWait      time.Duration
	LogDir               string
	PrometheusRegisterer prometheus.Registerer

	Logger Logger

	GetControllerConfig func(stdcontext.Context, ControllerConfigGetter) (controller.Config, error)
	NewTLSConfig        func(string, string, autocert.Cache, SNIGetterFunc, Logger) *tls.Config
	NewWorker           func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.HubName == "" {
		return errors.NotValidf("empty HubName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
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
			config.HubName,
			config.StateName,
			config.ServiceFactoryName,
			config.MuxName,
			config.APIServerName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := context.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var hub *pubsub.StructuredHub
	if err := context.Get(config.HubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	// We don't actually need anything from these workers, but we
	// shouldn't start until they're available.
	if err := context.Get(config.APIServerName, nil); err != nil {
		return nil, errors.Trace(err)
	}

	var controllerServiceFactory servicefactory.ControllerServiceFactory
	if err := context.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	ctx, cancel := stdcontext.WithCancel(stdcontext.Background())
	defer cancel()

	controllerConfig, err := config.GetControllerConfig(ctx, controllerServiceFactory.ControllerConfig())
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Annotate(err, "unable to get controller config")
	}
	tlsConfig := config.NewTLSConfig(
		controllerConfig.AutocertDNSName(),
		controllerConfig.AutocertURL(),
		controllerServiceFactory.AutocertCache(),
		pkitls.AuthoritySNITLSGetter(authority, config.Logger),
		config.Logger,
	)

	w, err := config.NewWorker(Config{
		AgentName:            config.AgentName,
		Clock:                config.Clock,
		PrometheusRegisterer: config.PrometheusRegisterer,
		Hub:                  hub,
		TLSConfig:            tlsConfig,
		Mux:                  mux,
		MuxShutdownWait:      config.MuxShutdownWait,
		LogDir:               config.LogDir,
		Logger:               config.Logger,
		APIPort:              controllerConfig.APIPort(),
		APIPortOpenDelay:     controllerConfig.APIPortOpenDelay(),
		ControllerAPIPort:    controllerConfig.ControllerAPIPort(),
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}
