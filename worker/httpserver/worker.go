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

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.httpserver")

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig          agent.Config
	TLSConfig            *tls.Config
	Mux                  *apiserverhttp.Mux
	PrometheusRegisterer prometheus.Registerer
}

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.TLSConfig == nil {
		return errors.NotValidf("nil TLSConfig")
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
		url:    make(chan string),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
	url      chan string
}

func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// URL returns the base URL of the HTTP server of the form
// https://ipaddr:port with no trailing slash.
func (w *Worker) URL() string {
	select {
	case <-w.catacomb.Dying():
		return ""
	case url := <-w.url:
		return url
	}
}

func (w *Worker) loop() error {
	servingInfo, ok := w.config.AgentConfig.StateServingInfo()
	if !ok {
		return errors.New("missing state serving info")
	}
	listenAddr := net.JoinHostPort("", strconv.Itoa(servingInfo.APIPort))
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return errors.Trace(err)
	}
	listener = tls.NewListener(listener, w.config.TLSConfig)
	defer listener.Close()
	// TODO(axw) rate-limit connections by wrapping listener

	logger.Infof("listening on %q", listener.Addr())
	server := &http.Server{
		Handler:   w.config.Mux,
		TLSConfig: w.config.TLSConfig,
		ErrorLog: log.New(&loggoWrapper{
			level:  loggo.WARNING,
			logger: logger,
		}, "", 0), // no prefix and no flags so log.Logger doesn't add extra prefixes
	}
	go server.Serve(listener)
	defer func() {
		// TODO(axw) we need to provide mux clients a means
		// of registering for graceful shutdown, and wait
		// for them also.
		logger.Infof("shutting down HTTP server")
		err := server.Shutdown(context.Background())
		w.catacomb.Kill(err)
	}()

	url := fmt.Sprintf("https://%s", listener.Addr())
	for {
		select {
		case <-w.catacomb.Dying():
			// Asked to shutdown - make sure we wait until all clients
			// have finished up.
			w.config.Mux.Wait()
			return w.catacomb.ErrDying()
		case w.url <- url:
		}
	}
}
