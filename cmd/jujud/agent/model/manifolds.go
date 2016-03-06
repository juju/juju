// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/utils/clock"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/addresser"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/charmrevision"
	"github.com/juju/juju/worker/charmrevision/charmrevisionmanifold"
	"github.com/juju/juju/worker/cleaner"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/discoverspaces"
	"github.com/juju/juju/worker/environ"
	"github.com/juju/juju/worker/firewaller"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/metricworker"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/servicescaler"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/statushistorypruner"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/unitassigner"
	"github.com/juju/juju/worker/util"
)

// ManifoldsConfig holds the dependencies and configuration options for a
// model agent: that is, for the set of interdependent workers that observe
// and manipulate a single model.
type ManifoldsConfig struct {

	// Agent identifies, and exposes configuration for, the controller
	// machine running these manifolds and the model the manifolds
	// should administer.
	//
	// You should almost certainly set this value to one created by
	// model.WrapAgent.
	Agent coreagent.Agent

	// Clock supplies timing services to any manifolds that need them.
	// Only a few workers have been converted to use them fo far.
	Clock clock.Clock

	// RunFlagDuration defines for how long this controller will ask
	// for model administration rights; most of the workers controlled
	// by this agent will only be started when the run flag is known
	// to be held.
	RunFlagDuration time.Duration

	// CharmRevisionUpdateInterval determines how often the charm-
	// revision worker will check for new revisions of known charms.
	CharmRevisionUpdateInterval time.Duration

	// EntityStatusHistory* values control status-history pruning
	// behaviour per entity.
	EntityStatusHistoryCount    uint
	EntityStatusHistoryInterval time.Duration
}

// Manifolds returns a set of interdependent dependency manifolds that will
// run together to administer a model, as configured.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {
	modelTag := config.Agent.CurrentConfig().Model()
	return dependency.Manifolds{

		// The first group are somewhat special; the agent and clock
		// which wrap those supplied in config, and the api-caller
		// and run-flag on which pretty much all the others depend.
		agentName: agent.Manifold(config.Agent),
		clockName: clockManifold(config.Clock),
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:     agentName,
			APIOpen:       apicaller.APIOpen,
			NewConnection: apicaller.OnlyConnect,
		}),
		runFlagName: singular.Manifold(singular.ManifoldConfig{
			ClockName:     clockName,
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Duration:      config.RunFlagDuration,

			NewFacade: singular.NewFacade,
			NewWorker: singular.NewWorker,
		}),
		// Everything else should depend on run-flag, to ensure that
		// only one controller is administering this model at a time.
		//
		// NOTE: not perfectly reliable at this stage? i.e. a worker
		// that ignores its stop signal for "too long" might continue
		// to take admin actions after the window of responsibility
		// closes. This *is* a pre-existing proble,, but demands some
		// thought/care: e.g. should we make sure the apiserver also
		// closes any connections that lose responsibility..? can we
		// make sure all possible environ operations are either time-
		// bounded or interruptible? etc
		//
		// On the other hand, all workers *should* be written in the
		// expectation of dealing with a sucky infrastructure running
		// things in parallel, just because the universe hates us and
		// will engineer matters such that it happens sometimes, even
		// when we try to avoid it.

		// The environ tracker is currently only used by the space
		// discovery worker, but could/should be used by several
		// others (firewaller, provisioners, instance poller).
		environTrackerName: runFlag(environ.Manifold(environ.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		spaceImporterName: runFlag(discoverspaces.Manifold(discoverspaces.ManifoldConfig{
			EnvironName:   environTrackerName,
			APICallerName: apiCallerName,
			// No unlocker name for now; might never be necessary
			// in exactly this form (because we should probably
			// just have a persistent flag set/read via the api).

			NewFacade: discoverspaces.NewFacade,
			NewWorker: discoverspaces.NewWorker,
		})),
		computeProvisionerName: runFlag(provisioner.Manifold(provisioner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
		})),
		storageProvisionerName: runFlag(storageprovisioner.Manifold(storageprovisioner.ManifoldConfig{
			APICallerName: apiCallerName,
			ClockName:     clockName,
			Scope:         modelTag,
		})),
		firewallerName: runFlag(firewaller.Manifold(firewaller.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		unitAssignerName: runFlag(unitassigner.Manifold(unitassigner.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		serviceScalerName: runFlag(servicescaler.Manifold(servicescaler.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     servicescaler.NewFacade,
			NewWorker:     servicescaler.New,
		})),
		instancePollerName: runFlag(instancepoller.Manifold(instancepoller.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		charmRevisionUpdaterName: runFlag(charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
			APICallerName: apiCallerName,
			ClockName:     clockName,
			Period:        config.CharmRevisionUpdateInterval,

			NewFacade: charmrevisionmanifold.NewAPIFacade,
			NewWorker: charmrevision.NewWorker,
		})),
		metricWorkerName: runFlag(metricworker.Manifold(metricworker.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		stateCleanerName: runFlag(cleaner.Manifold(cleaner.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		addressCleanerName: runFlag(addresser.Manifold(addresser.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		statusHistoryPrunerName: runFlag(statushistorypruner.Manifold(statushistorypruner.ManifoldConfig{
			APICallerName:    apiCallerName,
			MaxLogsPerEntity: config.EntityStatusHistoryCount,
			PruneInterval:    config.EntityStatusHistoryInterval,
			// TODO(fwereade): use the clock
			NewTimer: worker.NewTimer,
		})),
	}
}

// runFlag is a compact way of making a manifold depend on the agent's
// run-flag resource, representing the right to administer the model.
func runFlag(manifold dependency.Manifold) dependency.Manifold {
	return dependency.WithFlag(manifold, runFlagName)
}

// clockManifold expresses a Clock as a ValueWorker manifold.
func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			return util.NewValueWorker(clock)
		},
		Output: util.ValueWorkerOutput,
	}
}

const (
	agentName     = "agent"
	clockName     = "clock"
	apiCallerName = "api-caller"
	runFlagName   = "run-flag"

	environTrackerName       = "environ-tracker"
	spaceImporterName        = "space-importer"
	computeProvisionerName   = "compute-provisioner"
	storageProvisionerName   = "storage-provisioner"
	firewallerName           = "firewaller"
	unitAssignerName         = "unit-assigner"
	serviceScalerName        = "service-scaler"
	instancePollerName       = "instance-poller"
	charmRevisionUpdaterName = "charm-revision-updater"
	metricWorkerName         = "metric-worker"
	stateCleanerName         = "state-cleaner"
	addressCleanerName       = "address-cleaner"
	statusHistoryPrunerName  = "status-history-pruner"
)
