// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package leaseconsumer

import (
	"net/http"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/core/raftlease"
)

const applyTimeout = 5 * time.Second

// RaftApplier allows applying a command to the raft FSM.
type RaftApplier interface {
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
}

// Config is the configuration required for running an aposerver-based
// leases consumer worker.
type Config struct {
	// Authenticator is the HTTP request authenticator to use for
	// the lease consumer endpoint.
	Authenticator httpcontext.Authenticator

	// Mux is the API server HTTP mux into which the handler will
	// be installed.
	Mux *apiserverhttp.Mux

	// Path is the path of the lease consumer HTTP endpoint.
	Path string

	// Raft applies any changes to the underlying raft store.
	Raft RaftApplier

	// NotifyTarget updates the raft lease target.
	Target raftlease.NotifyTarget

	// PrometheusRegisterer ensures we record the applying metrics.
	PrometheusRegisterer prometheus.Registerer

	// Clock allows for better testing of the worker.
	Clock clock.Clock

	Logger Logger
}

// Validate validates the raft worker configuration.
func (config Config) Validate() error {
	if config.Authenticator == nil {
		return errors.NotValidf("nil Authenticator")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.Path == "" {
		return errors.NotValidf("empty Path")
	}
	if config.Raft == nil {
		return errors.NotValidf("nil Raft")
	}
	if config.Target == nil {
		return errors.NotValidf("nil Target")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

type operation struct {
	Command  string
	Callback func(error)
}

// Worker is a worker that manages requests from the lease store.
type Worker struct {
	catacomb   catacomb.Catacomb
	config     Config
	operations chan operation
	metrics    *metricsCollector
}

// NewWorker returns a new apiserver lease consumer worker.
func NewWorker(config Config) (worker.Worker, error) {
	w := &Worker{
		config:     config,
		operations: make(chan operation),
		metrics:    newMetricsCollector(config.Clock),
	}

	var h http.Handler = NewHandler(w.operations, w.catacomb.Dying(), config.Clock, config.Logger)
	h = &httpcontext.BasicAuthHandler{
		Handler:       h,
		Authenticator: w.config.Authenticator,
		Authorizer:    httpcontext.AuthorizerFunc(controllerAuthorizer),
	}

	_ = w.config.Mux.AddHandler("GET", w.config.Path, h)

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: func() error {
			defer w.config.Mux.RemoveHandler("GET", w.config.Path)
			return w.loop()
		},
		Init: []worker.Worker{},
	}); err != nil {
		w.config.Mux.RemoveHandler("GET", w.config.Path)
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {
	_ = w.config.PrometheusRegisterer.Register(w.metrics)
	defer w.config.PrometheusRegisterer.Unregister(w.metrics)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case op := <-w.operations:
			op.Callback(w.processCommand(op.Command))
		}
	}
}

func (w *Worker) processCommand(command string) error {
	start := w.config.Clock.Now()

	future := w.config.Raft.Apply([]byte(command), applyTimeout)
	if err := future.Error(); err != nil {
		return errors.Trace(err)
	}

	w.metrics.record(start, "apply")

	response := future.Response()
	fsmResponse, ok := response.(raftlease.FSMResponse)
	if !ok {
		// This should never happen.
		panic(errors.Errorf("programming error: expected an FSMResponse, got %T: %#v", response, response))
	}

	fsmResponse.Notify(w.config.Target)

	return fsmResponse.Error()
}

func controllerAuthorizer(authInfo httpcontext.AuthInfo) error {
	if authInfo.Controller {
		return nil
	}
	return errors.New("controller agents only")
}
