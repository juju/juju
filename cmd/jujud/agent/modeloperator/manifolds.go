// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeloperator

import (
	"github.com/juju/loggo"
	"github.com/juju/utils/v2/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/apiservercertwatcher"
	"github.com/juju/juju/worker/caasadmission"
	"github.com/juju/juju/worker/caasbroker"
	"github.com/juju/juju/worker/caasrbacmapper"
	"github.com/juju/juju/worker/caasupgrader"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/muxhttpserver"
)

type ManifoldConfig struct {
	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the unit agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// NewContainerBrokerFunc is a function opens a CAAS provider.
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
	Port                   string
	ServiceName            string
	ServiceNamespace       string

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// PreviousAgentVersion passes through the version the unit
	// agent was running before the current restart.
	PreviousAgentVersion version.Number

	// UpgradeStepsLock is passed to the upgrade steps gate to
	// coordinate workers that shouldn't do anything until the
	// upgrade-steps worker is done.
	UpgradeStepsLock gate.Lock
}

// Manifolds return a set of co-configured manifolds covering the various
// responsibilities of a model operator agent.
func Manifolds(config ManifoldConfig) dependency.Manifolds {
	return dependency.Manifolds{
		agentName: agent.Manifold(config.Agent),

		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             loggo.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIOpen:              api.Open,
			APIConfigWatcherName: apiConfigWatcherName,
			NewConnection:        apicaller.OnlyConnect,
			Logger:               loggo.GetLogger("juju.worker.apicaller"),
		}),

		caasAdmissionName: caasadmission.Manifold(caasadmission.ManifoldConfig{
			AgentName:        agentName,
			AuthorityName:    certificateWatcherName,
			BrokerName:       caasBrokerTrackerName,
			Logger:           loggo.GetLogger("juju.worker.caasadmission"),
			MuxName:          modelHTTPServerName,
			ServerInfoName:   modelHTTPServerName,
			RBACMapperName:   caasRBACMapperName,
			ServiceName:      config.ServiceName,
			ServiceNamespace: config.ServiceNamespace,
		}),

		caasBrokerTrackerName: caasbroker.Manifold(caasbroker.ManifoldConfig{
			APICallerName:          apiCallerName,
			NewContainerBrokerFunc: config.NewContainerBrokerFunc,
			Logger:                 loggo.GetLogger("juju.worker.caas"),
		}),

		caasRBACMapperName: caasrbacmapper.Manifold(
			caasrbacmapper.ManifoldConfig{
				BrokerName: caasBrokerTrackerName,
				Logger:     loggo.GetLogger("juju.worker.caasrbacmapper"),
			},
		),

		certificateWatcherName: apiservercertwatcher.Manifold(apiservercertwatcher.ManifoldConfig{
			AgentName:           agentName,
			CertWatcherWorkerFn: apiservercertwatcher.NewAuthorityWorker,
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender or rsyslog,
		// according to changes in model config. We should only need
		// one of these in a consolidated agent.
		loggingConfigUpdaterName: logger.Manifold(logger.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggingContext:  loggo.DefaultContext(),
			Logger:          loggo.GetLogger("juju.worker.loggerconfig"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		}),

		modelHTTPServerName: muxhttpserver.Manifold(
			muxhttpserver.ManifoldConfig{
				AuthorityName: certificateWatcherName,
				Logger:        loggo.GetLogger("juju.worker.muxhttpserver"),
				Port:          config.Port,
			},
		),

		upgraderName: caasupgrader.Manifold(caasupgrader.ManifoldConfig{
			AgentName:            agentName,
			APICallerName:        apiCallerName,
			UpgradeStepsGateName: upgradeStepsGateName,
			PreviousAgentVersion: config.PreviousAgentVersion,
		}),

		upgradeStepsGateName: gate.ManifoldEx(config.UpgradeStepsLock),
	}
}

const (
	agentName                = "agent"
	apiCallerName            = "api-caller"
	apiConfigWatcherName     = "api-config-watcher"
	caasAdmissionName        = "caas-admission"
	caasBrokerTrackerName    = "caas-broker-tracker"
	caasRBACMapperName       = "caas-rbac-mapper"
	certificateWatcherName   = "certificate-watcher"
	loggingConfigUpdaterName = "logging-config-updater"
	modelHTTPServerName      = "model-http-server"
	upgraderName             = "upgrader"
	upgradeStepsGateName     = "upgrade-steps-gate"
)
