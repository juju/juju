// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"
	"sync"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/catacomb"

	"github.com/juju/juju/agent/tools"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	uniterleadership "github.com/juju/juju/worker/uniter/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runcommands"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/uniter/storage"
	"github.com/juju/juju/worker/uniter/upgradeseries"
)

var (
	logger = loggo.GetLogger("juju.worker.uniter")

	// ErrCAASUnitDead is the error returned from terminate or init
	// if the unit is Dead.
	ErrCAASUnitDead = errors.New("unit dead")
)

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
}

// RebootQuerier is implemented by types that can deliver one-off machine
// reboot notifications to entities.
type RebootQuerier interface {
	Query(tag names.Tag) (bool, error)
}

// Uniter implements the capabilities of the unit agent. It is not intended to
// implement the actual *behaviour* of the unit agent; that responsibility is
// delegated to Mode values, which are expected to react to events and direct
// the uniter's responses to them.
type Uniter struct {
	catacomb  catacomb.Catacomb
	st        *uniter.State
	paths     Paths
	unit      *uniter.Unit
	modelType model.ModelType
	storage   *storage.Attachments
	clock     clock.Clock

	relationStateTracker relation.RelationStateTracker

	// Cache the last reported status information
	// so we don't make unnecessary api calls.
	setStatusMutex      sync.Mutex
	lastReportedStatus  status.Status
	lastReportedMessage string

	operationFactory        operation.Factory
	operationExecutor       operation.Executor
	newOperationExecutor    NewOperationExecutorFunc
	newRemoteRunnerExecutor NewRunnerExecutorFunc
	translateResolverErr    func(error) error

	leadershipTracker leadership.TrackerWorker
	charmDirGuard     fortress.Guard

	hookLock machinelock.Lock

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

	// applicationChannel, if set, is used to signal a change in the
	// application's charm. It is passed to the remote state watcher.
	applicationChannel watcher.NotifyChannel

	// runningStatusChannel, if set, is used to signal a change in the
	// unit's status. It is passed to the remote state watcher.
	runningStatusChannel watcher.NotifyChannel

	// runningStatusFunc used to determine the unit's running status.
	runningStatusFunc remotestate.RunningStatusFunc

	// hookRetryStrategy represents configuration for hook retries
	hookRetryStrategy params.RetryStrategy

	// downloader is the downloader that should be used to get the charm
	// archive.
	downloader charm.Downloader

	// rebootQuerier allows the uniter to detect when the machine has
	// rebooted so we can notify the charms accordingly.
	rebootQuerier RebootQuerier
}

// UniterParams hold all the necessary parameters for a new Uniter.
type UniterParams struct {
	UniterFacade            *uniter.State
	UnitTag                 names.UnitTag
	ModelType               model.ModelType
	LeadershipTracker       leadership.TrackerWorker
	DataDir                 string
	Downloader              charm.Downloader
	MachineLock             machinelock.Lock
	CharmDirGuard           fortress.Guard
	UpdateStatusSignal      remotestate.UpdateStatusTimerFunc
	HookRetryStrategy       params.RetryStrategy
	NewOperationExecutor    NewOperationExecutorFunc
	NewRemoteRunnerExecutor NewRunnerExecutorFunc
	RunListener             *RunListener
	TranslateResolverErr    func(error) error
	Clock                   clock.Clock
	ApplicationChannel      watcher.NotifyChannel
	RunningStatusChannel    watcher.NotifyChannel
	RunningStatusFunc       remotestate.RunningStatusFunc
	SocketConfig            *SocketConfig
	// TODO (mattyw, wallyworld, fwereade) Having the observer here make this approach a bit more legitimate, but it isn't.
	// the observer is only a stop gap to be used in tests. A better approach would be to have the uniter tests start hooks
	// that write to files, and have the tests watch the output to know that hooks have finished.
	Observer      UniterExecutionObserver
	RebootQuerier RebootQuerier
}

// NewOperationExecutorFunc is a func which returns an operations.Executor.
type NewOperationExecutorFunc func(operation.ExecutorConfig) (operation.Executor, error)

// ProviderIDGetter defines the API to get provider ID.
type ProviderIDGetter interface {
	ProviderID() string
	Refresh() error
	Name() string
}

// NewRunnerExecutorFunc defines the type of the NewRunnerExecutor.
type NewRunnerExecutorFunc func(ProviderIDGetter, Paths) runner.ExecFunc

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(uniterParams *UniterParams) (*Uniter, error) {
	startFunc := newUniter(uniterParams)
	w, err := startFunc()
	return w.(*Uniter), err
}

// StartUniter creates a new Uniter and starts it using the specified runner.
func StartUniter(runner *worker.Runner, params *UniterParams) error {
	startFunc := newUniter(params)

	logger.Debugf("starting uniter for  %q", params.UnitTag.Id())
	err := runner.StartWorker(params.UnitTag.Id(), startFunc)
	return errors.Annotate(err, "error starting uniter worker")
}

func newUniter(uniterParams *UniterParams) func() (worker.Worker, error) {
	translateResolverErr := uniterParams.TranslateResolverErr
	if translateResolverErr == nil {
		translateResolverErr = func(err error) error { return err }
	}
	u := &Uniter{
		st:                      uniterParams.UniterFacade,
		paths:                   NewPaths(uniterParams.DataDir, uniterParams.UnitTag, uniterParams.SocketConfig),
		modelType:               uniterParams.ModelType,
		hookLock:                uniterParams.MachineLock,
		leadershipTracker:       uniterParams.LeadershipTracker,
		charmDirGuard:           uniterParams.CharmDirGuard,
		updateStatusAt:          uniterParams.UpdateStatusSignal,
		hookRetryStrategy:       uniterParams.HookRetryStrategy,
		newOperationExecutor:    uniterParams.NewOperationExecutor,
		newRemoteRunnerExecutor: uniterParams.NewRemoteRunnerExecutor,
		translateResolverErr:    translateResolverErr,
		observer:                uniterParams.Observer,
		clock:                   uniterParams.Clock,
		downloader:              uniterParams.Downloader,
		applicationChannel:      uniterParams.ApplicationChannel,
		runningStatusChannel:    uniterParams.RunningStatusChannel,
		runningStatusFunc:       uniterParams.RunningStatusFunc,
		runListener:             uniterParams.RunListener,
		rebootQuerier:           uniterParams.RebootQuerier,
	}
	startFunc := func() (worker.Worker, error) {
		plan := catacomb.Plan{
			Site: &u.catacomb,
			Work: func() error {
				return u.loop(uniterParams.UnitTag)
			},
		}
		if u.modelType == model.CAAS {
			// For CAAS models, make sure the leadership tracker is killed when the Uniter
			// dies.
			plan.Init = append(plan.Init, u.leadershipTracker)
		}
		if err := catacomb.Invoke(plan); err != nil {
			return nil, errors.Trace(err)
		}
		return u, nil
	}
	return startFunc
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
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
	logger.Infof("unit %q started", u.unit)

	// Install is a special case, as it must run before there
	// is any remote state, and before the remote state watcher
	// is started.
	var charmURL *corecharm.URL
	var charmModifiedVersion int
	opState := u.operationExecutor.State()
	if opState.Kind == operation.Install {
		logger.Infof("resuming charm install")
		op, err := u.operationFactory.NewInstall(opState.CharmURL)
		if err != nil {
			return errors.Trace(err)
		}
		if err := u.operationExecutor.Run(op, nil); err != nil {
			return errors.Trace(err)
		}
		charmURL = opState.CharmURL
	} else {
		curl, err := u.unit.CharmURL()
		if err != nil {
			return errors.Trace(err)
		}
		charmURL = curl
		app, err := u.unit.Application()
		if err != nil {
			return errors.Trace(err)
		}
		charmModifiedVersion, err = app.CharmModifiedVersion()
		if err != nil {
			return errors.Trace(err)
		}
	}

	var (
		watcher   *remotestate.RemoteStateWatcher
		watcherMu sync.Mutex
	)

	logger.Infof("hooks are retried %v", u.hookRetryStrategy.ShouldRetry)
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
		watcherMu.Lock()
		defer watcherMu.Unlock()

		if watcher != nil {
			// watcher added to catacomb, will kill uniter if there's an error.
			worker.Stop(watcher)
		}
		var err error
		watcher, err = remotestate.NewWatcher(
			remotestate.WatcherConfig{
				State:                remotestate.NewAPIState(u.st),
				LeadershipTracker:    u.leadershipTracker,
				UnitTag:              unitTag,
				UpdateStatusChannel:  u.updateStatusAt,
				CommandChannel:       u.commandChannel,
				RetryHookChannel:     retryHookChan,
				ApplicationChannel:   u.applicationChannel,
				RunningStatusChannel: u.runningStatusChannel,
				RunningStatusFunc:    u.runningStatusFunc,
				ModelType:            u.modelType,
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
		return setAgentStatus(u, status.Idle, "", nil)
	}

	clearResolved := func() error {
		if err := u.unit.ClearResolved(); err != nil {
			return errors.Trace(err)
		}
		watcher.ClearResolvedMode()
		return nil
	}

	// If the machine rebooted and the charm was previously started, inject
	// a start hook to notify charms of the reboot. This logic only makes
	// sense for IAAS workloads as pods in a CAAS scenario can be recycled
	// at any time.
	if u.modelType == model.IAAS {
		machineRebooted, err := u.rebootQuerier.Query(unitTag)
		if err != nil {
			return errors.Annotatef(err, "could not check reboot status for %q", unitTag)
		}
		if opState.Started && machineRebooted {
			logger.Infof("reboot detected; triggering implicit start hook to notify charm")
			op, err := u.operationFactory.NewRunHook(hook.Info{Kind: hooks.Start})
			if err != nil {
				return errors.Trace(err)
			} else if err = u.operationExecutor.Run(op, nil); err != nil {
				return errors.Trace(err)
			}
		}
	}

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
			Actions:             actions.NewResolver(),
			UpgradeSeries:       upgradeseries.NewResolver(),
			Leadership:          uniterleadership.NewResolver(),
			CreatedRelations:    relation.NewCreatedRelationResolver(u.relationStateTracker),
			Relations:           relation.NewRelationResolver(u.relationStateTracker, u.unit),
			Storage:             storage.NewResolver(u.storage, u.modelType),
			Commands: runcommands.NewCommandsResolver(
				u.commands, watcher.CommandCompleted,
			),
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
			UpgradeSeriesStatus:  model.UpgradeSeriesNotStarted,
		}
		for err == nil {
			err = resolver.Loop(resolver.LoopConfig{
				Resolver:      uniterResolver,
				Watcher:       watcher,
				Executor:      u.operationExecutor,
				Factory:       u.operationFactory,
				Abort:         u.catacomb.Dying(),
				OnIdle:        onIdle,
				CharmDirGuard: u.charmDirGuard,
			}, &localState)

			err = u.translateResolverErr(err)

			switch cause := errors.Cause(err); cause {
			case nil:
				// Loop back around.
			case resolver.ErrLoopAborted:
				err = u.catacomb.ErrDying()
			case operation.ErrNeedsReboot:
				err = jworker.ErrRebootMachine
			case operation.ErrHookFailed:
				// Loop back around. The resolver can tell that it is in
				// an error state by inspecting the operation state.
				err = nil
			case resolver.ErrTerminate:
				err = u.terminate()
			case resolver.ErrRestart:
				// make sure we update the two values used above in
				// creating LocalState.
				charmURL = localState.CharmURL
				charmModifiedVersion = localState.CharmModifiedVersion
				// leave err assigned, causing loop to break
			default:
				// We need to set conflicted from here, because error
				// handling is outside of the resolver's control.
				if operation.IsDeployConflictError(cause) {
					localState.Conflicted = true
					err = setAgentStatus(u, status.Error, "upgrade failed", nil)
				} else {
					reportAgentError(u, "resolver loop error", err)
				}
			}
		}

		if errors.Cause(err) != resolver.ErrRestart {
			break
		}
	}

	// If this is a CAAS unit, then dead errors are fairly normal ways to exit
	// the uniter main loop, but the actual agent needs to keep running.
	if errors.Cause(err) == ErrCAASUnitDead {
		err = nil
	}
	if u.runListener != nil {
		u.runListener.UnregisterRunner(u.unit.Name())
	}
	u.localRunListener.UnregisterRunner(u.unit.Name())
	logger.Infof("unit %q shutting down: %s", u.unit, err)
	return err
}

func (u *Uniter) terminate() error {
	unitWatcher, err := u.unit.Watch()
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
			if err := u.unit.Refresh(); err != nil {
				return errors.Trace(err)
			}
			if hasSubs, err := u.unit.HasSubordinates(); err != nil {
				return errors.Trace(err)
			} else if hasSubs {
				continue
			}
			// The unit is known to be Dying; so if it didn't have subordinates
			// just above, it can't acquire new ones before this call.
			if err := u.unit.EnsureDead(); err != nil {
				return errors.Trace(err)
			}
			return u.stopUnitError()
		}
	}
}

// stopUnitError returns the error to use when exiting from stopping the unit.
// For IAAS models, we want to terminate the agent, as each unit is run by
// an individual agent for that unit.
func (u *Uniter) stopUnitError() error {
	logger.Debugf("u.modelType: %s", u.modelType)
	if u.modelType == model.CAAS {
		return ErrCAASUnitDead
	}
	return jworker.ErrTerminateAgent
}

func (u *Uniter) init(unitTag names.UnitTag) (err error) {
	switch u.modelType {
	case model.IAAS, model.CAAS:
		// known types, all good
	default:
		return errors.Errorf("unknown model type %q", u.modelType)
	}
	u.unit, err = u.st.Unit(unitTag)
	if err != nil {
		return err
	}
	if u.unit.Life() == life.Dead {
		// If we started up already dead, we should not progress further. If we
		// become Dead immediately after starting up, we may well complete any
		// operations in progress before detecting it; but that race is fundamental
		// and inescapable, whereas this one is not.
		return u.stopUnitError()
	}
	// If initialising for the first time after deploying, update the status.
	currentStatus, err := u.unit.UnitStatus()
	if err != nil {
		return err
	}
	// TODO(fwereade/wallyworld): we should have an explicit place in the model
	// to tell us when we've hit this point, instead of piggybacking on top of
	// status and/or status history.
	// If the previous status was waiting for machine, we transition to the next step.
	if currentStatus.Status == string(status.Waiting) &&
		(currentStatus.Info == status.MessageWaitForMachine || currentStatus.Info == status.MessageInstallingAgent) {
		if err := u.unit.SetUnitStatus(status.Waiting, status.MessageInitializingAgent, nil); err != nil {
			return errors.Trace(err)
		}
	}
	if err := tools.EnsureSymlinks(u.paths.ToolsDir, u.paths.ToolsDir, jujuc.CommandNames()); err != nil {
		return err
	}
	relStateTracker, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                u.st,
			Unit:                 u.unit,
			Tracker:              u.leadershipTracker,
			NewLeadershipContext: context.NewLeadershipContext,
			CharmDir:             u.paths.State.CharmDir,
			Abort:                u.catacomb.Dying(),
		})
	if err != nil {
		return errors.Annotatef(err, "cannot create relation state tracker")
	}
	u.relationStateTracker = relStateTracker
	u.commands = runcommands.NewCommands()
	u.commandChannel = make(chan string)

	storageAttachments, err := storage.NewAttachments(
		u.st, unitTag, u.unit, u.catacomb.Dying(),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create storage hook source")
	}
	u.storage = storageAttachments

	// Only IAAS models require the uniter to install charms.
	// For CAAS models this is done by the operator.
	var deployer charm.Deployer
	if u.modelType == model.IAAS {
		if err := charm.ClearDownloads(u.paths.State.BundlesDir); err != nil {
			logger.Warningf(err.Error())
		}
		deployer, err = charm.NewDeployer(
			u.paths.State.CharmDir,
			u.paths.State.DeployerDir,
			charm.NewBundlesDir(u.paths.State.BundlesDir, u.downloader),
		)
		if err != nil {
			return errors.Annotatef(err, "cannot create deployer")
		}
	}
	contextFactory, err := context.NewContextFactory(context.FactoryConfig{
		State:            u.st,
		Unit:             u.unit,
		Tracker:          u.leadershipTracker,
		GetRelationInfos: u.relationStateTracker.GetInfo,
		Storage:          u.storage,
		Paths:            u.paths,
		Clock:            u.clock,
	})
	if err != nil {
		return err
	}
	var remoteExecutor runner.ExecFunc
	if u.newRemoteRunnerExecutor != nil {
		remoteExecutor = u.newRemoteRunnerExecutor(u.unit, u.paths)
	}
	runnerFactory, err := runner.NewFactory(
		u.st, u.paths, contextFactory, remoteExecutor,
	)
	if err != nil {
		return errors.Trace(err)
	}
	u.operationFactory = operation.NewFactory(operation.FactoryParams{
		Deployer:       deployer,
		RunnerFactory:  runnerFactory,
		Callbacks:      &operationCallbacks{u},
		Abort:          u.catacomb.Dying(),
		MetricSpoolDir: u.paths.GetMetricsSpoolDir(),
	})

	charmURL, err := u.getApplicationCharmURL()
	if err != nil {
		return errors.Trace(err)
	}

	initialState := operation.State{
		Kind:     operation.Install,
		Step:     operation.Queued,
		CharmURL: charmURL,
	}

	if u.modelType == model.CAAS {
		// For CAAS, run the install hook, but not the
		// full install operation.
		initialState = operation.State{
			Hook: &hook.Info{Kind: hooks.Install},
			Kind: operation.RunHook,
			Step: operation.Queued,
		}
		if err := u.unit.SetCharmURL(charmURL); err != nil {
			return errors.Trace(err)
		}
	}

	operationExecutor, err := u.newOperationExecutor(operation.ExecutorConfig{
		StateReadWriter: u.unit,
		InitialState:    initialState,
		AcquireLock:     u.acquireExecutionLock,
	})
	if err != nil {
		return errors.Trace(err)
	}
	u.operationExecutor = operationExecutor

	// Ensure we have an agent directory to to write the socket.
	if err := os.MkdirAll(u.paths.State.BaseDir, 0755); err != nil {
		return errors.Trace(err)
	}
	socket := u.paths.Runtime.LocalJujuRunSocket.Server
	logger.Debugf("starting local juju-run listener on %v", socket)
	u.localRunListener, err = NewRunListener(socket)
	if err != nil {
		return errors.Annotate(err, "creating juju run listener")
	}
	rlw := NewRunListenerWrapper(u.localRunListener)
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
	return nil
}

func (u *Uniter) Kill() {
	u.catacomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.catacomb.Wait()
}

func (u *Uniter) getApplicationCharmURL() (*corecharm.URL, error) {
	// TODO(fwereade): pretty sure there's no reason to make 2 API calls here.
	app, err := u.st.Application(u.unit.ApplicationTag())
	if err != nil {
		return nil, err
	}
	charmURL, _, err := app.CharmURL()
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
func (u *Uniter) acquireExecutionLock(action string) (func(), error) {
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's catacomb into account.
	spec := machinelock.Spec{
		Cancel:  u.catacomb.Dying(),
		Worker:  "uniter",
		Comment: action,
	}
	releaser, err := u.hookLock.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return releaser, nil
}

func (u *Uniter) reportHookError(hookInfo hook.Info) error {
	// Set the agent status to "error". We must do this here in case the
	// hook is interrupted (e.g. unit agent crashes), rather than immediately
	// after attempting a runHookOp.
	hookName := string(hookInfo.Kind)
	statusData := map[string]interface{}{}
	if hookInfo.Kind.IsRelation() {
		statusData["relation-id"] = hookInfo.RelationId
		if hookInfo.RemoteUnit != "" {
			statusData["remote-unit"] = hookInfo.RemoteUnit
		}
		relationName, err := u.relationStateTracker.Name(hookInfo.RelationId)
		if err != nil {
			return errors.Trace(err)
		}
		hookName = fmt.Sprintf("%s-%s", relationName, hookInfo.Kind)
	}
	statusData["hook"] = hookName
	statusMessage := fmt.Sprintf("hook failed: %q", hookName)
	return setAgentStatus(u, status.Error, statusMessage, statusData)
}
