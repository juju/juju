// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/juju/api"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/voyeur"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/applicationscaler"
	"github.com/juju/juju/worker/charmrevision"
	"github.com/juju/juju/worker/charmrevision/charmrevisionmanifold"
	"github.com/juju/juju/worker/cleaner"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/discoverspaces"
	"github.com/juju/juju/worker/environ"
	"github.com/juju/juju/worker/firewaller"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/lifeflag"
	"github.com/juju/juju/worker/machineundertaker"
	"github.com/juju/juju/worker/metricworker"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/singular"
	"github.com/juju/juju/worker/statushistorypruner"
	"github.com/juju/juju/worker/storageprovisioner"
	"github.com/juju/juju/worker/undertaker"
	"github.com/juju/juju/worker/unitassigner"
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

	// AgentConfigChanged will be set whenever the agent's api config
	// is updated
	AgentConfigChanged *voyeur.Value

	// Clock supplies timing services to any manifolds that need them.
	// Only a few workers have been converted to use them fo far.
	Clock clock.Clock

	// InstPollerAggregationDelay is the delay before sending a batch of
	// requests in the instancpoller.Worker's aggregate loop.
	InstPollerAggregationDelay time.Duration

	// RunFlagDuration defines for how long this controller will ask
	// for model administration rights; most of the workers controlled
	// by this agent will only be started when the run flag is known
	// to be held.
	RunFlagDuration time.Duration

	// CharmRevisionUpdateInterval determines how often the charm-
	// revision worker will check for new revisions of known charms.
	CharmRevisionUpdateInterval time.Duration

	// StatusHistoryPruner* values control status-history pruning
	// behaviour.
	StatusHistoryPrunerMaxHistoryTime time.Duration
	StatusHistoryPrunerMaxHistoryMB   uint
	StatusHistoryPrunerInterval       time.Duration

	// SpacesImportedGate will be unlocked when spaces are known to
	// have been imported.
	SpacesImportedGate gate.Lock

	// NewEnvironFunc is a function opens a provider "environment"
	// (typically environs.New).
	NewEnvironFunc environs.NewEnvironFunc

	// NewMigrationMaster is called to create a new migrationmaster
	// worker.
	NewMigrationMaster func(migrationmaster.Config) (worker.Worker, error)
}

// Manifolds returns a set of interdependent dependency manifolds that will
// run together to administer a model, as configured.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {
	modelTag := config.Agent.CurrentConfig().Model()
	return dependency.Manifolds{

		// The first group are foundational; the agent and clock
		// which wrap those supplied in config, and the api-caller
		// through which everything else communicates with the
		// controller.
		agentName: agent.Manifold(config.Agent),
		clockName: clockManifold(config.Clock),
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:     agentName,
			APIOpen:       api.Open,
			NewConnection: apicaller.OnlyConnect,
			Filter:        apiConnectFilter,
		}),

		// The spaces-imported gate will be unlocked when space
		// discovery is known to be complete. Various manifolds
		// should also come to depend upon it (or rather, on a
		// Flag depending on it) in the future.
		spacesImportedGateName: gate.ManifoldEx(config.SpacesImportedGate),

		// All other manifolds should depend on at least one of these
		// three, which handle all the tasks that are safe and sane
		// to run in *all* controller machines.
		notDeadFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Entity:        modelTag,
			Result:        life.IsNotDead,
			Filter:        LifeFilter,

			NewFacade: lifeflag.NewFacade,
			NewWorker: lifeflag.NewWorker,
		}),
		notAliveFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Entity:        modelTag,
			Result:        life.IsNotAlive,
			Filter:        LifeFilter,

			NewFacade: lifeflag.NewFacade,
			NewWorker: lifeflag.NewWorker,
		}),
		isResponsibleFlagName: singular.Manifold(singular.ManifoldConfig{
			ClockName:     clockName,
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Duration:      config.RunFlagDuration,

			NewFacade: singular.NewFacade,
			NewWorker: singular.NewWorker,
		}),

		// The migration workers collaborate to run migrations;
		// and to create a mechanism for running other workers
		// so they can't accidentally interfere with a migration
		// in progress. Such a manifold should (1) depend on the
		// migration-inactive flag, to know when to start or die;
		// and (2) occupy the migration-fortress, so as to avoid
		// possible interference with the minion (which will not
		// take action until it's gained sole control of the
		// fortress).
		//
		// Note that the fortress and flag will only exist while
		// the model is not dead; this frees their dependencies
		// from model-lifetime concerns.
		migrationFortressName: ifNotDead(fortress.Manifold()),
		migrationInactiveFlagName: ifNotDead(migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
		})),
		migrationMasterName: ifNotDead(migrationmaster.Manifold(migrationmaster.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			FortressName:  migrationFortressName,
			Clock:         config.Clock,
			NewFacade:     migrationmaster.NewFacade,
			NewWorker:     config.NewMigrationMaster,
		})),

		// Everything else should be wrapped in ifResponsible,
		// ifNotAlive, ifNotDead, or ifNotMigrating (which also
		// implies NotDead), to ensure that only a single
		// controller is attempting to administer this model at
		// any one time.
		//
		// NOTE: not perfectly reliable at this stage? i.e. a
		// worker that ignores its stop signal for "too long"
		// might continue to take admin actions after the window
		// of responsibility closes. This *is* a pre-existing
		// problem, but demands some thought/care: e.g. should
		// we make sure the apiserver also closes any
		// connections that lose responsibility..? can we make
		// sure all possible environ operations are either time-
		// bounded or interruptible? etc
		//
		// On the other hand, all workers *should* be written in
		// the expectation of dealing with sucky infrastructure
		// running things in parallel unexpectedly, just because
		// the universe hates us and will engineer matters such
		// that it happens sometimes, even when we try to avoid
		// it.

		// The environ tracker could/should be used by several other
		// workers (firewaller, provisioners, address-cleaner?).
		environTrackerName: ifResponsible(environ.Manifold(environ.ManifoldConfig{
			APICallerName:  apiCallerName,
			NewEnvironFunc: config.NewEnvironFunc,
		})),

		// The undertaker is currently the only ifNotAlive worker.
		undertakerName: ifNotAlive(undertaker.Manifold(undertaker.ManifoldConfig{
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,

			NewFacade: undertaker.NewFacade,
			NewWorker: undertaker.NewWorker,
		})),

		// All the rest depend on ifNotMigrating.
		spaceImporterName: ifNotMigrating(discoverspaces.Manifold(discoverspaces.ManifoldConfig{
			EnvironName:   environTrackerName,
			APICallerName: apiCallerName,
			UnlockerName:  spacesImportedGateName,

			NewFacade: discoverspaces.NewFacade,
			NewWorker: discoverspaces.NewWorker,
		})),
		computeProvisionerName: ifNotMigrating(provisioner.Manifold(provisioner.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			EnvironName:        environTrackerName,
			NewProvisionerFunc: provisioner.NewEnvironProvisioner,
		})),
		storageProvisionerName: ifNotMigrating(storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
			APICallerName: apiCallerName,
			ClockName:     clockName,
			EnvironName:   environTrackerName,
			Scope:         modelTag,
		})),
		firewallerName: ifNotMigrating(firewaller.Manifold(firewaller.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		unitAssignerName: ifNotMigrating(unitassigner.Manifold(unitassigner.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		applicationScalerName: ifNotMigrating(applicationscaler.Manifold(applicationscaler.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     applicationscaler.NewFacade,
			NewWorker:     applicationscaler.New,
		})),
		instancePollerName: ifNotMigrating(instancepoller.Manifold(instancepoller.ManifoldConfig{
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,
			ClockName:     clockName,
			Delay:         config.InstPollerAggregationDelay,
		})),
		charmRevisionUpdaterName: ifNotMigrating(charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
			APICallerName: apiCallerName,
			ClockName:     clockName,
			Period:        config.CharmRevisionUpdateInterval,

			NewFacade: charmrevisionmanifold.NewAPIFacade,
			NewWorker: charmrevision.NewWorker,
		})),
		metricWorkerName: ifNotMigrating(metricworker.Manifold(metricworker.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		stateCleanerName: ifNotMigrating(cleaner.Manifold(cleaner.ManifoldConfig{
			APICallerName: apiCallerName,
		})),
		statusHistoryPrunerName: ifNotMigrating(statushistorypruner.Manifold(statushistorypruner.ManifoldConfig{
			APICallerName:  apiCallerName,
			MaxHistoryTime: config.StatusHistoryPrunerMaxHistoryTime,
			MaxHistoryMB:   config.StatusHistoryPrunerMaxHistoryMB,
			PruneInterval:  config.StatusHistoryPrunerInterval,
			// TODO(fwereade): 2016-03-17 lp:1558657
			NewTimer: worker.NewTimer,
		})),
		machineUndertakerName: ifNotMigrating(machineundertaker.Manifold(machineundertaker.ManifoldConfig{
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,
			NewWorker:     machineundertaker.NewWorker,
		})),
	}
}

// clockManifold expresses a Clock as a ValueWorker manifold.
func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ dependency.Context) (worker.Worker, error) {
			return engine.NewValueWorker(clock)
		},
		Output: engine.ValueWorkerOutput,
	}
}

func apiConnectFilter(err error) error {
	// If the model is no longer there, then convert to ErrRemoved so
	// that the dependency engine for the model is stopped.
	// See http://pad.lv/1614809
	if params.IsCodeModelNotFound(err) {
		return ErrRemoved
	}
	return err
}

var (
	// ifResponsible wraps a manifold such that it only runs if the
	// responsibility flag is set.
	ifResponsible = engine.Housing{
		Flags: []string{
			isResponsibleFlagName,
		},
	}.Decorate

	// ifNotAlive wraps a manifold such that it only runs if the
	// responsibility flag is set and the model is Dying or Dead.
	ifNotAlive = engine.Housing{
		Flags: []string{
			isResponsibleFlagName,
			notAliveFlagName,
		},
	}.Decorate

	// ifNotDead wraps a manifold such that it only runs if the
	// responsibility flag is set and the model is Alive or Dying.
	ifNotDead = engine.Housing{
		Flags: []string{
			isResponsibleFlagName,
			notDeadFlagName,
		},
	}.Decorate

	// ifNotMigrating wraps a manifold such that it only runs if the
	// migration-inactive flag is set; and then runs workers only
	// within Visits to the migration fortress. To avoid redundancy,
	// it takes advantage of the fact that those migration manifolds
	// themselves depend on ifNotDead, and eschews repeating those
	// dependencies.
	ifNotMigrating = engine.Housing{
		Flags: []string{
			migrationInactiveFlagName,
		},
		Occupy: migrationFortressName,
	}.Decorate
)

const (
	agentName            = "agent"
	clockName            = "clock"
	apiConfigWatcherName = "api-config-watcher"
	apiCallerName        = "api-caller"

	spacesImportedGateName = "spaces-imported-gate"
	isResponsibleFlagName  = "is-responsible-flag"
	notDeadFlagName        = "not-dead-flag"
	notAliveFlagName       = "not-alive-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMasterName       = "migration-master"

	environTrackerName       = "environ-tracker"
	undertakerName           = "undertaker"
	spaceImporterName        = "space-importer"
	computeProvisionerName   = "compute-provisioner"
	storageProvisionerName   = "storage-provisioner"
	firewallerName           = "firewaller"
	unitAssignerName         = "unit-assigner"
	applicationScalerName    = "application-scaler"
	instancePollerName       = "instance-poller"
	charmRevisionUpdaterName = "charm-revision-updater"
	metricWorkerName         = "metric-worker"
	stateCleanerName         = "state-cleaner"
	statusHistoryPrunerName  = "status-history-pruner"
	machineUndertakerName    = "machine-undertaker"
)
