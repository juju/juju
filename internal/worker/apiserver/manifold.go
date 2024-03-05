// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	stdcontext "context"
	"net/http"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/pubsub/v2"
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
	"github.com/juju/juju/core/lease"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/gate"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/internal/worker/trace"
	"github.com/juju/juju/jujuclient"
)

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName              string
	AuthenticatorName      string
	ClockName              string
	MultiwatcherName       string
	MuxName                string
	StateName              string
	UpgradeGateName        string
	AuditConfigUpdaterName string
	LeaseManagerName       string
	LogSinkName            string
	CharmhubHTTPClientName string
	ChangeStreamName       string
	ServiceFactoryName     string
	TraceName              string
	ObjectStoreName        string

	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	Hub                               *pubsub.StructuredHub
	Presence                          presence.Recorder

	NewWorker           func(stdcontext.Context, Config) (worker.Worker, error)
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
	if config.MultiwatcherName == "" {
		return errors.NotValidf("empty MultiwatcherName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.StateName == "" {
		return errors.NotValidf("empty StateName")
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
	if config.CharmhubHTTPClientName == "" {
		return errors.NotValidf("empty CharmhubHTTPClientName")
	}
	if config.ChangeStreamName == "" {
		return errors.NotValidf("empty ChangeStreamName")
	}
	if config.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if config.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if config.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.Presence == nil {
		return errors.NotValidf("nil Presence")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewMetricsCollector == nil {
		return errors.NotValidf("nil NewMetricsCollector")
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
			config.MultiwatcherName,
			config.MuxName,
			config.StateName,
			config.UpgradeGateName,
			config.AuditConfigUpdaterName,
			config.LeaseManagerName,
			config.CharmhubHTTPClientName,
			config.ChangeStreamName,
			config.ServiceFactoryName,
			config.TraceName,
			config.ObjectStoreName,
			config.LogSinkName,
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

	var stTracker workerstate.StateTracker
	if err := getter.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	var factory multiwatcher.Factory
	if err := getter.Get(config.MultiwatcherName, &factory); err != nil {
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

	var charmhubHTTPClient HTTPClient
	if err := getter.Get(config.CharmhubHTTPClientName, &charmhubHTTPClient); err != nil {
		return nil, errors.Trace(err)
	}

	var dbGetter changestream.WatchableDBGetter
	if err := getter.Get(config.ChangeStreamName, &dbGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var serviceFactoryGetter servicefactory.ServiceFactoryGetter
	if err := getter.Get(config.ServiceFactoryName, &serviceFactoryGetter); err != nil {
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

	// Register the metrics collector against the prometheus register.
	metricsCollector := config.NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	execEmbeddedCommand := func(ctx *cmd.Context, store jujuclient.ClientStore, whitelist []string, cmdPlusARgs string) int {
		jujuCmd := commands.NewJujuCommandWithStore(ctx, store, nil, "", `Type "help" to see a list of commands`, whitelist, true)
		return cmd.Main(jujuCmd, ctx, strings.Split(cmdPlusARgs, " "))
	}

	// Get the state pool after grabbing dependencies so we don't need
	// to remember to call Done on it if they're not running yet.
	statePool, _, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(ctx, Config{
		AgentConfig:                       agent.CurrentConfig(),
		Clock:                             clock,
		Mux:                               mux,
		StatePool:                         statePool,
		MultiwatcherFactory:               factory,
		LeaseManager:                      leaseManager,
		RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
		UpgradeComplete:                   upgradeLock.IsUnlocked,
		Hub:                               config.Hub,
		Presence:                          config.Presence,
		LocalMacaroonAuthenticator:        macaroonAuthenticator,
		GetAuditConfig:                    getAuditConfig,
		NewServer:                         newServerShim,
		MetricsCollector:                  metricsCollector,
		EmbeddedCommand:                   execEmbeddedCommand,
		LogSink:                           logSink,
		CharmhubHTTPClient:                charmhubHTTPClient,
		DBGetter:                          dbGetter,
		ServiceFactoryGetter:              serviceFactoryGetter,
		TracerGetter:                      tracerGetter,
		ObjectStoreGetter:                 objectStoreGetter,
	})
	if err != nil {
		// Ensure we clean up the resources we've registered with. This includes
		// the state pool and the metrics collector.
		_ = stTracker.Done()
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)

		return nil, errors.Trace(err)
	}
	mux.AddClient()
	return common.NewCleanupWorker(w, func() {
		mux.ClientDone()

		// Ensure we clean up the resources we've registered with. This includes
		// the state pool and the metrics collector.
		_ = stTracker.Done()
		_ = config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}
