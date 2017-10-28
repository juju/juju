// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"net"
	"net/http"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/statemetrics"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.apiserver")

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig                       agent.Config
	Clock                             clock.Clock
	Hub                               *pubsub.StructuredHub
	State                             *state.State
	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	SetStatePool                      func(*state.StatePool)
	LoginValidator                    LoginValidator
	GetCertificate                    func() *tls.Certificate
	StoreAuditEntry                   StoreAuditEntryFunc
	NewServer                         NewServerFunc
}

// NewServerFunc is the type of function that will be used
// by the worker to create a new API server.
//
// NOTE(axw) when we manage a StatePool in a manifold, we
// can get rid of the worker in this package and return
// the juju/apiserver worker directly from the manifold.
type NewServerFunc func(*state.StatePool, net.Listener, apiserver.ServerConfig) (worker.Worker, error)

// Validate validates the API server configuration.
func (config Config) Validate() error {
	if config.AgentConfig == nil {
		return errors.NotValidf("nil AgentConfig")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.State == nil {
		return errors.NotValidf("nil State")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.SetStatePool == nil {
		return errors.NotValidf("nil SetStatePool")
	}
	if config.LoginValidator == nil {
		return errors.NotValidf("nil LoginValidator")
	}
	if config.GetCertificate == nil {
		return errors.NotValidf("nil GetCertificate")
	}
	if config.StoreAuditEntry == nil {
		return errors.NotValidf("nil StoreAuditEntry")
	}
	if config.NewServer == nil {
		return errors.NotValidf("nil NewServer")
	}
	return nil
}

type LoginValidator apiserver.LoginValidator

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	servingInfo, ok := config.AgentConfig.StateServingInfo()
	if !ok {
		return nil, errors.New("missing state serving info")
	}
	listenAddr := net.JoinHostPort("", strconv.Itoa(servingInfo.APIPort))

	rateLimitConfig, err := getRateLimitConfig(config.AgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting rate limit config")
	}

	logSinkConfig, err := getLogSinkConfig(config.AgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting log sink config")
	}

	controllerConfig, err := config.State.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}

	logDir := config.AgentConfig.LogDir()
	observerFactory, err := newObserverFn(
		config.AgentConfig,
		controllerConfig,
		config.Clock,
		newAuditEntrySink(config.StoreAuditEntry, logDir),
		config.PrometheusRegisterer,
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create RPC observer factory")
	}

	serverConfig := apiserver.ServerConfig{
		Clock:                         config.Clock,
		Tag:                           config.AgentConfig.Tag(),
		DataDir:                       config.AgentConfig.DataDir(),
		LogDir:                        logDir,
		Validator:                     apiserver.LoginValidator(config.LoginValidator),
		Hub:                           config.Hub,
		GetCertificate:                config.GetCertificate,
		AutocertURL:                   controllerConfig.AutocertURL(),
		AutocertDNSName:               controllerConfig.AutocertDNSName(),
		AllowModelAccess:              controllerConfig.AllowModelAccess(),
		NewObserver:                   observerFactory,
		RegisterIntrospectionHandlers: config.RegisterIntrospectionHTTPHandlers,
		RateLimitConfig:               rateLimitConfig,
		LogSinkConfig:                 &logSinkConfig,
		PrometheusRegisterer:          config.PrometheusRegisterer,
	}

	w := apiserverWorker{
		st:           config.State,
		listenAddr:   listenAddr,
		setStatePool: config.SetStatePool,
		config:       serverConfig,
		newServer:    config.NewServer,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return &w, nil
}

// apiserverWorker is a worker.Worker that wraps an
// apiserver.Server, and the resources it requires.
type apiserverWorker struct {
	catacomb     catacomb.Catacomb
	st           *state.State
	listenAddr   string
	setStatePool func(*state.StatePool)
	config       apiserver.ServerConfig
	newServer    NewServerFunc
}

// Kill is part of the worker.Worker interface.
func (w *apiserverWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Wait interface.
func (w *apiserverWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *apiserverWorker) loop() error {
	// TODO(axw) the API server should be accepting a StatePool
	// as a manifold input, and the manifold that accepts the
	// state pool should be un/registering the state pool with
	// the Prometheus registry.
	statePool := state.NewStatePool(w.st)
	defer statePool.Close()
	w.setStatePool(statePool)
	defer w.setStatePool(nil)
	collector := statemetrics.New(statemetrics.NewStatePool(statePool))
	w.config.PrometheusRegisterer.Register(collector)
	defer w.config.PrometheusRegisterer.Unregister(collector)

	listener, err := net.Listen("tcp", w.listenAddr)
	if err != nil {
		return errors.Trace(err)
	}
	server, err := w.newServer(statePool, listener, w.config)
	if err != nil {
		listener.Close()
		return errors.Trace(err)
	}
	w.catacomb.Add(server)

	// Wait for server to finish before returning,
	// so we don't tear out the server pool from
	// underneath them.
	server.Wait()
	return w.catacomb.ErrDying()
}

func newServerShim(
	statePool *state.StatePool,
	listener net.Listener,
	config apiserver.ServerConfig,
) (worker.Worker, error) {
	return apiserver.NewServer(statePool, listener, config)
}
