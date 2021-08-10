// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package leaseconsumer

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
)

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
	return nil
}

// Worker is a worker that manages requests from the lease store.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// NewWorker returns a new apiserver lease consumer worker.
func NewWorker(config Config) (worker.Worker, error) {
	w := &Worker{
		config: config,
	}

	var h http.Handler = NewHandler(w.catacomb.Dying())
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
	// TODO (stickupkid): Take a connection and unmarshal the command operation
	// and apply it to the event target.
	return nil
}

func controllerAuthorizer(authInfo httpcontext.AuthInfo) error {
	if authInfo.Controller {
		return nil
	}
	return errors.New("controller agents only")
}
