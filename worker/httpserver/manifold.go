// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"
	"reflect"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/state"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an HTTP server
// in a dependency.Engine.
type ManifoldConfig struct {
	AgentName       string
	CertWatcherName string
	ClockName       string
	StateName       string

	PrometheusRegisterer prometheus.Registerer

	NewStateAuthenticator NewStateAuthenticatorFunc
	NewTLSConfig          func(*state.State, func() *tls.Certificate) (*tls.Config, error)
	NewWorker             func(Config) (worker.Worker, error)
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
	if config.NewStateAuthenticator == nil {
		return errors.NotValidf("nil NewStateAuthenticator")
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
			config.AgentName,
			config.CertWatcherName,
			config.ClockName,
			config.StateName,
		},
		Start:  config.start,
		Output: manifoldOutput,
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

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
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

	mux := apiserverhttp.NewMux()
	abort := make(chan struct{})
	authenticator, err := config.NewStateAuthenticator(statePool, mux, clock, abort)
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
		close(abort)
		return nil, errors.Trace(err)
	}
	return &wrapperWorker{
		Worker:        w,
		mux:           mux,
		authenticator: authenticator,
		cleanup: func() {
			close(abort)
			stTracker.Done()
		},
	}, nil
}

var (
	muxType           = reflect.TypeOf(&apiserverhttp.Mux{})
	authenticatorType = reflect.TypeOf((*httpcontext.LocalMacaroonAuthenticator)(nil)).Elem()
)

func manifoldOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(*wrapperWorker)
	if !ok {
		return errors.Errorf("expected worker of type %T, got %T", w, in)
	}
	rv := reflect.ValueOf(out)
	if rt := rv.Type(); rt.Kind() == reflect.Ptr {
		elemType := rt.Elem()
		switch {
		case muxType.AssignableTo(elemType):
			rv.Elem().Set(reflect.ValueOf(w.mux))
			return nil
		case authenticatorType.AssignableTo(elemType):
			rv.Elem().Set(reflect.ValueOf(w.authenticator))
			return nil
		}
	}
	return errors.Errorf("unexpected output type %T", out)
}

type wrapperWorker struct {
	worker.Worker
	mux           *apiserverhttp.Mux
	authenticator httpcontext.LocalMacaroonAuthenticator
	cleanupOnce   sync.Once
	cleanup       func()
}

func (w *wrapperWorker) Wait() error {
	err := w.Worker.Wait()
	w.cleanupOnce.Do(w.cleanup)
	return err
}
