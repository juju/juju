// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/pubsub"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig holds the information necessary to run an apiserver
// worker in a dependency.Engine.
type ManifoldConfig struct {
	AgentName              string
	AuthenticatorName      string
	ClockName              string
	MuxName                string
	RestoreStatusName      string
	StateName              string
	UpgradeGateName        string
	AuditConfigUpdaterName string

	PrometheusRegisterer              prometheus.Registerer
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))
	Hub                               *pubsub.StructuredHub

	NewWorker func(Config) (worker.Worker, error)
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
	if config.PrometheusRegisterer == nil {
		return errors.NotValidf("nil PrometheusRegisterer")
	}
	if config.RegisterIntrospectionHTTPHandlers == nil {
		return errors.NotValidf("nil RegisterIntrospectionHTTPHandlers")
	}
	if config.Hub == nil {
		return errors.NotValidf("nil Hub")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
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
			config.RestoreStatusName,
			config.StateName,
			config.UpgradeGateName,
			config.AuditConfigUpdaterName,
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
	var upgradeLock gate.Waiter
	if err := context.Get(config.UpgradeGateName, &upgradeLock); err != nil {
		return nil, errors.Trace(err)
	}

	var getAuditConfig func() auditlog.Config
	if err := context.Get(config.AuditConfigUpdaterName, &getAuditConfig); err != nil {
		return nil, errors.Trace(err)
	}

	// Get the state pool after grabbing dependencies so we don't need
	// to remember to call Done on it if they're not running yet.
	statePool, err := stTracker.Use()
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		AgentConfig:                       agent.CurrentConfig(),
		Clock:                             clock,
		Mux:                               mux,
		StatePool:                         statePool,
		PrometheusRegisterer:              config.PrometheusRegisterer,
		RegisterIntrospectionHTTPHandlers: config.RegisterIntrospectionHTTPHandlers,
		RestoreStatus:                     restoreStatus,
		UpgradeComplete:                   upgradeLock.IsUnlocked,
		Hub:                               config.Hub,
		Authenticator:                     authenticator,
		GetAuditConfig:                    getAuditConfig,
		NewServer:                         newServerShim,
	})
	if err != nil {
		stTracker.Done()
		return nil, errors.Trace(err)
	}
	mux.AddClient()
	return common.NewCleanupWorker(w, func() {
		mux.ClientDone()
		stTracker.Done()
	}), nil
}
