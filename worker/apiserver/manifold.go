// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/core/cache"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/gate"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName              string
	AuthenticatorName      string
	ClockName              string
	ModelCacheName         string
	MultiwatcherName       string
	MuxName                string
	RestoreStatusName      string
	StateName              string
	UpgradeGateName        string
	AuditConfigUpdaterName string
	LeaseManagerName       string
	RaftTransportName      string

	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	Hub                               *pubsub.StructuredHub
	Presence                          presence.Recorder

	NewWorker           func(Config) (worker.Worker, error)
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
	if config.ModelCacheName == "" {
		return errors.NotValidf("empty ModelCacheName")
	}
	if config.MultiwatcherName == "" {
		return errors.NotValidf("empty MultiwatcherName")
	}
	if config.MuxName == "" {
		return errors.NotValidf("empty MuxName")
	}
	if config.RestoreStatusName == "" {
		return errors.NotValidf("empty RestoreStatusName")
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
	if config.RaftTransportName == "" {
		return errors.NotValidf("empty RaftTransportName")
	}
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
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
			config.ModelCacheName,
			config.MultiwatcherName,
			config.MuxName,
			config.RestoreStatusName,
			config.StateName,
			config.UpgradeGateName,
			config.AuditConfigUpdaterName,
			config.LeaseManagerName,
			config.RaftTransportName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := context.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var mux *apiserverhttp.Mux
	if err := context.Get(config.MuxName, &mux); err != nil {
		return nil, errors.Trace(err)
	}

	var authenticator httpcontext.LocalMacaroonAuthenticator
	if err := context.Get(config.AuthenticatorName, &authenticator); err != nil {
		return nil, errors.Trace(err)
	}

	var restoreStatus func() state.RestoreStatus
	if err := context.Get(config.RestoreStatusName, &restoreStatus); err != nil {
		return nil, errors.Trace(err)
	}

	var stTracker workerstate.StateTracker
	if err := context.Get(config.StateName, &stTracker); err != nil {
		return nil, errors.Trace(err)
	}

	var factory multiwatcher.Factory
	if err := context.Get(config.MultiwatcherName, &factory); err != nil {
		return nil, errors.Trace(err)
	}

	var controller *cache.Controller
	if err := context.Get(config.ModelCacheName, &controller); err != nil {
		return nil, errors.Trace(err)
	}

	var upgradeLock gate.Waiter
	if err := context.Get(config.UpgradeGateName, &upgradeLock); err != nil {
		return nil, errors.Trace(err)
	}

	var getAuditConfig func() auditlog.Config
	if err := context.Get(config.AuditConfigUpdaterName, &getAuditConfig); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := context.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	// We don't need anything from the raft-transport but we need to
	// tie the lifetime of this worker to it - otherwise http-server
	// will hang waiting for this to release the mux.
	if err := context.Get(config.RaftTransportName, nil); err != nil {
		return nil, errors.Trace(err)
	}

	// Get the state pool after grabbing dependencies so we don't need
	// to remember to call Done on it if they're not running yet.
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Register the metrics collector against the prometheus register.
	metricsCollector := config.NewMetricsCollector()
	if err := config.PrometheusRegisterer.Register(metricsCollector); err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		AgentConfig:                       agent.CurrentConfig(),
		Clock:                             clock,
		Mux:                               mux,
		StatePool:                         statePool,
		Controller:                        controller,
		MultiwatcherFactory:               factory,
		LeaseManager:                      leaseManager,
		RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
		RestoreStatus:                     restoreStatus,
		UpgradeComplete:                   upgradeLock.IsUnlocked,
		Hub:                               config.Hub,
		Presence:                          config.Presence,
		Authenticator:                     authenticator,
		GetAuditConfig:                    getAuditConfig,
		NewServer:                         newServerShim,
		MetricsCollector:                  metricsCollector,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	mux.AddClient()
	return common.NewCleanupWorker(w, func() {
		mux.ClientDone()
		stTracker.Done()

		// clean up the metrics for the worker, so the next time a worker is
		// created we can safely register the metrics again.
		config.PrometheusRegisterer.Unregister(metricsCollector)
	}), nil
}
