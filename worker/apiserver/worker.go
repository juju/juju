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
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.worker.apiserver")

// Config is the configuration required for running an API server worker.
type Config struct {
	AgentConfig                       agent.Config
	Clock                             clock.Clock
	Hub                               *pubsub.StructuredHub
	StatePool                         *state.StatePool
	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	RestoreStatus                     func() state.RestoreStatus
	UpgradeComplete                   func() bool
	GetCertificate                    func() *tls.Certificate
	NewServer                         NewServerFunc
}

// NewServerFunc is the type of function that will be used
// by the worker to create a new API server.
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
	if config.StatePool == nil {
		return errors.NotValidf("nil StatePool")
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
	if config.GetCertificate == nil {
		return errors.NotValidf("nil GetCertificate")
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

	controllerConfig, err := config.StatePool.SystemState().ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "cannot fetch the controller config")
	}

	logDir := config.AgentConfig.LogDir()
	observerFactory, err := newObserverFn(
		config.AgentConfig,
		controllerConfig,
		config.Clock,
		config.PrometheusRegisterer,
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create RPC observer factory")
	}

	auditConfig := getAuditLogConfig(controllerConfig)

	serverConfig := apiserver.ServerConfig{
		Clock:                         config.Clock,
		Tag:                           config.AgentConfig.Tag(),
		DataDir:                       config.AgentConfig.DataDir(),
		LogDir:                        logDir,
		Hub:                           config.Hub,
		GetCertificate:                config.GetCertificate,
		RestoreStatus:                 config.RestoreStatus,
		UpgradeComplete:               config.UpgradeComplete,
		AutocertURL:                   controllerConfig.AutocertURL(),
		AutocertDNSName:               controllerConfig.AutocertDNSName(),
		AllowModelAccess:              controllerConfig.AllowModelAccess(),
		NewObserver:                   observerFactory,
		RegisterIntrospectionHandlers: config.RegisterIntrospectionHTTPHandlers,
		RateLimitConfig:               rateLimitConfig,
		LogSinkConfig:                 &logSinkConfig,
		PrometheusRegisterer:          config.PrometheusRegisterer,
		AuditLogConfig:                auditConfig,
	}
	if auditConfig.Enabled {
		serverConfig.AuditLog = auditlog.NewLogFile(
			logDir, auditConfig.MaxSizeMB, auditConfig.MaxBackups)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	server, err := config.NewServer(config.StatePool, listener, serverConfig)
	if err != nil {
		if err := listener.Close(); err != nil {
			logger.Warningf("failed to close listener: %s", err)
		}
		return nil, errors.Trace(err)
	}
	return server, nil
}

func newServerShim(
	statePool *state.StatePool,
	listener net.Listener,
	config apiserver.ServerConfig,
) (worker.Worker, error) {
	return apiserver.NewServer(statePool, listener, config)
}
