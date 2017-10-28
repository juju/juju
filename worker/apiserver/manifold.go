// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName       string
	CertWatcherName string
	ClockName       string
	StateName       string

	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	LoginValidator                    LoginValidator
	Hub                               *pubsub.StructuredHub

	// TODO(axw) once we pass in StatePool, we can get rid of this.
	SetStatePool           func(*state.StatePool)
	NewStoreAuditEntryFunc func(*state.State) StoreAuditEntryFunc
	NewWorker              func(Config) (worker.Worker, error)
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.CertWatcherName == "" {
		return errors.NotValidf("empty CertWatcherName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.LoginValidator == nil {
		return errors.NotValidf("nil LoginValidator")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.SetStatePool == nil {
		return errors.NotValidf("nil SetStatePool")
	}
	if config.NewStoreAuditEntryFunc == nil {
		return errors.NotValidf("nil NewStoreAuditEntryFunc")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.CertWatcherName,
			config.ClockName,
			config.StateName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
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

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	st, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		AgentConfig:                       agent.CurrentConfig(),
		Clock:                             clock,
		State:                             st,
		PrometheusRegisterer:              config.PrometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
		LoginValidator:                    config.LoginValidator,
		SetStatePool:                      config.SetStatePool,
		Hub:                               config.Hub,
		GetCertificate:                    getCertificate,
		NewServer:                         newServerShim,
		StoreAuditEntry:                   config.NewStoreAuditEntryFunc(st),
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}

	go func() {
		w.Wait()
		stTracker.Done()
	}()
	return w, nil
}
