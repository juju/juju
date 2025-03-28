// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	caasfirewallerapi "github.com/juju/juju/api/controller/caasfirewaller"
	controllerlifeflag "github.com/juju/juju/api/controller/lifeflag"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/http"
	"github.com/juju/juju/core/life"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/apicaller"
	"github.com/juju/juju/internal/worker/apiconfigwatcher"
	"github.com/juju/juju/internal/worker/asynccharmdownloader"
	"github.com/juju/juju/internal/worker/caasapplicationprovisioner"
	"github.com/juju/juju/internal/worker/caasfirewaller"
	"github.com/juju/juju/internal/worker/caasmodelconfigmanager"
	"github.com/juju/juju/internal/worker/caasmodeloperator"
	"github.com/juju/juju/internal/worker/charmrevisioner"
	"github.com/juju/juju/internal/worker/cleaner"
	"github.com/juju/juju/internal/worker/common"
	provisioner "github.com/juju/juju/internal/worker/computeprovisioner"
	"github.com/juju/juju/internal/worker/credentialvalidator"
	"github.com/juju/juju/internal/worker/firewaller"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/instancemutater"
	"github.com/juju/juju/internal/worker/instancepoller"
	"github.com/juju/juju/internal/worker/lifeflag"
	"github.com/juju/juju/internal/worker/logger"
	"github.com/juju/juju/internal/worker/machineundertaker"
	"github.com/juju/juju/internal/worker/migrationflag"
	"github.com/juju/juju/internal/worker/migrationmaster"
	"github.com/juju/juju/internal/worker/modelworkermanager"
	"github.com/juju/juju/internal/worker/providertracker"
	"github.com/juju/juju/internal/worker/remoterelations"
	"github.com/juju/juju/internal/worker/removal"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
	"github.com/juju/juju/internal/worker/secretspruner"
	"github.com/juju/juju/internal/worker/singular"
	"github.com/juju/juju/internal/worker/storageprovisioner"
	"github.com/juju/juju/internal/worker/undertaker"
	"github.com/juju/juju/internal/worker/unitassigner"
	"github.com/juju/juju/rpc/params"
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
	LoggingContext corelogger.LoggerContext

	// RunFlagDuration defines for how long this controller will ask
	// for model administration rights; most of the workers controlled
	// by this agent will only be started when the run flag is known
	// to be held.
	RunFlagDuration time.Duration

	// CharmRevisionUpdateInterval determines how often the charm-
	// revision worker will check for new revisions of known charms.
	CharmRevisionUpdateInterval time.Duration

	// NewEnvironFunc is a function opens a provider "environment"
	// (typically environs.New).
	NewEnvironFunc environs.NewEnvironFunc

	// NewContainerBrokerFunc is a function opens a CAAS provider.
	NewContainerBrokerFunc caas.NewContainerBrokerFunc

	// NewMigrationMaster is called to create a new migrationmaster
	// worker.
	NewMigrationMaster func(migrationmaster.Config) (worker.Worker, error)

	// ProviderServicesGetter is used to access the provider service.
	ProviderServicesGetter modelworkermanager.ProviderServicesGetter

	// DomainServices is used to access the domain services.
	DomainServices services.DomainServices

	// HTTPClientGetter is used to get a http client for a given namespace.
	HTTPClientGetter http.HTTPClientGetter
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

		// The provider domain services is used to access the provider service.
		// It's injected into the model worker manager so that it can be used
		// by the provider and broker workers.
		providerServiceFactoriesName: dependency.Manifold{
			Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
				return engine.NewValueWorker(config.ProviderServicesGetter)
			},
			Output: engine.ValueWorkerOutput,
		},

		// DomainServices is used to access the domain services.
		domainServicesName: dependency.Manifold{
			Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
				return engine.NewValueWorker(config.DomainServices)
			},
			Output: engine.ValueWorkerOutput,
		},

		// HTTPClientGetter is used to get a http client for a given namespace.
		httpClientName: dependency.Manifold{
			Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
				return engine.NewValueWorker(config.HTTPClientGetter)
			},
			Output: engine.ValueWorkerOutput,
		},

		// The logging config updater listens for logging config updates
		// for the model and configures the logging context appropriately.
		loggingConfigUpdaterName: ifNotMigrating(logger.Manifold(logger.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			LoggerContext: config.LoggingContext,
			Logger:        config.LoggingContext.GetLogger("juju.worker.logger"),
		})),

		// All other manifolds should depend on at least one of these
		// three, which handle all the tasks that are safe and sane
		// to run in *all* controller machines.
		notDeadFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Entity:        modelTag,
			Result:        life.IsNotDead,
			Filter:        LifeFilter,

			NewFacade: func(b base.APICaller) (lifeflag.Facade, error) {
				return controllerlifeflag.NewClient(b), nil
			},
			NewWorker: lifeflag.NewWorker,
			// No Logger defined in lifeflag package.
		}),
		notAliveFlagName: lifeflag.Manifold(lifeflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Entity:        modelTag,
			Result:        life.IsNotAlive,
			Filter:        LifeFilter,

			NewFacade: func(b base.APICaller) (lifeflag.Facade, error) {
				return controllerlifeflag.NewClient(b), nil
			},
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
		migrationFortressName: ifNotDead(fortress.Manifold()),
		migrationInactiveFlagName: ifNotDead(migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
		})),
		migrationMasterName: ifNotDead(migrationmaster.Manifold(migrationmaster.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			DomainServicesName: domainServicesName,
			FortressName:       migrationFortressName,
			Clock:              config.Clock,
			NewFacade:          migrationmaster.NewFacade,
			NewWorker:          config.NewMigrationMaster,
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

		charmRevisionerName: ifResponsible(charmrevisioner.Manifold(charmrevisioner.ManifoldConfig{
			DomainServicesName: domainServicesName,
			HTTPClientName:     httpClientName,
			NewHTTPClient:      charmrevisioner.NewHTTPClient,
			NewCharmhubClient:  charmrevisioner.NewCharmhubClient,
			Period:             config.CharmRevisionUpdateInterval,
			NewWorker:          charmrevisioner.NewWorker,
			ModelTag:           modelTag,
			Clock:              config.Clock,
			Logger:             config.LoggingContext.GetLogger("juju.worker.charmrevisioner"),
		})),
		remoteRelationsName: ifNotMigrating(remoterelations.Manifold(remoterelations.ManifoldConfig{
			AgentName:                agentName,
			APICallerName:            apiCallerName,
			NewControllerConnection:  apicaller.NewExternalControllerConnection,
			NewRemoteRelationsFacade: remoterelations.NewRemoteRelationsFacade,
			NewWorker:                remoterelations.NewWorker,
			Logger:                   config.LoggingContext.GetLogger("juju.worker.remoterelations", corelogger.CMR),
		})),
		removalName: ifNotMigrating(removal.Manifold(removal.ManifoldConfig{
			DomainServicesName: domainServicesName,
			GetRemovalService:  removal.GetRemovalService,
			NewWorker:          removal.NewWorker,
			Logger:             config.LoggingContext.GetLogger("juju.worker.removal"),
		})),
		stateCleanerName: ifNotMigrating(cleaner.Manifold(cleaner.ManifoldConfig{
			APICallerName: apiCallerName,
			Clock:         config.Clock,
			Logger:        config.LoggingContext.GetLogger("juju.worker.cleaner"),
		})),
		providerTrackerName: ifCredentialValid(ifResponsible(providertracker.SingularTrackerManifold(modelTag, providertracker.ManifoldConfig{
			ProviderServiceFactoriesName: providerServiceFactoriesName,
			NewWorker:                    providertracker.NewWorker,
			NewTrackerWorker:             providertracker.NewTrackerWorker,
			NewEphemeralProvider:         providertracker.NewEphemeralProvider,
			GetProviderServicesGetter:    providertracker.GetModelProviderServicesGetter,
			GetIAASProvider: providertracker.IAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
				return config.NewEnvironFunc(ctx, args, invalidator)
			}),
			GetCAASProvider: providertracker.CAASGetProvider(func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (caas.Broker, error) {
				return config.NewContainerBrokerFunc(ctx, args, invalidator)
			}),
			Logger: config.LoggingContext.GetLogger("juju.worker.providertracker"),
			Clock:  config.Clock,
		}))),

		asyncCharmDownloader: ifResponsible(asynccharmdownloader.Manifold(asynccharmdownloader.ManifoldConfig{
			DomainServicesName:     domainServicesName,
			HTTPClientName:         httpClientName,
			NewDownloader:          asynccharmdownloader.NewDownloader,
			NewHTTPClient:          asynccharmdownloader.NewHTTPClient,
			NewAsyncDownloadWorker: asynccharmdownloader.NewAsyncDownloadWorker,
			Logger:                 config.LoggingContext.GetLogger("juju.worker.asynccharmdownloader"),
			Clock:                  config.Clock,
		})),

		secretsPrunerName: ifNotMigrating(secretspruner.Manifold(secretspruner.ManifoldConfig{
			APICallerName:        apiCallerName,
			Logger:               config.LoggingContext.GetLogger("juju.worker.secretspruner"),
			NewUserSecretsFacade: secretspruner.NewUserSecretsFacade,
			NewWorker:            secretspruner.NewWorker,
		})),
		// The userSecretsDrainWorker is the worker that drains the user secrets from the inactive backend to the current active backend.
		userSecretsDrainWorker: ifNotMigrating(secretsdrainworker.Manifold(secretsdrainworker.ManifoldConfig{
			APICallerName:         apiCallerName,
			Logger:                config.LoggingContext.GetLogger("juju.worker.usersecretsdrainworker"),
			NewSecretsDrainFacade: secretsdrainworker.NewUserSecretsDrainFacade,
			NewWorker:             secretsdrainworker.NewWorker,
			NewBackendsClient:     secretsdrainworker.NewUserSecretBackendsClient,
		})),
	}
	return result
}

// IAASManifolds returns a set of interdependent dependency manifolds that will
// run together to administer an IAAS model, as configured.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()
	modelTag := agentConfig.Model()
	manifolds := dependency.Manifolds{
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
		undertakerName: ifNotAlive(undertaker.Manifold(undertaker.ManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.undertaker"),
			NewFacade:                    undertaker.NewFacade,
			NewWorker:                    undertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			NewCloudDestroyerFunc: func(ctx context.Context, params environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.CloudDestroyer, error) {
				return config.NewEnvironFunc(ctx, params, invalidator)
			},
		})),

		// All the rest depend on ifNotMigrating.
		computeProvisionerName: ifNotMigrating(provisioner.Manifold(provisioner.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			EnvironName:        providerTrackerName,
			DomainServicesName: domainServicesName,
			GetMachineService:  provisioner.GetMachineService,
			Logger:             config.LoggingContext.GetLogger("juju.worker.provisioner"),

			NewProvisionerFunc:           provisioner.NewEnvironProvisioner,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		})),
		storageProvisionerName: ifNotMigrating(storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.storageprovisioner"),
			StorageRegistryName:          providerTrackerName,
			Model:                        modelTag,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			NewWorker:                    storageprovisioner.NewStorageProvisioner,
		})),
		firewallerName: ifNotMigrating(firewaller.Manifold(firewaller.ManifoldConfig{
			AgentName:          agentName,
			APICallerName:      apiCallerName,
			DomainServicesName: domainServicesName,
			EnvironName:        providerTrackerName,
			Logger:             config.LoggingContext.GetLogger("juju.worker.firewaller"),

			NewControllerConnection:      apicaller.NewExternalControllerConnection,
			NewFirewallerWorker:          firewaller.NewWorker,
			NewFirewallerFacade:          firewaller.NewFirewallerFacade,
			NewRemoteRelationsFacade:     firewaller.NewRemoteRelationsFacade,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		})),
		unitAssignerName: ifNotMigrating(unitassigner.Manifold(unitassigner.ManifoldConfig{
			APICallerName: apiCallerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.unitassigner"),
		})),
		instancePollerName: ifNotMigrating(instancepoller.Manifold(instancepoller.ManifoldConfig{
			APICallerName:                apiCallerName,
			EnvironName:                  providerTrackerName,
			ClockName:                    clockName,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.instancepoller"),
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
		})),
		machineUndertakerName: ifNotMigrating(machineundertaker.Manifold(machineundertaker.ManifoldConfig{
			APICallerName:                apiCallerName,
			EnvironName:                  providerTrackerName,
			NewWorker:                    machineundertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.machineundertaker"),
		})),
		instanceMutaterName: ifNotMigrating(instancemutater.ModelManifold(instancemutater.ModelManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			EnvironName:   providerTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.instancemutater.environ"),
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
		undertakerName: ifNotAlive(undertaker.Manifold(undertaker.ManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.undertaker"),
			NewFacade:                    undertaker.NewFacade,
			NewWorker:                    undertaker.NewWorker,
			NewCredentialValidatorFacade: common.NewCredentialInvalidatorFacade,
			NewCloudDestroyerFunc: func(ctx context.Context, params environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.CloudDestroyer, error) {
				return config.NewContainerBrokerFunc(ctx, params, invalidator)
			},
		})),

		caasFirewallerName: ifNotMigrating(caasfirewaller.Manifold(
			caasfirewaller.ManifoldConfig{
				APICallerName:      apiCallerName,
				BrokerName:         providerTrackerName,
				DomainServicesName: domainServicesName,
				ControllerUUID:     agentConfig.Controller().Id(),
				ModelUUID:          agentConfig.Model().Id(),
				NewClient: func(caller base.APICaller) caasfirewaller.Client {
					return caasfirewallerapi.NewClient(caller)
				},
				NewWorker: caasfirewaller.NewWorker,
				Logger:    config.LoggingContext.GetLogger("juju.worker.caasfirewaller"),
			},
		)),

		caasModelOperatorName: ifResponsible(caasmodeloperator.Manifold(caasmodeloperator.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			BrokerName:    providerTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.caasmodeloperator"),
			ModelUUID:     agentConfig.Model().Id(),
		})),

		caasmodelconfigmanagerName: ifResponsible(caasmodelconfigmanager.Manifold(caasmodelconfigmanager.ManifoldConfig{
			APICallerName: apiCallerName,
			BrokerName:    providerTrackerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.caasmodelconfigmanager"),
			NewWorker:     caasmodelconfigmanager.NewWorker,
			NewFacade:     caasmodelconfigmanager.NewFacade,
			Clock:         config.Clock,
		})),

		caasApplicationProvisionerName: ifNotMigrating(caasapplicationprovisioner.Manifold(
			caasapplicationprovisioner.ManifoldConfig{
				APICallerName: apiCallerName,
				BrokerName:    providerTrackerName,
				ClockName:     clockName,
				NewWorker:     caasapplicationprovisioner.NewProvisionerWorker,
				Logger:        config.LoggingContext.GetLogger("juju.worker.caasapplicationprovisioner"),
			},
		)),
		caasStorageProvisionerName: ifNotMigrating(ifCredentialValid(storageprovisioner.ModelManifold(storageprovisioner.ModelManifoldConfig{
			APICallerName:                apiCallerName,
			Clock:                        config.Clock,
			Logger:                       config.LoggingContext.GetLogger("juju.worker.storageprovisioner"),
			StorageRegistryName:          providerTrackerName,
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
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
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

	providerTrackerName = "provider-tracker"

	applicationScalerName        = "application-scaler"
	asyncCharmDownloader         = "async-charm-downloader"
	charmRevisionerName          = "charm-revisioner"
	computeProvisionerName       = "compute-provisioner"
	domainServicesName           = "domain-services"
	firewallerName               = "firewaller"
	httpClientName               = "http-client"
	instanceMutaterName          = "instance-mutater"
	instancePollerName           = "instance-poller"
	loggingConfigUpdaterName     = "logging-config-updater"
	machineUndertakerName        = "machine-undertaker"
	providerServiceFactoriesName = "provider-service-factories"
	remoteRelationsName          = "remote-relations"
	removalName                  = "removal"
	stateCleanerName             = "state-cleaner"
	storageProvisionerName       = "storage-provisioner"
	undertakerName               = "undertaker"
	unitAssignerName             = "unit-assigner"

	caasFirewallerName             = "caas-firewaller"
	caasModelOperatorName          = "caas-model-operator"
	caasmodelconfigmanagerName     = "caas-model-config-manager"
	caasApplicationProvisionerName = "caas-application-provisioner"
	caasStorageProvisionerName     = "caas-storage-provisioner"

	secretsPrunerName      = "secrets-pruner"
	userSecretsDrainWorker = "user-secrets-drain-worker"

	validCredentialFlagName = "valid-credential-flag"
)
