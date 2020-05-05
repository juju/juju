// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	caasfirewallerapi "github.com/juju/juju/api/caasfirewaller"
	caasunitprovisionerapi "github.com/juju/juju/api/caasunitprovisioner"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/worker/actionpruner"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/applicationscaler"
	"github.com/juju/juju/worker/caasadmission"
	"github.com/juju/juju/worker/caasbroker"
	"github.com/juju/juju/worker/caasenvironupgrader"
	"github.com/juju/juju/worker/caasfirewaller"
	"github.com/juju/juju/worker/caasoperatorprovisioner"
	"github.com/juju/juju/worker/caasrbacmapper"
	"github.com/juju/juju/worker/caasunitprovisioner"
	"github.com/juju/juju/worker/charmrevision"
	"github.com/juju/juju/worker/charmrevision/charmrevisionmanifold"
	"github.com/juju/juju/worker/cleaner"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/credentialvalidator"
	"github.com/juju/juju/worker/environ"
	"github.com/juju/juju/worker/firewaller"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancepoller"
	"github.com/juju/juju/worker/lifeflag"
	"github.com/juju/juju/worker/logforwarder"
	"github.com/juju/juju/worker/logforwarder/sinks"
	"github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/machineundertaker"
	"github.com/juju/juju/worker/metricworker"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationmaster"
	"github.com/juju/juju/worker/modelupgrader"
	"github.com/juju/juju/worker/provisioner"
	"github.com/juju/juju/worker/pruner"
	"github.com/juju/juju/worker/remoterelations"
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

	Authority pki.Authority

	// Clock supplies timing services to any manifolds that need them.
	// Only a few workers have been converted to use them fo far.
	Clock clock.Clock

	// LoggingContext holds the model writers so that the loggers
	// for the workers running on behalf of other models get their logs
	// written into the model's logging collection rather than the controller's.
	LoggingContext *loggo.Context

	// HTTP server mux for registering caas admission controllers
	Mux *apiserverhttp.Mux

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
	StatusHistoryPrunerInterval time.Duration

	// ActionPrunerInterval controls the rate at which the action pruner
	// worker is run.
	ActionPrunerInterval time.Duration

	// NewEnvironFunc is a function opens a provider "environment"
	// (typically environs.New).
	NewEnvironFunc environs.NewEnvironFunc

	// NewContainerBrokerFunc is a function opens a CAAS provider.
	NewContainerBrokerFunc caas.NewContainerBrokerFunc

	// NewMigrationMaster is called to create a new migrationmaster
	// worker.
	NewMigrationMaster func(migrationmaster.Config) (worker.Worker, error)
}

// commonManifolds returns a set of interdependent dependency manifolds that will
// run together to administer a model, as configured. These manifolds are used
// by both IAAS and CAAS models.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()
	agentTag := agentConfig.Tag()
	modelTag := agentConfig.Model()
	result := dependency.Manifolds{
		// The first group are foundational; the agent and clock
		// which wrap those supplied in config, and the api-caller
		// through which everything else communicates with the
		// controller.
		agentName: agent.Manifold(config.Agent),
		clockName: clockManifold(config.Clock),
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             config.LoggingContext.GetLogger("juju.worker.apiconfigwatcher"),
		}),
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:     agentName,
			APIOpen:       api.Open,
			NewConnection: apicaller.OnlyConnect,
			Filter:        apiConnectFilter,
			Logger:        config.LoggingContext.GetLogger("juju.worker.apicaller"),
		}),

		// The logging config updater listens for logging config updates
		// for the model and configures the logging context appropriately.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:      agentName,
			APICallerName:  apiCallerName,
			LoggingContext: config.LoggingContext,
			Logger:         config.LoggingContext.GetLogger("juju.worker.logger"),
		})),

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
			// No Logger defined in lifeflag package.
		}),
		notAliveFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Entity:        modelTag,
			Result:        life.IsNotAlive,
			Filter:        LifeFilter,

			NewFacade: lifeflag.NewFacade,
			NewWorker: lifeflag.NewWorker,
			// No Logger defined in lifeflag package.
		}),
		isResponsibleFlagName: singular.Manifold(singular.ManifoldConfig{
			Clock:         config.Clock,
			APICallerName: apiCallerName,
			Duration:      config.RunFlagDuration,
			Claimant:      agentTag,
			Entity:        modelTag,

			NewFacade: singular.NewFacade,
			NewWorker: singular.NewWorker,
			// No Logger defined in singular package.
		}),
		// This flag runs on all models, and
		// indicates if model's cloud credential is valid.
		validCredentialFlagName: credentialvalidator.Manifold(credentialvalidator.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     credentialvalidator.NewFacade,
			NewWorker:     credentialvalidator.NewWorker,
			Logger:        config.LoggingContext.GetLogger("juju.worker.credentialvalidator"),
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
		// the model is not dead, and not upgrading; this frees
		// their dependencies from model-lifetime/upgrade concerns.
		migrationFortressName: ifNotUpgrading(ifNotDead(fortress.Manifold(
		// No Logger defined in fortress package.
		))),
		migrationInactiveFlagName: ifNotUpgrading(ifNotDead(migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
			// No Logger defined in migrationflag package.
		}))),
		migrationMasterName: ifNotUpgrading(ifNotDead(migrationmaster.Manifold(migrationmaster.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			FortressName:  migrationFortressName,
			Clock:         config.Clock,
			NewFacade:     migrationmaster.NewFacade,
			NewWorker:     config.NewMigrationMaster,
			// No Logger defined in migrationmaster package.
		}))),

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

		charmRevisionUpdaterName: ifNotMigrating(charmrevisionmanifold.Manifold(charmrevisionmanifold.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Period:        config.CharmRevisionUpdateInterval,

			NewFacade: charmrevisionmanifold.NewAPIFacade,
			NewWorker: charmrevision.NewWorker,
			// No Logger defined in charmrevision or charmrevisionmanifold package.
		})),
		remoteRelationsName: ifNotMigrating(remoterelations.Manifold(remoterelations.ManifoldConfig{
			AgentName:                agentName,
			APICallerName:            apiCallerName,
			NewControllerConnection:  apicaller.NewExternalControllerConnection,
			NewRemoteRelationsFacade: remoterelations.NewRemoteRelationsFacade,
			NewWorker:                remoterelations.NewWorker,
			Logger:                   config.LoggingContext.GetLogger("juju.worker.remoterelations"),
		})),
		stateCleanerName: ifNotMigrating(cleaner.Manifold(cleaner.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        config.LoggingContext.GetLogger("juju.worker.cleaner"),
		})),
		statusHistoryPrunerName: ifNotMigrating(pruner.Manifold(pruner.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			NewWorker:     statushistorypruner.New,
			NewFacade:     statushistorypruner.NewFacade,
			PruneInterval: config.StatusHistoryPrunerInterval,
			Logger:        config.LoggingContext.GetLogger("juju.worker.pruner.statushistory"),
		})),
		actionPrunerName: ifNotMigrating(pruner.Manifold(pruner.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			NewWorker:     actionpruner.New,
			NewFacade:     actionpruner.NewFacade,
			PruneInterval: config.ActionPrunerInterval,
			Logger:        config.LoggingContext.GetLogger("juju.worker.pruner.action"),
		})),
		logForwarderName: ifNotDead(logforwarder.Manifold(logforwarder.ManifoldConfig{
			APICallerName: apiCallerName,
			Sinks: []logforwarder.LogSinkSpec{{
				Name:   "juju-log-forward",
				OpenFn: sinks.OpenSyslog,
			}},
			Logger: config.LoggingContext.GetLogger("juju.worker.logforwarder"),
		})),
		// The environ upgrader runs on all controller agents, and
		// unlocks the gate when the environ is up-to-date. The
		// environ tracker will be supplied only to the leader,
		// which is the agent that will run the upgrade steps;
		// the other controller agents will wait for it to complete
		// running those steps before allowing logins to the model.
		modelUpgradeGateName: gate.Manifold(),
		modelUpgradedFlagName: gate.FlagManifold(gate.FlagManifoldConfig{
			GateName:  modelUpgradeGateName,
			NewWorker: gate.NewFlagWorker,
			// No Logger defined in gate package.
		}),
	}
	return result
}

// IAASManifolds returns a set of interdependent dependency manifolds that will
// run together to administer an IAAS model, as configured.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()
	controllerTag := agentConfig.Controller()
	modelTag := agentConfig.Model()
	manifolds := dependency.Manifolds{
		environTrackerName: ifCredentialValid(ifResponsible(environ.Manifold(environ.ManifoldConfig{
			APICallerName:  apiCallerName,
			NewEnvironFunc: config.NewEnvironFunc,
			Logger:         config.LoggingContext.GetLogger("juju.worker.environ"),
		}))),

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

		// The undertaker is currently the only ifNotAlive worker.
		undertakerName: ifNotUpgrading(ifNotAlive(undertaker.Manifold(undertaker.ManifoldConfig{
			APICallerName:      apiCallerName,
			CloudDestroyerName: environTrackerName,

			Logger:                       config.LoggingContext.GetLogger("juju.worker.undertaker"),
			NewFacade:                    undertaker.NewFacade,
			NewWorker:                    undertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),

		// All the rest depend on ifNotMigrating.
		computeProvisionerName: ifNotMigrating(ifCredentialValid(provisioner.Manifold(provisioner.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.provisioner"),

			NewProvisionerFunc:           provisioner.NewEnvironProvisioner,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),
		storageProvisionerName: ifNotMigrating(ifCredentialValid(storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.storageprovisioner"),
			StorageRegistryName:          environTrackerName,
			Model:                        modelTag,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			NewWorker:                    storageprovisioner.NewStorageProvisioner,
		}))),
		firewallerName: ifNotMigrating(ifCredentialValid(firewaller.Manifold(firewaller.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.firewaller"),

			NewControllerConnection:      apicaller.NewExternalControllerConnection,
			NewFirewallerWorker:          firewaller.NewWorker,
			NewFirewallerFacade:          firewaller.NewFirewallerFacade,
			NewRemoteRelationsFacade:     firewaller.NewRemoteRelationsFacade,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),
		unitAssignerName: ifNotMigrating(unitassigner.Manifold(unitassigner.ManifoldConfig{
			APICallerName: apiCallerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.unitassigner"),
		})),
		applicationScalerName: ifNotMigrating(applicationscaler.Manifold(applicationscaler.ManifoldConfig{
			APICallerName: apiCallerName,
			NewFacade:     applicationscaler.NewFacade,
			NewWorker:     applicationscaler.New,
			// No Logger defined in applicationscaler package.
		})),
		instancePollerName: ifNotMigrating(ifCredentialValid(instancepoller.Manifold(instancepoller.ManifoldConfig{
			APICallerName:                apiCallerName,
			EnvironName:                  environTrackerName,
			ClockName:                    clockName,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.instancepoller"),
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),
		metricWorkerName: ifNotMigrating(metricworker.Manifold(metricworker.ManifoldConfig{
			APICallerName: apiCallerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.metricworker"),
		})),
		machineUndertakerName: ifNotMigrating(ifCredentialValid(machineundertaker.Manifold(machineundertaker.ManifoldConfig{
			APICallerName:                apiCallerName,
			EnvironName:                  environTrackerName,
			NewWorker:                    machineundertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.machineundertaker"),
		}))),
		modelUpgraderName: ifNotDead(ifCredentialValid(modelupgrader.Manifold(modelupgrader.ManifoldConfig{
			APICallerName:                apiCallerName,
			EnvironName:                  environTrackerName,
			GateName:                     modelUpgradeGateName,
			ControllerTag:                controllerTag,
			ModelTag:                     modelTag,
			NewFacade:                    modelupgrader.NewFacade,
			NewWorker:                    modelupgrader.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.modelupgrader"),
		}))),
		instanceMutaterName: ifNotMigrating(instancemutater.ModelManifold(instancemutater.ModelManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			EnvironName:   environTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.instancemutater"),
			NewClient:     instancemutater.NewClient,
			NewWorker:     instancemutater.NewEnvironWorker,
		})),
	}

	result := commonManifolds(config)
	for name, manifold := range manifolds {
		result[name] = manifold
	}
	return result
}

// CAASManifolds returns a set of interdependent dependency manifolds that will
// run together to administer a CAAS model, as configured.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()
	modelTag := agentConfig.Model()
	manifolds := dependency.Manifolds{
		// The undertaker is currently the only ifNotAlive worker.
		undertakerName: ifNotUpgrading(ifNotAlive(undertaker.Manifold(undertaker.ManifoldConfig{
			APICallerName:                apiCallerName,
			CloudDestroyerName:           caasBrokerTrackerName,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.undertaker"),
			NewFacade:                    undertaker.NewFacade,
			NewWorker:                    undertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		}))),

		caasBrokerTrackerName: ifResponsible(caasbroker.Manifold(caasbroker.ManifoldConfig{
			APICallerName:          apiCallerName,
			NewContainerBrokerFunc: config.NewContainerBrokerFunc,
			Logger:                 config.LoggingContext.GetLogger("juju.worker.caas"),
		})),

		caasAdmissionName: ifResponsible(caasadmission.Manifold(caasadmission.ManifoldConfig{
			AgentName:      agentName,
			APICallerName:  apiCallerName,
			Authority:      config.Authority,
			BrokerName:     caasBrokerTrackerName,
			Logger:         loggo.GetLogger("juju.worker.caasadmission"),
			Mux:            config.Mux,
			RBACMapperName: caasRBACMapperName,
		})),

		caasFirewallerName: ifNotMigrating(caasfirewaller.Manifold(
			caasfirewaller.ManifoldConfig{
				APICallerName:  apiCallerName,
				BrokerName:     caasBrokerTrackerName,
				ControllerUUID: agentConfig.Controller().Id(),
				ModelUUID:      agentConfig.Model().Id(),
				NewClient: func(caller base.APICaller) caasfirewaller.Client {
					return caasfirewallerapi.NewClient(caller)
				},
				NewWorker: caasfirewaller.NewWorker,
				Logger:    config.LoggingContext.GetLogger("juju.worker.caasfirewaller"),
			},
		)),
		caasOperatorProvisionerName: ifNotMigrating(caasoperatorprovisioner.Manifold(
			caasoperatorprovisioner.ManifoldConfig{
				AgentName:     agentName,
				APICallerName: apiCallerName,
				BrokerName:    caasBrokerTrackerName,
				ClockName:     clockName,
				NewWorker:     caasoperatorprovisioner.NewProvisionerWorker,
				Logger:        config.LoggingContext.GetLogger("juju.worker.caasprovisioner"),
			},
		)),

		caasRBACMapperName: ifResponsible(caasrbacmapper.Manifold(
			caasrbacmapper.ManifoldConfig{
				BrokerName: caasBrokerTrackerName,
				Logger:     loggo.GetLogger("juju.worker.caasrbacmapper"),
			},
		)),

		caasUnitProvisionerName: ifNotMigrating(caasunitprovisioner.Manifold(
			caasunitprovisioner.ManifoldConfig{
				APICallerName: apiCallerName,
				BrokerName:    caasBrokerTrackerName,
				NewClient: func(caller base.APICaller) caasunitprovisioner.Client {
					return caasunitprovisionerapi.NewClient(caller)
				},
				NewWorker: caasunitprovisioner.NewWorker,
				Logger:    config.LoggingContext.GetLogger("juju.worker.caasunitprovisioner"),
			},
		)),
		modelUpgraderName: caasenvironupgrader.Manifold(caasenvironupgrader.ManifoldConfig{
			APICallerName: apiCallerName,
			GateName:      modelUpgradeGateName,
			ModelTag:      modelTag,
			NewFacade:     caasenvironupgrader.NewFacade,
			NewWorker:     caasenvironupgrader.NewWorker,
			// No Logger defined in caasenvironupgrader package.
		}),
		caasStorageProvisionerName: ifNotMigrating(ifCredentialValid(storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.storageprovisioner"),
			StorageRegistryName:          caasBrokerTrackerName,
			Model:                        modelTag,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			NewWorker:                    storageprovisioner.NewCaasWorker,
		}))),
	}
	result := commonManifolds(config)
	for name, manifold := range manifolds {
		result[name] = manifold
	}
	return result
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

	// ifNotUpgrading wraps a manifold such that it only runs after
	// the environ upgrade worker has completed.
	ifNotUpgrading = engine.Housing{
		Flags: []string{
			modelUpgradedFlagName,
		},
	}.Decorate

	// ifCredentialValid wraps a manifold such that it only runs if
	// the model has a valid credential.
	ifCredentialValid = engine.Housing{
		Flags: []string{
			validCredentialFlagName,
		},
	}.Decorate
)

const (
	agentName            = "agent"
	clockName            = "clock"
	apiConfigWatcherName = "api-config-watcher"
	apiCallerName        = "api-caller"

	isResponsibleFlagName = "is-responsible-flag"
	notDeadFlagName       = "not-dead-flag"
	notAliveFlagName      = "not-alive-flag"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMasterName       = "migration-master"

	modelUpgradeGateName  = "model-upgrade-gate"
	modelUpgradedFlagName = "model-upgraded-flag"
	modelUpgraderName     = "model-upgrader"

	environTrackerName       = "environ-tracker"
	undertakerName           = "undertaker"
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
	actionPrunerName         = "action-pruner"
	machineUndertakerName    = "machine-undertaker"
	remoteRelationsName      = "remote-relations"
	logForwarderName         = "log-forwarder"
	loggingConfigUpdaterName = "logging-config-updater"
	instanceMutaterName      = "instance-mutater"

	caasAdmissionName           = "caas-admission"
	caasFirewallerName          = "caas-firewaller"
	caasOperatorProvisionerName = "caas-operator-provisioner"
	caasRBACMapperName          = "caas-rbac-mapper"
	caasUnitProvisionerName     = "caas-unit-provisioner"
	caasStorageProvisionerName  = "caas-storage-provisioner"
	caasBrokerTrackerName       = "caas-broker-tracker"

	validCredentialFlagName = "valid-credential-flag"
)
