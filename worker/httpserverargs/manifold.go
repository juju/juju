// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserverargs

import (
	"net/http"
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the resources needed to run an httpserverargs
// worker.
type ManifoldConfig struct {
	ClockName          string
	ControllerPortName string
	StateName          string

	AgentName string
	HubName   string

	NewStateAuthenticator NewStateAuthenticatorFunc
	NewNotFoundHandler    func(*api.Info, *pubsub.StructuredHub) (http.Handler, error)
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
	if config.NewStateAuthenticator == nil {
		return errors.NotValidf("nil NewStateAuthenticator")
	}

	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.HubName == "" {
		return errors.NotValidf("empty HubName")
	}
	if config.NewNotFoundHandler == nil {
		return errors.NotValidf("nil NewNotFoundHandler")
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

	var hub *pubsub.StructuredHub
	if err := context.Get(config.HubName, &hub); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	apiInfo, ok := agent.CurrentConfig().APIInfo()
	if !ok {
		return nil, dependency.ErrMissing
	}

	var options []apiserverhttp.MuxOption
	handler, err := config.NewNotFoundHandler(apiInfo, hub)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if handler != nil {
		options = append(options, apiserverhttp.NotFoundHandlerOption(handler))
	}

	mux := apiserverhttp.NewMux(options...)
	abort := make(chan struct{})
	authenticator, err := config.NewStateAuthenticator(statePool, mux, clock, abort)
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
			config.AgentName,
			config.HubName,
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

// NewNotFoundHandler allows the creation of a new not found handler for the
// apiserver mux.
func NewNotFoundHandler(_ *api.Info, _ *pubsub.StructuredHub) (http.Handler, error) {
	// Returning nil here, falls back to the old not found handler that is
	// in the PatterServerMux.
	return nil, nil
}
