// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/authentication/macaroon"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coredependency "github.com/juju/juju/core/dependency"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/jwtparser"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/jujuclient"
)

// GetControllerConfigServiceFunc is a helper function that gets a
// [ControllerConfigService] from the manifold.
type GetControllerConfigServiceFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetControllerConfigService is a helper function that gets a
// [ControllerConfigService] from the manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// GetModelServiceFunc is a helper function that gets a [ModelService] from the
// manifold.
type GetModelServiceFunc func(getter dependency.Getter, name string) (ModelService, error)

// GetModelService is a helper function that gets a [ModelService] from the
// manifold.
func GetModelService(getter dependency.Getter, name string) (ModelService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerDomainServices) ModelService {
		return factory.Model()
	})
}

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName              string
	AuthenticatorName      string
	ClockName              string
	MuxName                string
	UpgradeGateName        string
	AuditConfigUpdaterName string
	LeaseManagerName       string
	LogSinkName            string
	HTTPClientName         string

	DBAccessorName     string
	ChangeStreamName   string
	DomainServicesName string
	TraceName          string
	ObjectStoreName    string
	JWTParserName      string

	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	GetControllerConfigService        GetControllerConfigServiceFunc
	GetModelService                   GetModelServiceFunc

	NewWorker           func(context.Context, Config) (worker.Worker, error)
	NewMetricsCollector func() *apiserver.Collector
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.AuthenticatorName == "" {
		return errors.NotValidf("empty AuthenticatorName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.UpgradeGateName == "" {
		return errors.NotValidf("empty UpgradeGateName")
	}
	if config.AuditConfigUpdaterName == "" {
		return errors.NotValidf("empty AuditConfigUpdaterName")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.LogSinkName == "" {
		return errors.NotValidf("empty LogSinkName")
	}
	if config.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if config.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if config.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if config.JWTParserName == "" {
		return errors.NotValidf("empty JWTParserName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewMetricsCollector == nil {
		return errors.NotValidf("nil NewMetricsCollector")
	}
	if config.GetControllerConfigService == nil {
		return errors.NotValidf("nil GetControllerConfigService")
	}
	if config.GetModelService == nil {
		return errors.NotValidf("nil GetModelService")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run an apiserver
// worker. The manifold outputs an *apiserverhttp.Mux, for other workers
// to register handlers against.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.AuthenticatorName,
			config.ClockName,
			config.MuxName,
			config.UpgradeGateName,
			config.AuditConfigUpdaterName,
			config.LeaseManagerName,
			config.HTTPClientName,
			config.DBAccessorName,
			config.ChangeStreamName,
			config.DomainServicesName,
			config.TraceName,
			config.ObjectStoreName,
			config.LogSinkName,
			config.JWTParserName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := getter.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	var macaroonAuthenticator macaroon.LocalMacaroonAuthenticator
	if err := getter.Get(config.AuthenticatorName, &macaroonAuthenticator); err != nil {
		return nil, errors.Trace(err)
	}

	var upgradeLock gate.Waiter
	if err := getter.Get(config.UpgradeGateName, &upgradeLock); err != nil {
		return nil, errors.Trace(err)
	}

	var getAuditConfig func() auditlog.Config
	if err := getter.Get(config.AuditConfigUpdaterName, &getAuditConfig); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	var logSink corelogger.ModelLogger
	if err := getter.Get(config.LogSinkName, &logSink); err != nil {
		return nil, errors.Trace(err)
	}

	var httpClientGetter corehttp.HTTPClientGetter
	if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
		return nil, errors.Trace(err)
	}

	charmhubHTTPClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.CharmhubPurpose)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var dbDeleter database.DBDeleter
	if err := getter.Get(config.DBAccessorName, &dbDeleter); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var tracerGetter trace.TracerGetter
	if err := getter.Get(config.TraceName, &tracerGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var objectStoreGetter objectstore.ObjectStoreGetter
	if err := getter.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfigService, err := config.GetControllerConfigService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelService, err := config.GetModelService(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var jwtParser *jwtparser.Parser
	if err := getter.Get(config.JWTParserName, &jwtParser); err != nil {
		return nil, errors.Trace(err)
	}

	// Register the metrics collector against the prometheus register.
	metricsCollector := config.NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	execEmbeddedCommand := func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusARgs string) int {
		jujuCmd := commands.NewJujuCommandWithStore(ctx, store, nil, "", `Type "help" to see a list of commands`, whitelist, true)
		return cmd.Main(jujuCmd, ctx, strings.Split(cmdPlusARgs, " "))
	}

	w, err := config.NewWorker(ctx, Config{
		AgentConfig:                       agent.CurrentConfig(),
		Clock:                             clock,
		Mux:                               mux,
		LeaseManager:                      leaseManager,
		RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
		UpgradeComplete:                   upgradeLock.IsUnlocked,
		LocalMacaroonAuthenticator:        macaroonAuthenticator,
		JWTParser:                         jwtParser,
		GetAuditConfig:                    getAuditConfig,
		NewServer:                         newServerShim,
		MetricsCollector:                  metricsCollector,
		EmbeddedCommand:                   execEmbeddedCommand,
		LogSink:                           logSink,
		CharmhubHTTPClient:                charmhubHTTPClient,
		DBGetter:                          dbGetter,
		DBDeleter:                         dbDeleter,
		DomainServicesGetter:              domainServicesGetter,
		ControllerConfigService:           controllerConfigService,
		TracerGetter:                      tracerGetter,
		ObjectStoreGetter:                 objectStoreGetter,
		ModelService:                      modelService,
	})
	if err != nil {
		// Ensure we clean up the resources we've registered with. This includes
		// the state pool and the metrics collector.
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)

		return nil, errors.Trace(err)
	}
	mux.AddClient()
	return common.NewCleanupWorker(w, func() {
		mux.ClientDone()

		// Ensure we clean up the resources we've registered with. This includes
		// the state pool and the metrics collector.
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}
