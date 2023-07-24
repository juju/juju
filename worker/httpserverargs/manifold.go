// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	stdcontext "context"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	controllerconfigstate "github.com/juju/juju/domain/controllerconfig/state"
	workerstate "github.com/juju/juju/worker/state"
)

// Logger is an interface that provides logging
type Logger interface {
	Debugf(string, ...interface{})
}

// ControllerConfigService is an interface that provides the controller
// configuration.
type ControllerConfigService interface {
	ControllerConfig(context stdcontext.Context) (controller.Config, error)
}

// ManifoldConfig holds the resources needed to run an httpserverargs
// worker.
type ManifoldConfig struct {
	ClockName          string
	ControllerPortName string
	StateName          string
	ChangeStreamName   string
	Logger             Logger

	NewStateAuthenticator      NewStateAuthenticatorFunc
	NewControllerConfigService func(changestream.WatchableDBGetter, Logger) ControllerConfigService
}

// Validate checks that we have all of the things we need.
func (config ManifoldConfig) Validate() error {
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.ControllerPortName == "" {
		return errors.NotValidf("empty ControllerPortName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewStateAuthenticator == nil {
		return errors.NotValidf("nil NewStateAuthenticator")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.NewControllerConfigService == nil {
		return errors.NotValidf("nil NewControllerConfigService")
	}
	return nil
}

func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure that the controller-port worker is running.
	if err := context.Get(config.ControllerPortName, nil); err != nil {
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

	var dbGetter changestream.WatchableDBGetter
	if err := context.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	ctrlConfigService := config.NewControllerConfigService(dbGetter, config.Logger)

	mux := apiserverhttp.NewMux()
	abort := make(chan struct{})
	authenticator, err := config.NewStateAuthenticator(statePool, mux, clock, abort, ctrlConfigService)
	if err != nil {
		_ = stTracker.Done()
		return nil, errors.Trace(err)
	}
	w := newWorker(mux, authenticator, func() {
		close(abort)
		_ = stTracker.Done()
	})
	return w, nil
}

// Manifold returns a dependency.Manifold to run a worker to hold the
// http server mux and authenticator. This means that we can ensure
// that all workers that need to register with them can be finished
// starting up before the httpserver responds to connections.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ClockName,
			config.ControllerPortName,
			config.StateName,
			config.ChangeStreamName,
		},
		Start:  config.start,
		Output: manifoldOutput,
	}
}

var (
	muxType           = reflect.TypeOf(&apiserverhttp.Mux{})
	authenticatorType = reflect.TypeOf((*macaroon.LocalMacaroonAuthenticator)(nil)).Elem()
)

func manifoldOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(*argsWorker)
	if !ok {
		return errors.Errorf("expected worker of type *argsWorker, got %T", in)
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

func newWorker(
	mux *apiserverhttp.Mux,
	authenticator macaroon.LocalMacaroonAuthenticator,
	cleanup func(),
) worker.Worker {
	w := argsWorker{
		mux:           mux,
		authenticator: authenticator,
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		cleanup()
		return nil
	})
	return &w
}

type argsWorker struct {
	mux           *apiserverhttp.Mux
	authenticator macaroon.LocalMacaroonAuthenticator
	tomb          tomb.Tomb
}

func (w *argsWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *argsWorker) Wait() error {
	return w.tomb.Wait()
}

// NewControllerConfigService returns a new ControllerConfigService.
func NewControllerConfigService(dbGetter changestream.WatchableDBGetter, logger Logger) ControllerConfigService {
	return controllerconfigservice.NewService(
		controllerconfigstate.NewState(coredatabase.NewTxnRunnerFactoryForNamespace(
			dbGetter.GetWatchableDB,
			coredatabase.ControllerNS,
		)),
		domain.NewWatcherFactory(
			func() (changestream.WatchableDB, error) {
				return dbGetter.GetWatchableDB(coredatabase.ControllerNS)
			},
			logger,
		),
	)
}
