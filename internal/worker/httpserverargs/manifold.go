// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"context"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// ManifoldConfig holds the resources needed to run an httpserverargs
// worker.
type ManifoldConfig struct {
	ClockName          string
	StateName          string
	DomainServicesName string

	NewStateAuthenticator NewStateAuthenticatorFunc
}

// Validate checks that we have all of the things we need.
func (config ManifoldConfig) Validate() error {
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.NewStateAuthenticator == nil {
		return errors.NotValidf("nil NewStateAuthenticator")
	}
	return nil
}

func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var controllerDomainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, _, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := newWorker(workerConfig{
		statePool:               statePool,
		domainServicesGetter:    domainServicesGetter,
		controllerConfigService: controllerDomainServices.ControllerConfig(),
		accessService:           controllerDomainServices.Access(),
		macaroonService:         controllerDomainServices.Macaroon(),
		mux:                     apiserverhttp.NewMux(),
		clock:                   clock,
		newStateAuthenticatorFn: config.NewStateAuthenticator,
	})
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	return common.NewCleanupWorker(w, func() { _ = stTracker.Done() }), nil
}

// Manifold returns a dependency.Manifold to run a worker to hold the
// http server mux and authenticator. This means that we can ensure
// that all workers that need to register with them can be finished
// starting up before the httpserver responds to connections.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.StateName,
			config.DomainServicesName,
		},
		Start:  config.start,
		Output: manifoldOutput,
	}
}

var (
	muxType           = reflect.TypeOf(&apiserverhttp.Mux{})
	authenticatorType = reflect.TypeOf((*macaroon.LocalMacaroonAuthenticator)(nil)).Elem()
)

func manifoldOutput(in worker.Worker, out any) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*argsWorker)
	if !ok {
		return errors.Errorf("expected worker of type *argsWorker, got %T", in)
	}
	rv := reflect.ValueOf(out)
	if rt := rv.Type(); rt.Kind() == reflect.Ptr {
		elemType := rt.Elem()
		switch {
		case muxType.AssignableTo(elemType):
			rv.Elem().Set(reflect.ValueOf(w.cfg.mux))
			return nil
		case authenticatorType.AssignableTo(elemType):
			rv.Elem().Set(reflect.ValueOf(w.authenticator))
			return nil
		}
	}
	return errors.Errorf("unexpected output type %T", out)
}
