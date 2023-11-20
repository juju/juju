// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stdcontext "context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/client/charms"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/observability/probe"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/common/reboot"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/secretexpire"
	"github.com/juju/juju/worker/secretrotate"
	"github.com/juju/juju/worker/trace"
	uniterapi "github.com/juju/juju/worker/uniter/api"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner"
)

// Logger represents the methods used for logging messages.
type Logger interface {
	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
	IsTraceEnabled() bool

	Child(string) loggo.Logger
	ChildWithLabels(name string, labels ...string) loggo.Logger
}

// ManifoldConfig defines the names of the manifolds on which a
// Manifold will depend.
type ManifoldConfig struct {
	AgentName             string
	APICallerName         string
	S3CallerName          string
	LeadershipTrackerName string
	CharmDirName          string
	HookRetryStrategyName string
	TraceName             string

	ModelType                    model.ModelType
	MachineLock                  machinelock.Lock
	Clock                        clock.Clock
	TranslateResolverErr         func(error) error
	Logger                       Logger
	Sidecar                      bool
	EnforcedCharmModifiedVersion int
	ContainerNames               []string
}

// Validate ensures all the required values for the config are set.
func (config *ManifoldConfig) Validate() error {
	if config.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if len(config.ModelType) == 0 {
		return errors.NotValidf("missing model type")
	}
	if config.MachineLock == nil {
		return errors.NotValidf("missing MachineLock")
	}
	if config.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a uniter worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.S3CallerName,
			config.LeadershipTrackerName,
			config.CharmDirName,
			config.HookRetryStrategyName,
			config.TraceName,
		},
		Start: func(ctx dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			// Collect all required resources.
			var agent agent.Agent
			if err := ctx.Get(config.AgentName, &agent); err != nil {
				return nil, errors.Trace(err)
			}
			var apiConn api.Connection
			if err := ctx.Get(config.APICallerName, &apiConn); err != nil {
				// TODO(fwereade): absence of an APICaller shouldn't be the end of
				// the world -- we ought to return a type that can at least run the
				// leader-deposed hook -- but that's not done yet.
				return nil, errors.Trace(err)
			}
			var leadershipTracker leadership.TrackerWorker
			if err := ctx.Get(config.LeadershipTrackerName, &leadershipTracker); err != nil {
				return nil, errors.Trace(err)
			}
			leadershipTrackerFunc := func(_ names.UnitTag) leadership.TrackerWorker {
				return leadershipTracker
			}
			var charmDirGuard fortress.Guard
			if err := ctx.Get(config.CharmDirName, &charmDirGuard); err != nil {
				return nil, errors.Trace(err)
			}

			var hookRetryStrategy params.RetryStrategy
			if err := ctx.Get(config.HookRetryStrategyName, &hookRetryStrategy); err != nil {
				return nil, errors.Trace(err)
			}

			// Ensure the agent is correctly configured with a unit tag.
			agentConfig := agent.CurrentConfig()
			tag := agentConfig.Tag()
			unitTag, ok := tag.(names.UnitTag)
			if !ok {
				return nil, errors.Errorf("expected a unit tag, got %v", tag)
			}

			// Get the tracer from the context.
			var tracerGetter trace.TracerGetter
			if err := ctx.Get(config.TraceName, &tracerGetter); err != nil {
				return nil, errors.Trace(err)
			}

			tracer, err := tracerGetter.GetTracer(stdcontext.TODO(), coretrace.Namespace("uniter", agentConfig.Model().Id()))
			if err != nil {
				tracer = coretrace.NoopTracer{}
			}

			var objectStoreCaller objectstore.Session
			if err := ctx.Get(config.S3CallerName, &objectStoreCaller); err != nil {
				return nil, errors.Trace(err)
			}

			s3Downloader := charms.NewS3CharmDownloader(objectStoreCaller, apiConn)

			jujuSecretsAPI := secretsmanager.NewClient(apiConn, uniter.WithTracer(tracer))
			secretRotateWatcherFunc := func(unitTag names.UnitTag, isLeader bool, rotateSecrets chan []string) (worker.Worker, error) {
				owners := []names.Tag{unitTag}
				if isLeader {
					appName, _ := names.UnitApplication(unitTag.Id())
					owners = append(owners, names.NewApplicationTag(appName))
				}
				return secretrotate.New(secretrotate.Config{
					SecretManagerFacade: jujuSecretsAPI,
					Clock:               config.Clock,
					Logger:              config.Logger.Child("secretsrotate"),
					SecretOwners:        owners,
					RotateSecrets:       rotateSecrets,
				})
			}
			secretExpiryWatcherFunc := func(unitTag names.UnitTag, isLeader bool, expireRevisions chan []string) (worker.Worker, error) {
				owners := []names.Tag{unitTag}
				if isLeader {
					appName, _ := names.UnitApplication(unitTag.Id())
					owners = append(owners, names.NewApplicationTag(appName))
				}
				return secretexpire.New(secretexpire.Config{
					SecretManagerFacade: jujuSecretsAPI,
					Clock:               config.Clock,
					Logger:              config.Logger.Child("secretrevisionsexpire"),
					SecretOwners:        owners,
					ExpireRevisions:     expireRevisions,
				})
			}

			resourcesClient, err := uniter.NewResourcesFacadeClient(apiConn, unitTag)
			if err != nil {
				return nil, err
			}
			payloadClient := uniter.NewPayloadFacadeClient(apiConn)

			secretsBackendGetter := func() (uniterapi.SecretsBackend, error) {
				return secrets.NewClient(jujuSecretsAPI)
			}

			manifoldConfig := config
			uniter, err := NewUniter(&UniterParams{
				UniterClient: uniterapi.UniterClientShim{
					Client: uniter.NewClient(apiConn, unitTag, uniter.WithTracer(tracer)),
				},
				ResourcesClient:              resourcesClient,
				PayloadClient:                payloadClient,
				SecretsClient:                jujuSecretsAPI,
				SecretsBackendGetter:         secretsBackendGetter,
				UnitTag:                      unitTag,
				ModelType:                    config.ModelType,
				LeadershipTrackerFunc:        leadershipTrackerFunc,
				SecretRotateWatcherFunc:      secretRotateWatcherFunc,
				SecretExpiryWatcherFunc:      secretExpiryWatcherFunc,
				DataDir:                      agentConfig.DataDir(),
				Downloader:                   s3Downloader,
				MachineLock:                  manifoldConfig.MachineLock,
				CharmDirGuard:                charmDirGuard,
				UpdateStatusSignal:           NewUpdateStatusTimer(),
				HookRetryStrategy:            hookRetryStrategy,
				NewOperationExecutor:         operation.NewExecutor,
				NewDeployer:                  charm.NewDeployer,
				NewProcessRunner:             runner.NewRunner,
				TranslateResolverErr:         config.TranslateResolverErr,
				Clock:                        manifoldConfig.Clock,
				RebootQuerier:                reboot.NewMonitor(agentConfig.TransientDataDir()),
				Logger:                       config.Logger,
				Sidecar:                      config.Sidecar,
				EnforcedCharmModifiedVersion: config.EnforcedCharmModifiedVersion,
				ContainerNames:               config.ContainerNames,
				Tracer:                       tracer,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return uniter, nil
		},
		Output: output,
	}
}

func output(in worker.Worker, out interface{}) error {
	uniter, _ := in.(*Uniter)
	if uniter == nil {
		return errors.Errorf("expected Uniter in")
	}

	switch outPtr := out.(type) {
	case *probe.ProbeProvider:
		*outPtr = &uniter.Probe
	case **Uniter:
		*outPtr = uniter
	default:
		return errors.Errorf("unknown out type")
	}
	return nil
}

// TranslateFortressErrors turns errors returned by dependent
// manifolds due to fortress lockdown (i.e. model migration) into an
// error which causes the resolver loop to be restarted. When this
// happens the uniter is about to be shut down anyway.
func TranslateFortressErrors(err error) error {
	if fortress.IsFortressError(err) {
		return resolver.ErrRestart
	}
	return err
}
