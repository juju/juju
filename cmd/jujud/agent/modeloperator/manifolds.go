// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeloperator

import (
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2/dependency"

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
	"github.com/juju/juju/worker/muxhttpserver"
)

type ManifoldConfig struct {
	Agent              coreagent.Agent
	AgentConfigChanged *voyeur.Value

	// NewContainerBrokerFunc is a function opens a CAAS provider.
	NewContainerBrokerFunc caas.NewContainerBrokerFunc
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
			AgentName:      agentName,
			AuthorityName:  certificateWatcherName,
			BrokerName:     caasBrokerTrackerName,
			Logger:         loggo.GetLogger("juju.worker.caasadmission"),
			MuxName:        modelHTTPServerName,
			RBACMapperName: caasRBACMapperName,
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

		modelHTTPServerName: muxhttpserver.Manifold(
			muxhttpserver.ManifoldConfig{
				AuthorityName: certificateWatcherName,
				Logger:        loggo.GetLogger("juju.worker.muxhttpserver"),
			},
		),
	}
}

const (
	agentName              = "agent"
	apiCallerName          = "api-caller"
	apiConfigWatcherName   = "api-config-watcher"
	caasAdmissionName      = "caas-admission"
	caasBrokerTrackerName  = "caas-broker-tracker"
	caasRBACMapperName     = "caas-rbac-mapper"
	certificateWatcherName = "certificate-watcher"
	modelHTTPServerName    = "model-http-server"
)
