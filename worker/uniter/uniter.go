// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/exec"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/status"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
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
	jujuos "github.com/juju/utils/os"
)

var logger = loggo.GetLogger("juju.worker.uniter")

// A UniterExecutionObserver gets the appropriate methods called when a hook
// is executed and either succeeds or fails.  Missing hooks don't get reported
// in this way.
type UniterExecutionObserver interface {
	HookCompleted(hookName string)
	HookFailed(hookName string)
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
	relations relation.Relations
	storage   *storage.Attachments
	clock     clock.Clock

	// Cache the last reported status information
	// so we don't make unnecessary api calls.
	setStatusMutex      sync.Mutex
	lastReportedStatus  status.Status
	lastReportedMessage string

	operationFactory     operation.Factory
	operationExecutor    operation.Executor
	newOperationExecutor NewExecutorFunc

	leadershipTracker leadership.Tracker
	charmDirGuard     fortress.Guard

	hookLockName string

	// TODO(axw) move the runListener and run-command code outside of the
	// uniter, and introduce a separate worker. Each worker would feed
	// operations to a single, synchronized runner to execute.
	runListener    *RunListener
	commands       runcommands.Commands
	commandChannel chan string

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// updateStatusAt defines a function that will be used to generate signals for
	// the update-status hook
	updateStatusAt func() <-chan time.Time

	// hookRetryStrategy represents configuration for hook retries
	hookRetryStrategy params.RetryStrategy

	// downloader is the downloader that should be used to get the charm
	// archive.
	downloader charm.Downloader
}

// UniterParams hold all the necessary parameters for a new Uniter.
type UniterParams struct {
	UniterFacade         *uniter.State
	UnitTag              names.UnitTag
	LeadershipTracker    leadership.Tracker
	DataDir              string
	Downloader           charm.Downloader
	MachineLockName      string
	CharmDirGuard        fortress.Guard
	UpdateStatusSignal   func() <-chan time.Time
	HookRetryStrategy    params.RetryStrategy
	NewOperationExecutor NewExecutorFunc
	Clock                clock.Clock
	// TODO (mattyw, wallyworld, fwereade) Having the observer here make this approach a bit more legitimate, but it isn't.
	// the observer is only a stop gap to be used in tests. A better approach would be to have the uniter tests start hooks
	// that write to files, and have the tests watch the output to know that hooks have finished.
	Observer UniterExecutionObserver
}

type NewExecutorFunc func(string, func() (*corecharm.URL, error), func() (mutex.Releaser, error)) (operation.Executor, error)

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(uniterParams *UniterParams) (*Uniter, error) {
	u := &Uniter{
		st:                   uniterParams.UniterFacade,
		paths:                NewPaths(uniterParams.DataDir, uniterParams.UnitTag),
		hookLockName:         uniterParams.MachineLockName,
		leadershipTracker:    uniterParams.LeadershipTracker,
		charmDirGuard:        uniterParams.CharmDirGuard,
		updateStatusAt:       uniterParams.UpdateStatusSignal,
		hookRetryStrategy:    uniterParams.HookRetryStrategy,
		newOperationExecutor: uniterParams.NewOperationExecutor,
		observer:             uniterParams.Observer,
		clock:                uniterParams.Clock,
		downloader:           uniterParams.Downloader,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: func() error {
			return u.loop(uniterParams.UnitTag)
		},
	})
	return u, errors.Trace(err)
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
		if err == worker.ErrTerminateAgent {
			return err
		}
		return errors.Annotatef(err, "failed to initialize uniter for %q", unitTag)
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
		if err := u.operationExecutor.Run(op); err != nil {
			return errors.Trace(err)
		}
		charmURL = opState.CharmURL
	} else {
		curl, err := u.unit.CharmURL()
		if err != nil {
			return errors.Trace(err)
		}
		charmURL = curl
		svc, err := u.unit.Application()
		if err != nil {
			return errors.Trace(err)
		}
		charmModifiedVersion, err = svc.CharmModifiedVersion()
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
				State:               remotestate.NewAPIState(u.st),
				LeadershipTracker:   u.leadershipTracker,
				UnitTag:             unitTag,
				UpdateStatusChannel: u.updateStatusAt,
				CommandChannel:      u.commandChannel,
				RetryHookChannel:    retryHookChan,
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
		return setAgentStatus(u, status.StatusIdle, "", nil)
	}

	clearResolved := func() error {
		if err := u.unit.ClearResolved(); err != nil {
			return errors.Trace(err)
		}
		watcher.ClearResolvedMode()
		return nil
	}

	for {
		if err = restartWatcher(); err != nil {
			err = errors.Annotate(err, "(re)starting watcher")
			break
		}

		uniterResolver := NewUniterResolver(ResolverConfig{
			ClearResolved:       clearResolved,
			ReportHookError:     u.reportHookError,
			ShouldRetryHooks:    u.hookRetryStrategy.ShouldRetry,
			StartRetryHookTimer: retryHookTimer.Start,
			StopRetryHookTimer:  retryHookTimer.Reset,
			Actions:             actions.NewResolver(),
			Leadership:          uniterleadership.NewResolver(),
			Relations:           relation.NewRelationsResolver(u.relations),
			Storage:             storage.NewResolver(u.storage),
			Commands: runcommands.NewCommandsResolver(
				u.commands, watcher.CommandCompleted,
			),
		})

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
			err = resolver.Loop(resolver.LoopConfig{
				Resolver:      uniterResolver,
				Watcher:       watcher,
				Executor:      u.operationExecutor,
				Factory:       u.operationFactory,
				Abort:         u.catacomb.Dying(),
				OnIdle:        onIdle,
				CharmDirGuard: u.charmDirGuard,
			}, &localState)
			switch cause := errors.Cause(err); cause {
			case nil:
				// Loop back around.
			case resolver.ErrLoopAborted:
				err = u.catacomb.ErrDying()
			case operation.ErrNeedsReboot:
				err = worker.ErrRebootMachine
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
					err = setAgentStatus(u, status.StatusError, "upgrade failed", nil)
				} else {
					reportAgentError(u, "resolver loop error", err)
				}
			}
		}

		if errors.Cause(err) != resolver.ErrRestart {
			break
		}
	}

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
			return worker.ErrTerminateAgent
		}
	}
}

func (u *Uniter) init(unitTag names.UnitTag) (err error) {
	u.unit, err = u.st.Unit(unitTag)
	if err != nil {
		return err
	}
	if u.unit.Life() == params.Dead {
		// If we started up already dead, we should not progress further. If we
		// become Dead immediately after starting up, we may well complete any
		// operations in progress before detecting it; but that race is fundamental
		// and inescapable, whereas this one is not.
		return worker.ErrTerminateAgent
	}
	if err := jujuc.EnsureSymlinks(u.paths.ToolsDir); err != nil {
		return err
	}
	if err := os.MkdirAll(u.paths.State.RelationsDir, 0755); err != nil {
		return errors.Trace(err)
	}
	relations, err := relation.NewRelations(
		u.st, unitTag, u.paths.State.CharmDir,
		u.paths.State.RelationsDir, u.catacomb.Dying(),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create relations")
	}
	u.relations = relations
	storageAttachments, err := storage.NewAttachments(
		u.st, unitTag, u.paths.State.StorageDir, u.catacomb.Dying(),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create storage hook source")
	}
	u.storage = storageAttachments
	u.commands = runcommands.NewCommands()
	u.commandChannel = make(chan string)

	deployer, err := charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir, u.downloader),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create deployer")
	}
	contextFactory, err := context.NewContextFactory(
		u.st, unitTag, u.leadershipTracker, u.relations.GetInfo, u.storage, u.paths, u.clock,
	)
	if err != nil {
		return err
	}
	runnerFactory, err := runner.NewFactory(
		u.st, u.paths, contextFactory,
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

	operationExecutor, err := u.newOperationExecutor(u.paths.State.OperationsFile, u.getServiceCharmURL, u.acquireExecutionLock)
	if err != nil {
		return errors.Trace(err)
	}
	u.operationExecutor = operationExecutor

	logger.Debugf("starting juju-run listener on unix:%s", u.paths.Runtime.JujuRunSocket)
	commandRunner, err := NewChannelCommandRunner(ChannelCommandRunnerConfig{
		Abort:          u.catacomb.Dying(),
		Commands:       u.commands,
		CommandChannel: u.commandChannel,
	})
	if err != nil {
		return errors.Annotate(err, "creating command runner")
	}
	u.runListener, err = NewRunListener(RunListenerConfig{
		SocketPath:    u.paths.Runtime.JujuRunSocket,
		CommandRunner: commandRunner,
	})
	if err != nil {
		return errors.Trace(err)
	}
	rlw := newRunListenerWrapper(u.runListener)
	if err := u.catacomb.Add(rlw); err != nil {
		return errors.Trace(err)
	}
	// The socket needs to have permissions 777 in order for other users to use it.
	if jujuos.HostOS() != jujuos.Windows {
		return os.Chmod(u.paths.Runtime.JujuRunSocket, 0777)
	}
	return nil
}

func (u *Uniter) Kill() {
	u.catacomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.catacomb.Wait()
}

func (u *Uniter) getServiceCharmURL() (*corecharm.URL, error) {
	// TODO(fwereade): pretty sure there's no reason to make 2 API calls here.
	service, err := u.st.Application(u.unit.ApplicationTag())
	if err != nil {
		return nil, err
	}
	charmURL, _, err := service.CharmURL()
	return charmURL, err
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	// TODO(axw) drop this when we move the run-listener to an independent
	// worker. This exists purely for the tests.
	return u.runListener.RunCommands(args)
}

// acquireExecutionLock acquires the machine-level execution lock, and
// returns a func that must be called to unlock it. It's used by operation.Executor
// when running operations that execute external code.
func (u *Uniter) acquireExecutionLock() (mutex.Releaser, error) {
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's catacomb into account.
	spec := mutex.Spec{
		Name:   u.hookLockName,
		Clock:  u.clock,
		Delay:  250 * time.Millisecond,
		Cancel: u.catacomb.Dying(),
	}
	logger.Debugf("acquire lock %q for uniter hook execution", u.hookLockName)
	releaser, err := mutex.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("lock %q acquired", u.hookLockName)
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
		relationName, err := u.relations.Name(hookInfo.RelationId)
		if err != nil {
			return errors.Trace(err)
		}
		hookName = fmt.Sprintf("%s-%s", relationName, hookInfo.Kind)
	}
	statusData["hook"] = hookName
	statusMessage := fmt.Sprintf("hook failed: %q", hookName)
	return setAgentStatus(u, status.StatusError, statusMessage, statusData)
}
