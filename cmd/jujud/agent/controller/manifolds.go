// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controller provides the manifolds for the jujud controller
// agent. This package is populated in Phase 2 of the step-3
// implementation. For now it provides a stub that returns an empty
// manifold set so that the controller agent can compile.
package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/flightrecorder"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/internal/upgradesteps"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/gate"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// ManifoldsConfig allows specialisation of the result of
// ControllerManifolds.
type ManifoldsConfig struct {

	// AgentName is the name of the controller agent, like
	// "controller-0". This will never change during the execution of
	// an agent, and is used to provide this as config into a worker
	// rather than making the worker get it from the agent worker
	// itself.
	AgentName string

	// Agent contains the agent that will be wrapped and made available
	// to its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the controller agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// RootDir is the root directory that any worker that needs to
	// access local filesystems should use as a base. In actual use it
	// will be "" but it may be overridden in tests.
	RootDir string

	// PreviousAgentVersion passes through the version the controller
	// agent was running before the current restart.
	PreviousAgentVersion semversion.Number

	// BootstrapLock is passed to the bootstrap gate to coordinate
	// workers that should not do anything until the bootstrap worker
	// is done.
	BootstrapLock gate.Lock

	// UpgradeDBLock is passed to the upgrade database gate to
	// coordinate workers that should not do anything until the
	// upgrade-database worker is done.
	UpgradeDBLock gate.Lock

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that should not do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock

	// UpgradeCheckLock is passed to the upgrade check gate to
	// coordinate workers that should not do anything until the
	// upgrader worker completes its first check.
	UpgradeCheckLock gate.Lock

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// PreUpgradeSteps is a function that is used by the upgradesteps
	// worker to ensure that conditions are OK for an upgrade to
	// proceed.
	PreUpgradeSteps func(model.ModelType) upgrades.PreUpgradeStepsFunc

	// UpgradeSteps is a function that is used by the upgradesteps
	// worker to perform the upgrade steps.
	UpgradeSteps upgrades.UpgradeStepsFunc

	// LogSink defines an interface for writing log records to a log
	// sink.
	LogSink corelogger.LogSink

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// FlightRecorder is used to record significant events.
	FlightRecorder flightrecorder.FlightRecorderWorker

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(context.Context, base.APICaller) error

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// NewAgentStatusSetter provides upgradesteps.StatusSetter.
	NewAgentStatusSetter func(context.Context, base.APICaller) (upgradesteps.StatusSetter, error)

	// ControllerLeaseDuration defines for how long this agent will ask
	// for controller administration rights.
	ControllerLeaseDuration time.Duration

	// TransactionPruneInterval defines how frequently mgo/txn
	// transactions are pruned from the database.
	TransactionPruneInterval time.Duration

	// RegisterIntrospectionHTTPHandlers is a function that calls the
	// supplied function to register introspection HTTP handlers. The
	// function will be passed a path and a handler; the function may
	// alter the path as it sees fit, e.g. by adding a prefix.
	RegisterIntrospectionHTTPHandlers func(func(path string, _ http.Handler))

	// NewModelWorker returns a new worker for managing the model with
	// the specified UUID and type.
	NewModelWorker modelworkermanager.NewModelWorkerFunc

	// MuxShutdownWait is the maximum time the http-server worker will
	// wait for all mux clients to gracefully terminate before the
	// http-worker exits regardless.
	MuxShutdownWait time.Duration

	// SetupLogging is used to initialize the logging context for model
	// workers.
	SetupLogging func(corelogger.LoggerContext, coreagent.Config)

	// DependencyEngineMetrics creates a set of metrics for a model, so
	// it is possible to know the lifecycle of the workers in the
	// dependency engine.
	DependencyEngineMetrics modelworkermanager.ModelMetrics

	// NewEnvironFunc is a function that opens a provider
	// "environment" (typically environs.New).
	NewEnvironFunc func(context.Context, environs.OpenParams, environs.CredentialInvalidator) (environs.Environ, error)
}

// ControllerManifolds returns a set of co-configured manifolds for the
// controller agent. This is a stub that will be populated in Phase 2.
func ControllerManifolds(_ ManifoldsConfig) dependency.Manifolds {
	return dependency.Manifolds{}
}
