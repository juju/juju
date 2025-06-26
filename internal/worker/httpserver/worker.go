// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/core/logger"
)

var (
	// ShutdownTimeout is how long the http server will wait for active
	// connections to close.
	ShutdownTimeout = 30 * time.Second
)

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentName            string
	Clock                clock.Clock
	TLSConfig            *tls.Config
	Mux                  *apiserverhttp.Mux
	MuxShutdownWait      time.Duration
	LogDir               string
	Logger               logger.Logger
	PrometheusRegisterer prometheus.Registerer
	APIPort              int
}

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.TLSConfig == nil {
		return errors.NotValidf("nil TLSConfig")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	return nil
}

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: config,
		logger: config.Logger,
		status: "starting",
	}
	listener, err := w.newSimpleListener()
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.listener = listener

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "httpserver",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		listener.Close()
		return nil, errors.Trace(err)
	}
	return w, nil
}

type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	listener *simpleListener
	logger   logger.Logger

	// mu controls access to both status and reporter.
	mu     sync.Mutex
	status string
}

// Kill implements worker.Kill.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements worker.Wait.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report provides information for the engine report.
func (w *Worker) Report() map[string]interface{} {
	w.mu.Lock()
	result := map[string]interface{}{
		"api-port": w.config.APIPort,
		"status":   w.status,
	}
	w.mu.Unlock()
	return result
}

// URL returns the base URL of the HTTP server of the form
// https://ipaddr:port with no trailing slash.
func (w *Worker) URL() string {
	return w.listener.URL()
}

func (w *Worker) loop() error {
	ctx, cancel := w.scopedContext()
	defer cancel()

	serverLog := log.New(&loggerWrapper{
		level:  logger.WARNING,
		logger: w.logger,
	}, "", 0) // no prefix and no flags so log.Logger doesn't add extra prefixes
	server := &http.Server{
		Handler:   w.config.Mux,
		TLSConfig: w.config.TLSConfig,
		ErrorLog:  serverLog,
	}

	go func() {
		err := server.Serve(tls.NewListener(w.listener, w.config.TLSConfig))
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			w.logger.Errorf(ctx, "server finished with error %v", err)
			return
		}
		w.logger.Infof(ctx, "server finished successfully")
	}()

	w.mu.Lock()
	w.status = "running"
	w.mu.Unlock()

	<-w.catacomb.Dying()
	w.mu.Lock()
	w.status = "dying"
	w.mu.Unlock()

	w.logger.Infof(ctx, "shutting down HTTP server on %q", w.listener.Addr())

	defer func() {
		if err := w.listener.Close(); err != nil {
			w.logger.Errorf(ctx, "error closing listener: %v", err)
		}
	}()

	shutDownCtx, cancel := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutDownCtx); err != nil {
		return errors.Annotatef(err, "shutting down HTTP server")
	}

	return w.shutdown()
}

func (w *Worker) shutdown() error {
	muxDone := make(chan struct{})
	go func() {
		// HTTP Server is shutting down, wait for the mux clients (aka the API
		// server) to have terminated.
		w.config.Mux.Wait()
		close(muxDone)
	}()

	select {
	case <-muxDone:
	case <-w.config.Clock.After(w.config.MuxShutdownWait):
		w.logger.Warningf(context.Background(), "timeout waiting for apiserver shutdown")
	}

	return w.catacomb.ErrDying()
}

func (w *Worker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *Worker) newSimpleListener() (*simpleListener, error) {
	listenAddr := net.JoinHostPort("", strconv.Itoa(w.config.APIPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	w.logger.Infof(context.Background(), "listening on %q", listener.Addr())
	return &simpleListener{Listener: listener}, nil
}

type simpleListener struct {
	net.Listener

	mu     sync.Mutex
	closed bool
}

func (s *simpleListener) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ""
	}

	return fmt.Sprintf("https://%s", s.Addr())
}

func (s *simpleListener) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	return s.Listener.Close()
}
