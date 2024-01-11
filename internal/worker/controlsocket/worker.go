// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package controlsocket

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/juju/sockets"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
type logger any

var _ logger = struct{}{}

// Logger represents the methods used by the worker to log information.
type Logger interface {
	Errorf(string, ...any)
	Warningf(string, ...any)
	Infof(string, ...any)
	Debugf(string, ...any)
	Tracef(string, ...any)
}

// Config represents configuration for the controlsocket worker.
type Config struct {
	State      State
	Logger     Logger
	SocketName string
}

// Validate returns an error if config cannot drive the Worker.
func (config Config) Validate() error {
	if config.State == nil {
		return errors.NotValidf("nil State")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	return nil
}

// Worker is a controlsocket worker.
type Worker struct {
	config   Config
	tomb     tomb.Tomb
	listener net.Listener
}

// NewWorker returns a controlsocket worker with the given config.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	l, err := sockets.Listen(sockets.Socket{
		Address: config.SocketName,
		Network: "unix",
	})
	if err != nil {
		return nil, errors.Annotate(err, "unable to listen on unix socket")
	}
	config.Logger.Debugf("controlsocket worker listening on socket %q", config.SocketName)

	w := &Worker{
		config:   config,
		listener: l,
	}
	w.tomb.Go(w.run)
	return w, nil
}

func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

// run listens on the control socket and handles requests.
func (w *Worker) run() error {
	router := mux.NewRouter()
	w.registerHandlers(router)

	srv := http.Server{Handler: router}
	defer func() {
		err := srv.Close()
		if err != nil {
			w.config.Logger.Warningf("error closing HTTP server: %v", err)
		}
	}()

	go func() {
		// Wait for the tomb to start dying and then shut the server down.
		<-w.tomb.Dying()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			w.config.Logger.Warningf("error shutting down HTTP server: %v", err)
		}
	}()

	w.config.Logger.Debugf("controlsocket worker now serving")
	defer w.config.Logger.Debugf("controlsocket worker serving finished")
	if err := srv.Serve(w.listener); err != http.ErrServerClosed {
		return err
	}
	return nil
}
