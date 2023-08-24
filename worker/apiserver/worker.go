// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/servicefactory"
	"github.com/juju/juju/worker/syslogger"
)

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig                       agent.Config
	Clock                             clock.Clock
	Hub                               *pubsub.StructuredHub
	Presence                          presence.Recorder
	Mux                               *apiserverhttp.Mux
	MultiwatcherFactory               multiwatcher.Factory
	LocalMacaroonAuthenticator        macaroon.LocalMacaroonAuthenticator
	StatePool                         *state.StatePool
	LeaseManager                      lease.Manager
	SysLogger                         syslogger.SysLogger
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	UpgradeComplete                   func() bool
	GetAuditConfig                    func() auditlog.Config
	NewServer                         NewServerFunc
	MetricsCollector                  *apiserver.Collector
	EmbeddedCommand                   apiserver.ExecEmbeddedCommandFunc
	CharmhubHTTPClient                HTTPClient

	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter             changestream.WatchableDBGetter
	ServiceFactoryGetter servicefactory.ServiceFactoryGetter
}

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
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
	if config.Presence == nil {
		return errors.NotValidf("nil Presence")
	}
	if config.StatePool == nil {
		return errors.NotValidf("nil StatePool")
	}
	if config.Mux == nil {
		return errors.NotValidf("nil Mux")
	}
	if config.MultiwatcherFactory == nil {
		return errors.NotValidf("nil MultiwatcherFactory")
	}
	if config.LocalMacaroonAuthenticator == nil {
		return errors.NotValidf("nil LocalMacaroonAuthenticator")
	}
	if config.LeaseManager == nil {
		return errors.NotValidf("nil LeaseManager")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.SysLogger == nil {
		return errors.NotValidf("nil SysLogger")
	}
	if config.UpgradeComplete == nil {
		return errors.NotValidf("nil UpgradeComplete")
	}
	if config.NewServer == nil {
		return errors.NotValidf("nil NewServer")
	}
	if config.MetricsCollector == nil {
		return errors.NotValidf("nil MetricsCollector")
	}
	if config.CharmhubHTTPClient == nil {
		return errors.NotValidf("nil CharmhubHTTPClient")
	}
	if config.ServiceFactoryGetter == nil {
		return errors.NotValidf("nil ServiceFactoryGetter")
	}
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	return nil
}

// NewWorker returns a new API server worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	logSinkConfig, err := getLogSinkConfig(config.AgentConfig)
	if err != nil {
		return nil, errors.Annotate(err, "getting log sink config")
	}

	systemState, err := config.StatePool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerConfig, err := systemState.LegacyControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}

	observerFactory, err := newObserverFn(
		config.AgentConfig,
		controllerConfig,
		config.Clock,
		config.Hub,
		config.MetricsCollector,
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create RPC observer factory")
	}

	jwtAuthenticator, err := gatherJWTAuthenticator(controllerConfig)
	if err != nil {
		return nil, fmt.Errorf("gathering authenticators for apiserver: %w", err)
	}

	serverConfig := apiserver.ServerConfig{
		StatePool:                     config.StatePool,
		Clock:                         config.Clock,
		Tag:                           config.AgentConfig.Tag(),
		DataDir:                       config.AgentConfig.DataDir(),
		LogDir:                        config.AgentConfig.LogDir(),
		Hub:                           config.Hub,
		Presence:                      config.Presence,
		MultiwatcherFactory:           config.MultiwatcherFactory,
		Mux:                           config.Mux,
		LocalMacaroonAuthenticator:    config.LocalMacaroonAuthenticator,
		JWTAuthenticator:              jwtAuthenticator,
		UpgradeComplete:               config.UpgradeComplete,
		PublicDNSName:                 controllerConfig.AutocertDNSName(),
		AllowModelAccess:              controllerConfig.AllowModelAccess(),
		NewObserver:                   observerFactory,
		RegisterIntrospectionHandlers: config.RegisterIntrospectionHTTPHandlers,
		MetricsCollector:              config.MetricsCollector,
		LogSinkConfig:                 &logSinkConfig,
		GetAuditConfig:                config.GetAuditConfig,
		LeaseManager:                  config.LeaseManager,
		ExecEmbeddedCommand:           config.EmbeddedCommand,
		SysLogger:                     config.SysLogger,
		CharmhubHTTPClient:            config.CharmhubHTTPClient,
		DBGetter:                      config.DBGetter,
		ServiceFactoryGetter:          config.ServiceFactoryGetter,
	}
	return config.NewServer(serverConfig)
}

// gatherJWTAuthenticator is responsible for building up the jwt authenticator
// if this controller has been provisioned to trust external jwt tokens.
func gatherJWTAuthenticator(controllerConfig jujucontroller.Config) (jwt.Authenticator, error) {
	jwtRefreshURL := controllerConfig.LoginTokenRefreshURL()
	if jwtRefreshURL == "" {
		return nil, nil
	}
	jwtAuthenticator := jwt.NewAuthenticator(jwtRefreshURL)
	if err := jwtAuthenticator.RegisterJWKSCache(context.Background()); err != nil {
		return nil, err
	}
	return jwtAuthenticator, nil
}

func newServerShim(config apiserver.ServerConfig) (worker.Worker, error) {
	return apiserver.NewServer(config)
}

// NewMetricsCollector returns a new apiserver collector
func NewMetricsCollector() *apiserver.Collector {
	return apiserver.NewMetricsCollector()
}
