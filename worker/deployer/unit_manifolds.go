// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3/voyeur"
	"github.com/juju/worker/v3/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	msapi "github.com/juju/juju/api/agent/meterstatus"
	"github.com/juju/juju/api/base"
	commonapi "github.com/juju/juju/api/common"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apiaddressupdater"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/apiconfigwatcher"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/leadership"
	loggerworker "github.com/juju/juju/worker/logger"
	"github.com/juju/juju/worker/logsender"
	"github.com/juju/juju/worker/meterstatus"
	"github.com/juju/juju/worker/metrics/collect"
	"github.com/juju/juju/worker/metrics/sender"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/migrationflag"
	"github.com/juju/juju/worker/migrationminion"
	"github.com/juju/juju/worker/retrystrategy"
	"github.com/juju/juju/worker/s3caller"
	"github.com/juju/juju/worker/secretsdrainworker"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/upgrader"
)

// UnitManifoldsConfig allows specialisation of the result of Manifolds.
type UnitManifoldsConfig struct {

	// LoggingContext holds the unit writers so that the loggers
	// for the unit get tagged with the right source.
	LoggingContext *loggo.Context

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// LogSource will be read from by the logsender component.
	LogSource logsender.LogRecordCh

	// LeadershipGuarantee controls the behaviour of the leadership tracker.
	LeadershipGuarantee time.Duration

	// AgentConfigChanged is set whenever the unit agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// ValidateMigration is called by the migrationminion during the
	// migration process to check that the agent will be ok when
	// connected to the new target controller.
	ValidateMigration func(base.APICaller) error

	// UpdateLoggerConfig is a function that will save the specified
	// config value as the logging config in the agent.conf file.
	UpdateLoggerConfig func(string) error

	// MachineLock is a central source for acquiring the machine lock.
	// This is used by a number of workers to ensure serialisation of actions
	// across the machine.
	MachineLock machinelock.Lock

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock
}

// UnitManifolds returns a set of co-configured manifolds covering the various
// responsibilities of nested unit agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func UnitManifolds(config UnitManifoldsConfig) dependency.Manifolds {

	// connectFilter exists to let us retry api connections immediately
	// on password change, rather than causing the dependency engine to
	// wait for a while.
	connectFilter := func(err error) error {
		cause := errors.Cause(err)
		if cause == apicaller.ErrChangedPassword {
			return dependency.ErrBounce
		} else if cause == apicaller.ErrConnectImpossible {
			// TODO: almost certainly want a different error here.
			return worker.ErrTerminateAgent
		}
		return err
	}

	return dependency.Manifolds{

		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		// (Currently, that is "all manifolds", but consider a shared clock.)
		agentName: agent.Manifold(config.Agent),

		// The api-config-watcher manifold monitors the API server
		// addresses in the agent config and bounces when they
		// change. It's required as part of model migrations.
		apiConfigWatcherName: apiconfigwatcher.Manifold(apiconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
			Logger:             config.LoggingContext.GetLogger("juju.worker.apiconfigwatcher"),
		}),

		// The api caller is a thin concurrent wrapper around a connection
		// to some API server. It's used by many other manifolds, which all
		// select their own desired facades. It will be interesting to see
		// how this works when we consolidate the agents; might be best to
		// handle the auth changes server-side..?
		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:            agentName,
			APIConfigWatcherName: apiConfigWatcherName,
			APIOpen:              api.Open,
			NewConnection:        apicaller.ScaryConnect,
			Filter:               connectFilter,
			Logger:               config.LoggingContext.GetLogger("juju.worker.apicaller"),
		}),

		// The S3 API caller is a shim API that wraps the /charms REST
		// API for uploading and downloading charms. It provides a
		// S3-compatible API.
		s3CallerName: s3caller.Manifold(s3caller.ManifoldConfig{
			AgentName:            agentName,
			APIConfigWatcherName: apiConfigWatcherName,
			APICallerName:        apiCallerName,
			NewS3Client:          s3caller.NewS3Client,
			Filter:               connectFilter,
			Logger:               loggo.GetLogger("juju.worker.s3caller"),
		}),

		// The log sender is a leaf worker that sends log messages to some
		// API server, when configured so to do. We should only need one of
		// these in a consolidated agent.
		logSenderName: logsender.Manifold(logsender.ManifoldConfig{
			APICallerName: apiCallerName,
			LogSource:     config.LogSource,
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
		migrationFortressName: fortress.Manifold(),
		migrationInactiveFlagName: migrationflag.Manifold(migrationflag.ManifoldConfig{
			APICallerName: apiCallerName,
			Check:         migrationflag.IsTerminal,
			NewFacade:     migrationflag.NewFacade,
			NewWorker:     migrationflag.NewWorker,
			// No Logger defined in migrationflag package.
		}),
		migrationMinionName: migrationminion.Manifold(migrationminion.ManifoldConfig{
			AgentName:         agentName,
			APICallerName:     apiCallerName,
			FortressName:      migrationFortressName,
			Clock:             config.Clock,
			APIOpen:           api.Open,
			ValidateMigration: config.ValidateMigration,
			NewFacade:         migrationminion.NewFacade,
			NewWorker:         migrationminion.NewWorker,
			Logger:            config.LoggingContext.GetLogger("juju.worker.migrationminion", corelogger.MIGRATION),
		}),

		// The logging config updater is a leaf worker that indirectly
		// controls the messages sent via the log sender according to
		// changes in environment config. We should only need one of
		// these in a consolidated agent (not yet).
		loggingConfigUpdaterName: ifNotMigrating(loggerworker.Manifold(loggerworker.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			LoggingContext:  config.LoggingContext,
			Logger:          config.LoggingContext.GetLogger("juju.worker.logger"),
			UpdateAgentFunc: config.UpdateLoggerConfig,
		})),

		// The api address updater is a leaf worker that rewrites agent config
		// as the controller addresses change. We should only need one of
		// these in a consolidated agent.
		apiAddressUpdaterName: ifNotMigrating(apiaddressupdater.Manifold(apiaddressupdater.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.apiaddressupdater"),
		})),

		// The charmdir resource coordinates whether the charm directory is
		// available or not; after 'start' hook and before 'stop' hook
		// executes, and not during upgrades.
		charmDirName: ifNotMigrating(fortress.Manifold()),

		// The leadership tracker attempts to secure and retain leadership of
		// the unit's service, and is consulted on such matters by the
		// uniter. As it stands today, we'll need one per unit in a
		// consolidated agent.
		leadershipTrackerName: ifNotMigrating(leadership.Manifold(leadership.ManifoldConfig{
			AgentName:           agentName,
			APICallerName:       apiCallerName,
			Clock:               config.Clock,
			LeadershipGuarantee: config.LeadershipGuarantee,
		})),

		// HookRetryStrategy uses a retrystrategy worker to get a
		// retry strategy that will be used by the uniter to run its hooks.
		hookRetryStrategyName: ifNotMigrating(retrystrategy.Manifold(retrystrategy.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			NewFacade:     retrystrategy.NewFacade,
			NewWorker:     retrystrategy.NewRetryStrategyWorker,
			Logger:        config.LoggingContext.GetLogger("juju.worker.retrystrategy"),
		})),

		// The uniter installs charms; manages the unit's presence in its
		// relations; creates subordinate units; runs all the hooks; sends
		// metrics; etc etc etc. We expect to break it up further in the
		// coming weeks, and to need one per unit in a consolidated agent
		// (and probably one for each component broken out).
		uniterName: ifNotMigrating(uniter.Manifold(uniter.ManifoldConfig{
			AgentName:             agentName,
			ModelType:             model.IAAS,
			APICallerName:         apiCallerName,
			S3CallerName:          s3CallerName,
			MachineLock:           config.MachineLock,
			Clock:                 config.Clock,
			LeadershipTrackerName: leadershipTrackerName,
			CharmDirName:          charmDirName,
			HookRetryStrategyName: hookRetryStrategyName,
			TranslateResolverErr:  uniter.TranslateFortressErrors,
			Logger:                config.LoggingContext.GetLogger("juju.worker.uniter"),
		})),

		// TODO (mattyw) should be added to machine agent.
		metricSpoolName: ifNotMigrating(spool.Manifold(spool.ManifoldConfig{
			AgentName: agentName,
		})),

		// The metric collect worker executes the collect-metrics hook in a
		// restricted context that can safely run concurrently with other hooks.
		metricCollectName: ifNotMigrating(collect.Manifold(collect.ManifoldConfig{
			AgentName:       agentName,
			MetricSpoolName: metricSpoolName,
			CharmDirName:    charmDirName,
			Clock:           config.Clock,
			Logger:          config.LoggingContext.GetLogger("juju.worker.metrics.collect"),
		})),

		// The meter status worker executes the meter-status-changed hook when it detects
		// that the meter status has changed.
		meterStatusName: ifNotMigrating(meterstatus.Manifold(meterstatus.ManifoldConfig{
			AgentName:                agentName,
			APICallerName:            apiCallerName,
			MachineLock:              config.MachineLock,
			Clock:                    config.Clock,
			Logger:                   config.LoggingContext.GetLogger("juju.worker.meterstatus"),
			NewHookRunner:            meterstatus.NewHookRunner,
			NewMeterStatusAPIClient:  msapi.NewClient,
			NewUniterStateAPIClient:  commonapi.NewUniterStateAPI,
			NewConnectedStatusWorker: meterstatus.NewConnectedStatusWorker,
			NewIsolatedStatusWorker:  meterstatus.NewIsolatedStatusWorker,
		})),

		// The metric sender worker periodically sends accumulated metrics to the controller.
		metricSenderName: ifNotMigrating(sender.Manifold(sender.ManifoldConfig{
			AgentName:       agentName,
			APICallerName:   apiCallerName,
			MetricSpoolName: metricSpoolName,
		})),

		// For the nested deployer, the upgrade worker is only used to record
		// the running agent version for the unit. It then stops.
		upgraderName: upgrader.Manifold(upgrader.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			Logger:        config.LoggingContext.GetLogger("juju.worker.upgrader"),
			Clock:         config.Clock,
		}),

		// The secretDrainWorker is the worker that drains secrets from the inactive backend to the current active backend.
		secretDrainWorker: ifNotMigrating(secretsdrainworker.Manifold(secretsdrainworker.ManifoldConfig{
			APICallerName:         apiCallerName,
			Logger:                config.LoggingContext.GetLogger("juju.worker.secretsdrainworker"),
			NewSecretsDrainFacade: secretsdrainworker.NewSecretsDrainFacadeForAgent,
			NewWorker:             secretsdrainworker.NewWorker,
			NewBackendsClient:     secretsdrainworker.NewSecretBackendsClientForAgent,
		})),
	}
}

var ifNotMigrating = engine.Housing{
	Flags: []string{
		migrationInactiveFlagName,
	},
	Occupy: migrationFortressName,
}.Decorate

const (
	agentName            = "agent"
	apiConfigWatcherName = "api-config-watcher"
	apiCallerName        = "api-caller"
	s3CallerName         = "s3-caller"
	logSenderName        = "log-sender"

	migrationFortressName     = "migration-fortress"
	migrationInactiveFlagName = "migration-inactive-flag"
	migrationMinionName       = "migration-minion"

	loggingConfigUpdaterName = "logging-config-updater"
	apiAddressUpdaterName    = "api-address-updater"

	charmDirName          = "charm-dir"
	leadershipTrackerName = "leadership-tracker"
	hookRetryStrategyName = "hook-retry-strategy"
	uniterName            = "uniter"
	upgraderName          = "upgrader"

	metricSpoolName   = "metric-spool"
	meterStatusName   = "meter-status"
	metricCollectName = "metric-collect"
	metricSenderName  = "metric-sender"

	secretDrainWorker = "secret-drain-worker"
)
