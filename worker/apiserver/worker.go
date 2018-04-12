// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.worker.apiserver")

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig                       agent.Config
	Clock                             clock.Clock
	Hub                               *pubsub.StructuredHub
	Mux                               *apiserverhttp.Mux
	Authenticator                     httpcontext.LocalMacaroonAuthenticator
	StatePool                         *state.StatePool
	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	RestoreStatus                     func() state.RestoreStatus
	UpgradeComplete                   func() bool
	GetAuditConfig                    func() auditlog.Config
	NewServer                         NewServerFunc
}

// NewServerFunc is the type of function that will be used
// by the worker to create a new API server.
type NewServerFunc func(apiserver.ServerConfig) (worker.Worker, error)

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
	if config.StatePool == nil {
		return errors.NotValidf("nil StatePool")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.Authenticator == nil {
		return errors.NotValidf("nil Authenticator")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.RestoreStatus == nil {
		return errors.NotValidf("nil RestoreStatus")
	}
	if config.UpgradeComplete == nil {
		return errors.NotValidf("nil UpgradeComplete")
	}
	if config.NewServer == nil {
		return errors.NotValidf("nil NewServer")
	}
	return nil
}

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	rateLimitConfig, err := getRateLimitConfig(config.AgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting rate limit config")
	}

	logSinkConfig, err := getLogSinkConfig(config.AgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting log sink config")
	}

	controllerConfig, err := config.StatePool.SystemState().ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}

	observerFactory, err := newObserverFn(
		config.AgentConfig,
		controllerConfig,
		config.Clock,
		config.PrometheusRegisterer,
		config.Hub,
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create RPC observer factory")
	}

	serverConfig := apiserver.ServerConfig{
		StatePool:                     config.StatePool,
		Clock:                         config.Clock,
		Tag:                           config.AgentConfig.Tag(),
		DataDir:                       config.AgentConfig.DataDir(),
		LogDir:                        config.AgentConfig.LogDir(),
		Hub:                           config.Hub,
		Mux:                           config.Mux,
		Authenticator:                 config.Authenticator,
		RestoreStatus:                 config.RestoreStatus,
		UpgradeComplete:               config.UpgradeComplete,
		PublicDNSName:                 controllerConfig.AutocertDNSName(),
		AllowModelAccess:              controllerConfig.AllowModelAccess(),
		NewObserver:                   observerFactory,
		RegisterIntrospectionHandlers: config.RegisterIntrospectionHTTPHandlers,
		RateLimitConfig:               rateLimitConfig,
		LogSinkConfig:                 &logSinkConfig,
		PrometheusRegisterer:          config.PrometheusRegisterer,
		GetAuditConfig:                config.GetAuditConfig,
	}
	return config.NewServer(serverConfig)
}

func newServerShim(config apiserver.ServerConfig) (worker.Worker, error) {
	return apiserver.NewServer(config)
}
