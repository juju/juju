// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the resources needed to run an httpserverargs
// worker.
type ManifoldConfig struct {
	ClockName string
	StateName string

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
	if config.NewStateAuthenticator == nil {
		return errors.NotValidf("nil NewStateAuthenticator")
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

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	mux := apiserverhttp.NewMux()
	abort := make(chan struct{})
	authenticator, err := config.NewStateAuthenticator(statePool, mux, clock, abort)
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	w := newWorker(mux, authenticator, func() {
		close(abort)
		stTracker.Done()
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
			config.StateName,
		},
		Start:  config.start,
		Output: manifoldOutput,
	}
}

var (
	muxType           = reflect.TypeOf(&apiserverhttp.Mux{})
	authenticatorType = reflect.TypeOf((*httpcontext.LocalMacaroonAuthenticator)(nil)).Elem()
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
	authenticator httpcontext.LocalMacaroonAuthenticator,
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
	authenticator httpcontext.LocalMacaroonAuthenticator
	tomb          tomb.Tomb
}

func (w *argsWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *argsWorker) Wait() error {
	return w.tomb.Wait()
}
