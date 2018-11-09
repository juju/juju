// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an HTTP server
// in a dependency.Engine.
type ManifoldConfig struct {
	CertWatcherName string
	HubName         string
	MuxName         string
	StateName       string

	// We don't use these in the worker, but we want to prevent the
	// httpserver from starting until they're running so that all of
	// their handlers are registered.
	RaftTransportName string
	APIServerName     string

	Clock                clock.Clock
	PrometheusRegisterer prometheus.Registerer

	GetControllerConfig func(*state.State) (controller.Config, error)
	NewTLSConfig        func(*state.State, func() *tls.Certificate) (*tls.Config, http.Handler, error)
	NewWorker           func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.CertWatcherName == "" {
		return errors.NotValidf("empty CertWatcherName")
	}
	if config.HubName == "" {
		return errors.NotValidf("empty HubName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.RaftTransportName == "" {
		return errors.NotValidf("empty RaftTransportName")
	}
	if config.APIServerName == "" {
		return errors.NotValidf("empty APIServerName")
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
	if config.NewTLSConfig == nil {
		return errors.NotValidf("nil NewTLSConfig")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an HTTP server
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.CertWatcherName,
			config.HubName,
			config.StateName,
			config.MuxName,
			config.RaftTransportName,
			config.APIServerName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (_ worker.Worker, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var hub *pubsub.StructuredHub
	if err := context.Get(config.HubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var getCertificate func() *tls.Certificate
	if err := context.Get(config.CertWatcherName, &getCertificate); err != nil {
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
	if err := context.Get(config.RaftTransportName, nil); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			stTracker.Done()
		}
	}()

	systemState := statePool.SystemState()
	tlsConfig, err := config.NewTLSConfig(systemState, getCertificate)
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := config.GetControllerConfig(systemState)
	if err != nil {
		return nil, errors.Annotate(err, "unable to get controller config")
	}

	w, err := config.NewWorker(Config{
		Clock:                config.Clock,
		PrometheusRegisterer: config.PrometheusRegisterer,
		Hub:                  hub,
		TLSConfig:            tlsConfig,
		Mux:                  mux,
		APIPort:              controllerConfig.APIPort(),
		APIPortOpenDelay:     controllerConfig.APIPortOpenDelay(),
		ControllerAPIPort:    controllerConfig.ControllerAPIPort(),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}
