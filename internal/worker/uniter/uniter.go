// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	stdcontext "context"
	"fmt"
	"os"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/utils/v4"
	"github.com/juju/utils/v4/exec"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	coretrace "github.com/juju/juju/core/trace"
	jujucharm "github.com/juju/juju/internal/charm"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/uniter/actions"
	"github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/charm"
	"github.com/juju/juju/internal/worker/uniter/container"
	"github.com/juju/juju/internal/worker/uniter/hook"
	uniterleadership "github.com/juju/juju/internal/worker/uniter/leadership"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/reboot"
	"github.com/juju/juju/internal/worker/uniter/relation"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/runcommands"
	"github.com/juju/juju/internal/worker/uniter/runner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/context/resources"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/secrets"
	"github.com/juju/juju/internal/worker/uniter/storage"
	"github.com/juju/juju/internal/worker/uniter/verifycharmprofile"
	"github.com/juju/juju/rpc/params"
)

const (
	// ErrCAASUnitDead is the error returned from terminate or init
	// if the unit is Dead.
	ErrCAASUnitDead = errors.ConstError("unit dead")
)

// Uniter implements the capabilities of the unit agent, for example running hooks.
type Uniter struct {
	catacomb                     catacomb.Catacomb
	client                       api.UniterClient
	secretsClient                api.SecretsClient
	secretsBackendGetter         context.SecretsBackendGetter
	paths                        Paths
	unit                         api.Unit
	resources                    resources.OpenedResourceClient
	modelType                    model.ModelType
	sidecar                      bool
	enforcedCharmModifiedVersion int
	storage                      *storage.Attachments
	clock                        clock.Clock
	tracer                       coretrace.Tracer

	relationStateTracker relation.RelationStateTracker

	secretsTracker secrets.SecretStateTracker

	// Cache the last reported status information
	// so we don't make unnecessary api calls.
	setStatusMutex      sync.Mutex
	lastReportedStatus  status.Status
	lastReportedMessage string

	operationFactory     operation.Factory
	operationExecutor    operation.Executor
	newOperationExecutor NewOperationExecutorFunc
	newProcessRunner     runner.NewRunnerFunc
	newDeployer          charm.NewDeployerFunc
	translateResolverErr func(error) error

	leadershipTracker leadership.Tracker
	charmDirGuard     fortress.Guard

	hookLock machinelock.Lock

	// secretRotateWatcherFunc returns a watcher that triggers when secrets
	// owned by this unit ot its application should be rotated.
	secretRotateWatcherFunc remotestate.SecretTriggerWatcherFunc

	// secretExpiryWatcherFunc returns a watcher that triggers when
	// secret revisions owned by this unit or its application should be expired.
	secretExpiryWatcherFunc remotestate.SecretTriggerWatcherFunc

	Probe Probe

	// TODO(axw) move the runListener and run-command code outside of the
	// uniter, and introduce a separate worker. Each worker would feed
	// operations to a single, synchronized runner to execute.
	runListener      *RunListener
	localRunListener *RunListener
	commands         runcommands.Commands
	commandChannel   chan string

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// updateStatusAt defines a function that will be used to generate signals for
	// the update-status hook
	updateStatusAt remotestate.UpdateStatusTimerFunc

	// containerNames will have a list of the workload containers created alongside this
	// unit agent.
	containerNames []string

	workloadEvents       container.WorkloadEvents
	workloadEventChannel chan string

	newPebbleClient NewPebbleClientFunc

	// hookRetryStrategy represents configuration for hook retries
	hookRetryStrategy params.RetryStrategy

	// downloader is the downloader that should be used to get the charm
	// archive.
	downloader charm.Downloader

	// rebootQuerier allows the uniter to detect when the machine has
	// rebooted so we can notify the charms accordingly.
	rebootQuerier RebootQuerier
	logger        logger.Logger

	// shutdownChannel is passed to the remote state watcher. When true is
	// sent on the channel, it causes the uniter to start the shutdown process.
	shutdownChannel chan bool
}

// UniterParams hold all the necessary parameters for a new Uniter.
type UniterParams struct {
	UniterClient            api.UniterClient
	ResourcesClient         resources.OpenedResourceClient
	SecretsClient           api.SecretsClient
	SecretsBackendGetter    context.SecretsBackendGetter
	UnitTag                 names.UnitTag
	ModelType               model.ModelType
	LeadershipTrackerFunc   func(names.UnitTag) leadership.Tracker
	SecretRotateWatcherFunc remotestate.SecretTriggerWatcherFunc
	SecretExpiryWatcherFunc remotestate.SecretTriggerWatcherFunc
	DataDir                 string
	Downloader              charm.Downloader
	MachineLock             machinelock.Lock
	CharmDirGuard           fortress.Guard
	UpdateStatusSignal      remotestate.UpdateStatusTimerFunc
	HookRetryStrategy       params.RetryStrategy
	NewOperationExecutor    NewOperationExecutorFunc
	NewProcessRunner        runner.NewRunnerFunc
	NewDeployer             charm.NewDeployerFunc
	RunListener             *RunListener
	TranslateResolverErr    func(error) error
	Clock                   clock.Clock
	IsRemoteUnit            bool
	SocketConfig            *SocketConfig
	// TODO (mattyw, wallyworld, fwereade) Having the observer here make this approach a bit more legitimate, but it isn't.
	// the observer is only a stop gap to be used in tests. A better approach would be to have the uniter tests start hooks
	// that write to files, and have the tests watch the output to know that hooks have finished.
	Observer                     UniterExecutionObserver
	RebootQuerier                RebootQuerier
	Logger                       logger.Logger
	Sidecar                      bool
	EnforcedCharmModifiedVersion int
	ContainerNames               []string
	NewPebbleClient              NewPebbleClientFunc
	Tracer                       coretrace.Tracer
}

// NewOperationExecutorFunc is a func which returns an operations.Executor.
type NewOperationExecutorFunc func(stdcontext.Context, string, operation.ExecutorConfig) (operation.Executor, error)

// NewRunnerExecutorFunc defines the type of the NewRunnerExecutor.
type NewRunnerExecutorFunc func(api.ProviderIDGetter, Paths) runner.ExecFunc

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(uniterParams *UniterParams) (*Uniter, error) {
	startFunc := newUniter(uniterParams)
	w, err := startFunc()
	return w.(*Uniter), err
}

func newUniter(uniterParams *UniterParams) func() (worker.Worker, error) {
	translateResolverErr := uniterParams.TranslateResolverErr
	if translateResolverErr == nil {
		translateResolverErr = func(err error) error { return err }
	}
	startFunc := func() (worker.Worker, error) {
		u := &Uniter{
			client:                       uniterParams.UniterClient,
			resources:                    uniterParams.ResourcesClient,
			secretsClient:                uniterParams.SecretsClient,
			secretsBackendGetter:         uniterParams.SecretsBackendGetter,
			paths:                        NewPaths(uniterParams.DataDir, uniterParams.UnitTag, uniterParams.SocketConfig),
			modelType:                    uniterParams.ModelType,
			hookLock:                     uniterParams.MachineLock,
			leadershipTracker:            uniterParams.LeadershipTrackerFunc(uniterParams.UnitTag),
			secretRotateWatcherFunc:      uniterParams.SecretRotateWatcherFunc,
			secretExpiryWatcherFunc:      uniterParams.SecretExpiryWatcherFunc,
			charmDirGuard:                uniterParams.CharmDirGuard,
			updateStatusAt:               uniterParams.UpdateStatusSignal,
			hookRetryStrategy:            uniterParams.HookRetryStrategy,
			newOperationExecutor:         uniterParams.NewOperationExecutor,
			newProcessRunner:             uniterParams.NewProcessRunner,
			newDeployer:                  uniterParams.NewDeployer,
			translateResolverErr:         translateResolverErr,
			observer:                     uniterParams.Observer,
			clock:                        uniterParams.Clock,
			tracer:                       uniterParams.Tracer,
			downloader:                   uniterParams.Downloader,
			runListener:                  uniterParams.RunListener,
			rebootQuerier:                uniterParams.RebootQuerier,
			logger:                       uniterParams.Logger,
			sidecar:                      uniterParams.Sidecar,
			enforcedCharmModifiedVersion: uniterParams.EnforcedCharmModifiedVersion,
			containerNames:               uniterParams.ContainerNames,
			newPebbleClient:              uniterParams.NewPebbleClient,
			shutdownChannel:              make(chan bool, 1),
		}
		plan := catacomb.Plan{
			Name: "uniter",
			Site: &u.catacomb,
			Work: func() error {
				return u.loop(uniterParams.UnitTag)
			},
		}
		if err := catacomb.Invoke(plan); err != nil {
			return nil, errors.Trace(err)
		}
		return u, nil
	}
	return startFunc
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	ctx, cancel := u.scopedContext()
	defer cancel()

	loggo.GetLogger("****").Criticalf("LOOP")

	// Ensure that we pass in the tracer to the context, so that it can be
	// used by the resolver.
	ctx = coretrace.WithTracer(ctx, u.tracer)

	defer func() {
		// If this is a CAAS unit, then dead errors are fairly normal ways to exit
		// the uniter main loop, but the parent operator agent needs to keep running.
		errorString := "<unknown>"
		if err != nil {
			errorString = err.Error()
		}
		// If something else killed the tomb, then use that error.
		if errors.Is(err, tomb.ErrDying) {
			select {
			case <-u.catacomb.Dying():
				errorString = u.catacomb.Err().Error()
			default:
			}
		}
		if errors.Is(err, ErrCAASUnitDead) {
			errorString = err.Error()
			err = nil
		}
		if u.runListener != nil {
			u.runListener.UnregisterRunner(unitTag.Id())
		}
		if u.localRunListener != nil {
			u.localRunListener.UnregisterRunner(unitTag.Id())
		}
		u.logger.Infof(ctx, "unit %q shutting down: %s", unitTag.Id(), errorString)
	}()

	if err := u.init(ctx, unitTag); err != nil {
		switch cause := errors.Cause(err); cause {
		case resolver.ErrLoopAborted:
			return u.catacomb.ErrDying()
		case ErrCAASUnitDead:
			// Normal exit from the loop as we don't want it restarted.
			return nil
		case jworker.ErrTerminateAgent:
			return err
		default:
			return errors.Annotatef(err, "failed to initialize uniter for %q", unitTag)
		}
	}
	u.logger.Infof(ctx, "unit %q started", u.unit)

	// Check we are running the correct charm version.
	if u.sidecar && u.enforcedCharmModifiedVersion != -1 {
		app, err := u.unit.Application(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		appCharmModifiedVersion, err := app.CharmModifiedVersion(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		if appCharmModifiedVersion != u.enforcedCharmModifiedVersion {
			u.logger.Infof(ctx, "remote charm modified version (%d) does not match agent's (%d)",
				appCharmModifiedVersion, u.enforcedCharmModifiedVersion)
			return u.stopUnitError(ctx)
		}
	}

	canApplyCharmProfile, charmURL, charmModifiedVersion, err := u.charmState(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	var watcher *remotestate.RemoteStateWatcher

	u.logger.Infof(ctx, "hooks are retried %v", u.hookRetryStrategy.ShouldRetry)
	retryHookChan := make(chan struct{}, 1)
	// TODO(katco): 2016-08-09: This type is deprecated: lp:1611427
	retryHookTimer := utils.NewBackoffTimer(utils.BackoffTimerConfig{
		Min:    u.hookRetryStrategy.MinRetryTime,
		Max:    u.hookRetryStrategy.MaxRetryTime,
		Jitter: u.hookRetryStrategy.JitterRetryTime,
		Factor: u.hookRetryStrategy.RetryTimeFactor,
		Func: func() {
			// Don't try to send on the channel if it's already full
			// This can happen if the timer fires off before the event is consumed
			// by the resolver loop
			select {
			case retryHookChan <- struct{}{}:
			default:
			}
		},
		Clock: u.clock,
	})
	defer func() {
		// Whenever we exit the uniter we want to stop a potentially
		// running timer so it doesn't trigger for nothing.
		retryHookTimer.Reset()
	}()

	restartWatcher := func() error {
		if watcher != nil {
			// watcher added to catacomb, will kill uniter if there's an error.
			_ = worker.Stop(watcher)
		}
		var err error
		watcher, err = remotestate.NewWatcher(
			remotestate.WatcherConfig{
				UniterClient:                 u.client,
				LeadershipTracker:            u.leadershipTracker,
				SecretsClient:                u.secretsClient,
				SecretRotateWatcherFunc:      u.secretRotateWatcherFunc,
				SecretExpiryWatcherFunc:      u.secretExpiryWatcherFunc,
				UnitTag:                      unitTag,
				UpdateStatusChannel:          u.updateStatusAt,
				CommandChannel:               u.commandChannel,
				RetryHookChannel:             retryHookChan,
				ModelType:                    u.modelType,
				Logger:                       u.logger.Child("remotestate"),
				CanApplyCharmProfile:         canApplyCharmProfile,
				Sidecar:                      u.sidecar,
				EnforcedCharmModifiedVersion: u.enforcedCharmModifiedVersion,
				WorkloadEventChannel:         u.workloadEventChannel,
				InitialWorkloadEventIDs:      u.workloadEvents.EventIDs(),
				ShutdownChannel:              u.shutdownChannel,
			})
		if err != nil {
			return errors.Trace(err)
		}
		if err := u.catacomb.Add(watcher); err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	onIdle := func() error {
		opState := u.operationExecutor.State()
		if opState.Kind != operation.Continue {
			// We should only set idle status if we're in
			// the "Continue" state, which indicates that
			// there is nothing to do and we're not in an
			// error state.
			return nil
		}
		return setAgentStatus(ctx, u, status.Idle, "", nil)
	}

	clearResolved := func() error {
		if err := u.unit.ClearResolved(ctx); err != nil {
			return errors.Trace(err)
		}
		watcher.ClearResolvedMode()
		return nil
	}

	var rebootDetected bool
	if u.modelType == model.IAAS {
		if rebootDetected, err = u.rebootQuerier.Query(unitTag); err != nil {
			return errors.Annotatef(err, "could not check reboot status for %q", unitTag)
		}
	} else if u.modelType == model.CAAS && u.sidecar {
		rebootDetected = true
	}
	rebootResolver := reboot.NewResolver(u.logger, rebootDetected)

	for {
		if err = restartWatcher(); err != nil {
			err = errors.Annotate(err, "(re)starting watcher")
			break
		}

		cfg := ResolverConfig{
			ModelType:           u.modelType,
			ClearResolved:       clearResolved,
			ReportHookError:     u.reportHookError,
			ShouldRetryHooks:    u.hookRetryStrategy.ShouldRetry,
			StartRetryHookTimer: retryHookTimer.Start,
			StopRetryHookTimer:  retryHookTimer.Reset,
			Actions: actions.NewResolver(
				u.logger.Child("actions"),
			),
			VerifyCharmProfile: verifycharmprofile.NewResolver(
				u.logger.Child("verifycharmprofile"),
				u.modelType,
			),
			Reboot: rebootResolver,
			Leadership: uniterleadership.NewResolver(
				u.logger.Child("leadership"),
			),
			CreatedRelations: relation.NewCreatedRelationResolver(
				u.relationStateTracker, u.logger.Child("relation", logger.CMR)),
			Relations: relation.NewRelationResolver(
				u.relationStateTracker, u.unit, u.logger.Child("relation", logger.CMR)),
			Storage: storage.NewResolver(
				u.logger.Child("storage"), u.storage, u.modelType),
			Commands: runcommands.NewCommandsResolver(
				u.commands, watcher.CommandCompleted,
			),
			Secrets: secrets.NewSecretsResolver(
				u.logger.Child("secrets", logger.SECRETS),
				u.secretsTracker,
				watcher.RotateSecretCompleted,
				watcher.ExpireRevisionCompleted,
				watcher.RemoveSecretsCompleted,
			),
			Logger: u.logger,
		}
		if len(u.containerNames) > 0 {
			cfg.OptionalResolvers = append(cfg.OptionalResolvers, container.NewWorkloadHookResolver(
				u.logger.Child("workload"),
				u.workloadEvents,
				watcher.WorkloadEventCompleted),
			)
		}
		uniterResolver := NewUniterResolver(cfg)

		// We should not do anything until there has been a change
		// to the remote state. The watcher will trigger at least
		// once initially.
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case <-watcher.RemoteStateChanged():
		}

		localState := resolver.LocalState{
			CharmURL:             charmURL,
			CharmModifiedVersion: charmModifiedVersion,
		}

		for err == nil {
			err = resolver.Loop(ctx, resolver.LoopConfig{
				Resolver:      uniterResolver,
				Watcher:       watcher,
				Executor:      u.operationExecutor,
				Factory:       u.operationFactory,
				OnIdle:        onIdle,
				CharmDirGuard: u.charmDirGuard,
				CharmDir:      u.paths.State.CharmDir,
				Logger:        u.logger.Child("resolver"),
			}, &localState)

			err = u.translateResolverErr(err)

			switch {
			case err == nil:
				// Loop back around.
			case errors.Is(err, resolver.ErrLoopAborted):
				err = u.catacomb.ErrDying()
			case errors.Is(err, operation.ErrNeedsReboot):
				err = jworker.ErrRebootMachine
			case errors.Is(err, operation.ErrHookFailed):
				// Loop back around. The resolver can tell that it is in
				// an error state by inspecting the operation state.
				err = nil
			case errors.Is(err, runner.ErrTerminated):
				localState.HookWasShutdown = true
				err = nil
			case errors.Is(err, resolver.ErrUnitDead):
				err = u.terminate(ctx)
			case errors.Is(err, resolver.ErrRestart):
				// make sure we update the two values used above in
				// creating LocalState.
				charmURL = localState.CharmURL
				charmModifiedVersion = localState.CharmModifiedVersion
				// leave err assigned, causing loop to break
			case errors.Is(err, jworker.ErrTerminateAgent):
				// terminate agent
			default:
				// We need to set conflicted from here, because error
				// handling is outside of the resolver's control.
				if _, is := errors.AsType[*operation.DeployConflictError](err); is {
					localState.Conflicted = true
					err = setAgentStatus(ctx, u, status.Error, "upgrade failed", nil)
				} else {
					reportAgentError(ctx, u, "resolver loop error", err)
				}
			}
		}

		if !errors.Is(err, resolver.ErrRestart) {
			break
		}
	}
	return err
}

func (u *Uniter) verifyCharmProfile(ctx stdcontext.Context, url string) error {
	// NOTE: this is very similar code to verifyCharmProfile.NextOp,
	// if you make changes here, check to see if they are needed there.
	ch, err := u.client.Charm(url)
	if err != nil {
		return errors.Trace(err)
	}
	required, err := ch.LXDProfileRequired(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if !required {
		// If no lxd profile is required for this charm, move on.
		u.logger.Debugf(ctx, "no lxd profile required for %s", url)
		return nil
	}
	profile, err := u.unit.LXDProfileName(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if profile == "" {
		if err := u.unit.SetUnitStatus(ctx, status.Waiting, "required charm profile not yet applied to machine", nil); err != nil {
			return errors.Trace(err)
		}
		u.logger.Debugf(ctx, "required lxd profile not found on machine")
		return errors.NotFoundf("required charm profile on machine")
	}
	// double check profile revision matches charm revision.
	rev, err := lxdprofile.ProfileRevision(profile)
	if err != nil {
		return errors.Trace(err)
	}
	curl, err := jujucharm.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}
	if rev != curl.Revision {
		if err := u.unit.SetUnitStatus(ctx, status.Waiting, fmt.Sprintf("required charm profile %q not yet applied to machine", profile), nil); err != nil {
			return errors.Trace(err)
		}
		u.logger.Debugf(ctx, "charm is revision %d, charm profile has revision %d", curl.Revision, rev)
		return errors.NotFoundf("required charm profile, %q, on machine", profile)
	}
	u.logger.Debugf(ctx, "required lxd profile %q FOUND on machine", profile)
	if err := u.unit.SetUnitStatus(ctx, status.Waiting, status.MessageInitializingAgent, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// charmState returns data for the local state setup.
// While gathering the data, look for interrupted Install or pending
// charm upgrade, execute if found.
func (u *Uniter) charmState(ctx stdcontext.Context) (bool, string, int, error) {
	// Install is a special case, as it must run before there
	// is any remote state, and before the remote state watcher
	// is started.
	var charmURL string
	var charmModifiedVersion int

	canApplyCharmProfile, err := u.unit.CanApplyLXDProfile(ctx)
	if err != nil {
		return canApplyCharmProfile, charmURL, charmModifiedVersion, err
	}

	opState := u.operationExecutor.State()
	if opState.Kind == operation.Install {
		u.logger.Infof(ctx, "resuming charm install")
		if canApplyCharmProfile {
			// Note: canApplyCharmProfile will be false for a CAAS model.
			// Verify the charm profile before proceeding.
			if err := u.verifyCharmProfile(ctx, opState.CharmURL); err != nil {
				return canApplyCharmProfile, charmURL, charmModifiedVersion, err
			}
		}
		op, err := u.operationFactory.NewInstall(opState.CharmURL)
		if err != nil {
			return canApplyCharmProfile, charmURL, charmModifiedVersion, errors.Trace(err)
		}
		if err := u.operationExecutor.Run(ctx, op, nil); err != nil {
			return canApplyCharmProfile, charmURL, charmModifiedVersion, errors.Trace(err)
		}
		charmURL = opState.CharmURL
		return canApplyCharmProfile, charmURL, charmModifiedVersion, nil
	}
	// No install needed, find the curl and start.
	curl, err := u.unit.CharmURL(ctx)
	if err != nil {
		return canApplyCharmProfile, charmURL, charmModifiedVersion, errors.Trace(err)
	}
	charmURL = curl
	app, err := u.unit.Application(ctx)
	if err != nil {
		return canApplyCharmProfile, charmURL, charmModifiedVersion, errors.Trace(err)
	}

	// TODO (hml) 25-09-2020 - investigate
	// This assumes that the uniter is not restarting after an application
	// changed notification, with changes to CharmModifiedVersion, but before
	// it could be acted on.
	charmModifiedVersion, err = app.CharmModifiedVersion(ctx)
	if err != nil {
		return canApplyCharmProfile, charmURL, charmModifiedVersion, errors.Trace(err)
	}

	return canApplyCharmProfile, charmURL, charmModifiedVersion, nil
}

func (u *Uniter) terminate(ctx stdcontext.Context) error {
	unitWatcher, err := u.unit.Watch(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if err := u.catacomb.Add(unitWatcher); err != nil {
		return errors.Trace(err)
	}
	for {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case _, ok := <-unitWatcher.Changes():
			if !ok {
				return errors.New("unit watcher closed")
			}
			if err := u.unit.Refresh(ctx); err != nil {
				return errors.Trace(err)
			}
			if hasSubs, err := u.unit.HasSubordinates(ctx); err != nil {
				return errors.Trace(err)
			} else if hasSubs {
				continue
			}
			// The unit is known to be Dying; so if it didn't have subordinates
			// just above, it can't acquire new ones before this call.
			// The same goes for secrets.

			// Just before the transition to dead, remove any secret content
			// for secrets owned by this unit.
			// We only handle unit owned secrets here. Any app owned secrets
			// can only be deleted when the app itself is removed. This is
			// done in the api server.
			u.logger.Debugf(ctx, "deleting secret content")
			secrets, err := u.secretsClient.SecretMetadata(ctx)
			if err != nil {
				return errors.Trace(err)
			}
			backend, err := u.secretsBackendGetter()
			if err != nil {
				return errors.Trace(err)
			}
			for _, s := range secrets {
				if s.Metadata.Owner.Kind == coresecrets.ApplicationOwner {
					continue
				}
				for _, rev := range s.Revisions {
					err = backend.DeleteContent(ctx, s.Metadata.URI, rev)
					if err != nil {
						return errors.Annotatef(err, "deleting secret content for %s/%d", s.Metadata.URI.ID, rev)
					}
				}
			}

			if err := u.unit.EnsureDead(ctx); err != nil {
				return errors.Trace(err)
			}

			return u.stopUnitError(ctx)
		}
	}
}

// stopUnitError returns the error to use when exiting from stopping the unit.
// For IAAS models, we want to terminate the agent, as each unit is run by
// an individual agent for that unit.
func (u *Uniter) stopUnitError(ctx stdcontext.Context) error {
	u.logger.Debugf(ctx, "u.modelType: %s", u.modelType)
	if u.modelType == model.CAAS {
		if u.sidecar {
			return errors.WithType(jworker.ErrTerminateAgent, ErrCAASUnitDead)
		}
		return ErrCAASUnitDead
	}
	return jworker.ErrTerminateAgent
}

func (u *Uniter) init(ctx stdcontext.Context, unitTag names.UnitTag) (err error) {
	switch u.modelType {
	case model.IAAS, model.CAAS:
		// known types, all good
	default:
		return errors.Errorf("unknown model type %q", u.modelType)
	}

	loggo.GetLogger("*****").Criticalf("UNIT %q STARTING UP", unitTag.Id())

	// If we started up already dead, we should not progress further.
	// If we become Dead immediately after starting up, we may well
	// complete any operations in progress before detecting it,
	// but that race is fundamental and inescapable,
	// whereas this one is not.
	u.unit, err = u.client.Unit(ctx, unitTag)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return u.stopUnitError(ctx)
		}
		return errors.Trace(err)
	}
	if u.unit.Life() == life.Dead {
		return u.stopUnitError(ctx)
	}

	// If initialising for the first time after deploying, update the status.
	currentStatus, err := u.unit.UnitStatus(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	loggo.GetLogger("*****").Criticalf("WAITING FOR UNIT %q TO INITIALIZE %v", u.unit.Name(), currentStatus)

	// TODO(fwereade/wallyworld): we should have an explicit place in the model
	// to tell us when we've hit this point, instead of piggybacking on top of
	// status and/or status history.
	// If the previous status was waiting for machine, we transition to the next step.
	if currentStatus.Status == string(status.Waiting) &&
		(currentStatus.Info == status.MessageWaitForMachine || currentStatus.Info == status.MessageInstallingAgent) {
		if err := u.unit.SetUnitStatus(ctx, status.Waiting, status.MessageInitializingAgent, nil); err != nil {
			return errors.Trace(err)
		}
	}
	if err := tools.EnsureSymlinks(u.paths.ToolsDir, u.paths.ToolsDir, jujuc.CommandNames()); err != nil {
		return err
	}
	relStateTracker, err := relation.NewRelationStateTracker(ctx,
		relation.RelationStateTrackerConfig{
			Client:            u.client,
			Unit:              u.unit,
			LeadershipContext: context.NewLeadershipContext(u.leadershipTracker),
			CharmDir:          u.paths.State.CharmDir,
			Abort:             u.catacomb.Dying(),
			Logger:            u.logger.Child("relation"),
		})
	if err != nil {
		return errors.Annotatef(err, "cannot create relation state tracker")
	}
	u.relationStateTracker = relStateTracker
	u.commands = runcommands.NewCommands()
	u.commandChannel = make(chan string)

	storageAttachments, err := storage.NewAttachments(
		ctx,
		u.client, unitTag, u.unit, u.catacomb.Dying(),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create storage hook source")
	}
	u.storage = storageAttachments

	secretsTracker, err := secrets.NewSecrets(
		ctx,
		u.secretsClient, unitTag, u.unit,
		u.logger.Child("secrets", logger.SECRETS),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create secrets tracker")
	}
	u.secretsTracker = secretsTracker

	if err := charm.ClearDownloads(u.paths.State.BundlesDir); err != nil {
		u.logger.Warningf(ctx, err.Error())
	}
	charmLogger := u.logger.Child("charm")
	deployer, err := u.newDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(
			u.paths.State.BundlesDir,
			u.downloader,
			charmLogger),
		charmLogger,
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create deployer")
	}
	contextFactory, err := context.NewContextFactory(ctx, context.FactoryConfig{
		Uniter:               u.client,
		SecretsClient:        u.secretsClient,
		SecretsBackendGetter: u.secretsBackendGetter,
		Unit:                 u.unit,
		Resources:            u.resources,
		Tracker:              u.leadershipTracker,
		GetRelationInfos:     u.relationStateTracker.GetInfo,
		Paths:                u.paths,
		Clock:                u.clock,
		Logger:               u.logger.Child("context"),
	})
	if err != nil {
		return err
	}
	runnerFactory, err := runner.NewFactory(
		u.paths, contextFactory, u.newProcessRunner,
	)
	if err != nil {
		return errors.Trace(err)
	}
	u.operationFactory = operation.NewFactory(operation.FactoryParams{
		Deployer:       deployer,
		RunnerFactory:  runnerFactory,
		Callbacks:      &operationCallbacks{u},
		ActionGetter:   u.client,
		MetricSpoolDir: u.paths.GetMetricsSpoolDir(),
		Logger:         u.logger.Child("operation"),
	})

	charmURL, err := u.getApplicationCharmURL(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	initialState := operation.State{
		Kind:     operation.Install,
		Step:     operation.Queued,
		CharmURL: charmURL,
	}

	operationExecutor, err := u.newOperationExecutor(ctx, u.unit.Name(), operation.ExecutorConfig{
		StateReadWriter: u.unit,
		InitialState:    initialState,
		AcquireLock:     u.acquireExecutionLock,
		Logger:          u.logger.Child("operation"),
	})
	if err != nil {
		return errors.Trace(err)
	}
	u.operationExecutor = operationExecutor

	// Ensure we have an agent directory to to write the socket.
	if err := os.MkdirAll(u.paths.State.BaseDir, 0755); err != nil {
		return errors.Trace(err)
	}
	socket := u.paths.Runtime.LocalJujuExecSocket.Server
	u.logger.Debugf(ctx, "starting local juju-exec listener on %v", socket)
	u.localRunListener, err = NewRunListener(socket, u.logger)
	if err != nil {
		return errors.Annotate(err, "creating juju run listener")
	}
	rlw := NewRunListenerWrapper(u.localRunListener, u.logger)
	if err := u.catacomb.Add(rlw); err != nil {
		return errors.Trace(err)
	}

	commandRunner, err := NewChannelCommandRunner(ChannelCommandRunnerConfig{
		Abort:          u.catacomb.Dying(),
		Commands:       u.commands,
		CommandChannel: u.commandChannel,
	})
	if err != nil {
		return errors.Annotate(err, "creating command runner")
	}
	u.localRunListener.RegisterRunner(u.unit.Name(), commandRunner)
	if u.runListener != nil {
		u.runListener.RegisterRunner(u.unit.Name(), commandRunner)
	}

	u.workloadEvents = container.NewWorkloadEvents()
	u.workloadEventChannel = make(chan string)
	if len(u.containerNames) > 0 {
		poller := NewPebblePoller(u.logger, u.clock, u.containerNames, u.workloadEventChannel, u.workloadEvents, u.newPebbleClient)
		if err := u.catacomb.Add(poller); err != nil {
			return errors.Trace(err)
		}
		noticer := NewPebbleNoticer(u.logger, u.clock, u.containerNames, u.workloadEventChannel, u.workloadEvents, u.newPebbleClient)
		if err := u.catacomb.Add(noticer); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (u *Uniter) Kill() {
	u.catacomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.catacomb.Wait()
}

func (u *Uniter) getApplicationCharmURL(ctx stdcontext.Context) (string, error) {
	// TODO(fwereade): pretty sure there's no reason to make 2 API calls here.
	app, err := u.client.Application(ctx, u.unit.ApplicationTag())
	if err != nil {
		return "", err
	}
	charmURL, _, err := app.CharmURL(ctx)
	return charmURL, err
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	// TODO(axw) drop this when we move the run-listener to an independent
	// worker. This exists purely for the tests.
	return u.localRunListener.RunCommands(args)
}

// acquireExecutionLock acquires the machine-level execution lock, and
// returns a func that must be called to unlock it. It's used by operation.Executor
// when running operations that execute external code.
func (u *Uniter) acquireExecutionLock(action, executionGroup string) (func(), error) {
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's catacomb into account.
	spec := machinelock.Spec{
		Cancel:  u.catacomb.Dying(),
		Worker:  fmt.Sprintf("%s uniter", u.unit.Name()),
		Comment: action,
		Group:   executionGroup,
	}
	releaser, err := u.hookLock.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return releaser, nil
}

func (u *Uniter) reportHookError(ctx stdcontext.Context, hookInfo hook.Info) error {
	// Set the agent status to "error". We must do this here in case the
	// hook is interrupted (e.g. unit agent crashes), rather than immediately
	// after attempting a runHookOp.
	hookName := string(hookInfo.Kind)
	hookMessage := string(hookInfo.Kind)
	statusData := map[string]interface{}{}
	if hookInfo.Kind.IsRelation() {
		statusData["relation-id"] = hookInfo.RelationId
		if hookInfo.RemoteUnit != "" {
			statusData["remote-unit"] = hookInfo.RemoteUnit
		}
		relationName, err := u.relationStateTracker.Name(hookInfo.RelationId)
		if err != nil {
			hookMessage = fmt.Sprintf("%s: %v", hookInfo.Kind, err)
		} else {
			hookName = fmt.Sprintf("%s-%s", relationName, hookInfo.Kind)
			hookMessage = hookName
		}
	}
	if hookInfo.Kind.IsSecret() {
		statusData["secret-uri"] = hookInfo.SecretURI
		statusData["secret-label"] = hookInfo.SecretLabel
	}
	statusData["hook"] = hookName
	statusMessage := fmt.Sprintf("hook failed: %q", hookMessage)
	return setAgentStatus(ctx, u, status.Error, statusMessage, statusData)
}

// Terminate terminates the Uniter worker, ensuring the stop hook is fired before
// exiting with ErrTerminateAgent.
func (u *Uniter) Terminate() error {
	select {
	case u.shutdownChannel <- true:
	default:
	}
	return nil
}

// Report provides information for the engine report.
func (u *Uniter) Report() map[string]interface{} {
	result := make(map[string]interface{})

	// We need to guard against attempting to report when setting up or dying,
	// so we don't end up panic'ing with missing information.
	if u.unit != nil {
		result["unit"] = u.unit.Name()
	}
	if u.operationExecutor != nil {
		result["local-state"] = u.operationExecutor.State().Report()
	}
	if u.relationStateTracker != nil {
		result["relations"] = u.relationStateTracker.Report()
	}
	if u.secretsTracker != nil {
		result["secrets"] = u.secretsTracker.Report()
	}

	return result
}

// scopedContext returns a context that is in the scope of the worker lifetime.
// It returns a cancellable context that is cancelled when the action has
// completed.
func (u *Uniter) scopedContext() (stdcontext.Context, stdcontext.CancelFunc) {
	ctx, cancel := stdcontext.WithCancel(u.catacomb.Context(stdcontext.Background()))
	return ctx, cancel
}
