// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"

	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an HTTP server
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName       string
	CertWatcherName string
	StateName       string
	MuxName         string
	StartAfter      []string

	PrometheusRegisterer prometheus.Registerer

	NewTLSConfig func(*state.State, func() *tls.Certificate) (*tls.Config, error)
	NewWorker    func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.CertWatcherName == "" {
		return errors.NotValidf("empty CertWatcherName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
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
	allInputs := []string{
		config.AgentName,
		config.CertWatcherName,
		config.StateName,
		config.MuxName,
	}
	allInputs = append(allInputs, config.StartAfter...)
	return dependency.Manifold{
		Inputs: allInputs,
		Start:  config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (_ worker.Worker, err error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
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

	for _, name := range config.StartAfter {
		// We don't actually need anything from these workers, but we
		// shouldn't start until they're available.
		if err := context.Get(name, nil); err != nil {
			return nil, errors.Trace(err)
		}
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

	w, err := config.NewWorker(Config{
		AgentConfig:          agent.CurrentConfig(),
		PrometheusRegisterer: config.PrometheusRegisterer,
		TLSConfig:            tlsConfig,
		Mux:                  mux,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { stTracker.Done() }), nil
}
