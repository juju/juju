// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/charmdir"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/actions"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/hook"
	uniterleadership "github.com/juju/juju/worker/uniter/leadership"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
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

	// Cache the last reported status information
	// so we don't make unnecessary api calls.
	setStatusMutex      sync.Mutex
	lastReportedStatus  params.Status
	lastReportedMessage string

	deployer             *deployerProxy
	operationFactory     operation.Factory
	operationExecutor    operation.Executor
	newOperationExecutor NewExecutorFunc

	leadershipTracker leadership.Tracker
	charmDirLocker    charmdir.Locker

	hookLock    *fslock.Lock
	runListener *RunListener

	ranConfigChanged bool

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// updateStatusAt defines a function that will be used to generate signals for
	// the update-status hook
	updateStatusAt func() <-chan time.Time
}

// UniterParams hold all the necessary parameters for a new Uniter.
type UniterParams struct {
	UniterFacade         *uniter.State
	UnitTag              names.UnitTag
	LeadershipTracker    leadership.Tracker
	DataDir              string
	MachineLock          *fslock.Lock
	CharmDirLocker       charmdir.Locker
	UpdateStatusSignal   func() <-chan time.Time
	NewOperationExecutor NewExecutorFunc
	// TODO (mattyw, wallyworld, fwereade) Having the observer here make this approach a bit more legitimate, but it isn't.
	// the observer is only a stop gap to be used in tests. A better approach would be to have the uniter tests start hooks
	// that write to files, and have the tests watch the output to know that hooks have finished.
	Observer UniterExecutionObserver
}

type NewExecutorFunc func(string, func() (*corecharm.URL, error), func(string) (func() error, error)) (operation.Executor, error)

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(uniterParams *UniterParams) (*Uniter, error) {
	u := &Uniter{
		st:                   uniterParams.UniterFacade,
		paths:                NewPaths(uniterParams.DataDir, uniterParams.UnitTag),
		hookLock:             uniterParams.MachineLock,
		leadershipTracker:    uniterParams.LeadershipTracker,
		charmDirLocker:       uniterParams.CharmDirLocker,
		updateStatusAt:       uniterParams.UpdateStatusSignal,
		newOperationExecutor: uniterParams.NewOperationExecutor,
		observer:             uniterParams.Observer,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &u.catacomb,
		Work: func() error {
			return u.loop(uniterParams.UnitTag)
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return u, nil
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
		if err == worker.ErrTerminateAgent {
			return err
		}
		return fmt.Errorf("failed to initialize uniter for %q: %v", unitTag, err)
	}
	logger.Infof("unit %q started", u.unit)

	// Install is a special case, as it must run before there
	// is any remote state, and before the remote state watcher
	// is started.
	var charmURL *corecharm.URL
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
	}

	var (
		watcher   *remotestate.RemoteStateWatcher
		watcherMu sync.Mutex
	)

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
		return setAgentStatus(u, params.StatusIdle, "", nil)
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

		uniterResolver := &uniterResolver{
			clearResolved:      clearResolved,
			reportHookError:    u.reportHookError,
			fixDeployer:        u.deployer.Fix,
			actionsResolver:    actions.NewResolver(),
			leadershipResolver: uniterleadership.NewResolver(),
			relationsResolver:  relation.NewRelationsResolver(u.relations),
			storageResolver:    storage.NewResolver(u.storage),
		}

		// We should not do anything until there has been a change
		// to the remote state. The watcher will trigger at least
		// once initially.
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		case <-watcher.RemoteStateChanged():
		}

		localState := resolver.LocalState{CharmURL: charmURL}
		for err == nil {
			err = resolver.Loop(resolver.LoopConfig{
				Resolver:       uniterResolver,
				Watcher:        watcher,
				Executor:       u.operationExecutor,
				Factory:        u.operationFactory,
				Abort:          u.catacomb.Dying(),
				OnIdle:         onIdle,
				CharmDirLocker: u.charmDirLocker,
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
				charmURL = localState.CharmURL
				// leave err assigned, causing loop to break
			default:
				// We need to set conflicted from here, because error
				// handling is outside of the resolver's control.
				if operation.IsDeployConflictError(cause) {
					localState.Conflicted = true
					err = setAgentStatus(u, params.StatusError, "upgrade failed", nil)
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

func (u *Uniter) setupLocks() (err error) {
	if message := u.hookLock.Message(); u.hookLock.IsLocked() && message != "" {
		// Look to see if it was us that held the lock before.  If it was, we
		// should be safe enough to break it, as it is likely that we died
		// before unlocking, and have been restarted by the init system.
		parts := strings.SplitN(message, ":", 2)
		if len(parts) > 1 && parts[0] == u.unit.Name() {
			if err := u.hookLock.BreakLock(); err != nil {
				return err
			}
		}
	}
	return nil
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
	if err = u.setupLocks(); err != nil {
		return err
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

	deployer, err := charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create deployer")
	}
	u.deployer = &deployerProxy{deployer}
	contextFactory, err := context.NewContextFactory(
		u.st, unitTag, u.leadershipTracker, u.relations.GetInfo, u.storage, u.paths,
	)
	if err != nil {
		return errors.Trace(err)
	}
	runnerFactory, err := runner.NewFactory(
		u.st, u.paths, contextFactory,
	)
	if err != nil {
		return errors.Trace(err)
	}
	u.operationFactory = operation.NewFactory(operation.FactoryParams{
		Deployer:       u.deployer,
		RunnerFactory:  runnerFactory,
		Callbacks:      &operationCallbacks{u},
		StorageUpdater: u.storage,
		Abort:          u.catacomb.Dying(),
		MetricSpoolDir: u.paths.GetMetricsSpoolDir(),
	})

	operationExecutor, err := u.newOperationExecutor(u.paths.State.OperationsFile, u.getServiceCharmURL, u.acquireExecutionLock)
	if err != nil {
		return errors.Trace(err)
	}
	u.operationExecutor = operationExecutor

	logger.Debugf("starting juju-run listener on unix:%s", u.paths.Runtime.JujuRunSocket)
	u.runListener, err = NewRunListener(u, u.paths.Runtime.JujuRunSocket)
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

// Kill is part of the worker.Worker interface.
func (u *Uniter) Kill() {
	u.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (u *Uniter) Wait() error {
	return u.catacomb.Wait()
}

func (u *Uniter) Dead() <-chan struct{} {
	// TODO(fwereade): do we really need this? tests that use it could
	// probably just use a:
	//
	//     go func() {
	//         done <- u.Wait()
	//     }()
	//
	// ...construction or similar.
	return u.catacomb.Dead()
}

func (u *Uniter) getServiceCharmURL() (*corecharm.URL, error) {
	// TODO(fwereade): pretty sure there's no reason to make 2 API calls here.
	service, err := u.st.Service(u.unit.ServiceTag())
	if err != nil {
		return nil, err
	}
	charmURL, _, err := service.CharmURL()
	return charmURL, err
}

func (u *Uniter) operationState() operation.State {
	return u.operationExecutor.State()
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	// TODO(fwereade): this is *still* all sorts of messed-up and not especially
	// goroutine-safe, but that's not what I'm fixing at the moment. We could
	// address this by:
	//  1) implementing an operation to encapsulate the relations.Update call
	//  2) (quick+dirty) mutex runOperation until we can
	//  3) (correct) feed RunCommands requests into the mode funcs (or any queue
	//     that replaces them) such that they're handled and prioritised like
	//     every other operation.
	logger.Tracef("run commands: %s", args.Commands)

	type responseInfo struct {
		response *exec.ExecResponse
		err      error
	}
	responseChan := make(chan responseInfo, 1)
	sendResponse := func(response *exec.ExecResponse, err error) {
		responseChan <- responseInfo{response, err}
	}

	commandArgs := operation.CommandArgs{
		Commands:        args.Commands,
		RelationId:      args.RelationId,
		RemoteUnitName:  args.RemoteUnitName,
		ForceRemoteUnit: args.ForceRemoteUnit,
	}
	err = u.runOperation(func(f operation.Factory) (operation.Operation, error) {
		return f.NewCommands(commandArgs, sendResponse)
	})
	if err == nil {
		select {
		case response := <-responseChan:
			results, err = response.response, response.err
		default:
			err = errors.New("command response never sent")
		}
	}
	if errors.Cause(err) == operation.ErrNeedsReboot {
		u.catacomb.Kill(worker.ErrRebootMachine)
		err = nil
	}
	if err != nil {
		u.catacomb.Kill(err)
	}
	return results, err
}

// creator exists primarily to make the implementation of the Mode funcs more
// readable -- the general pattern is to switch to get a creator func (which
// doesn't allow for the possibility of error) and then to pass the chosen
// creator down to runOperation (which can then consistently create and run
// all the operations in the same way).
type creator func(factory operation.Factory) (operation.Operation, error)

// runOperation uses the uniter's operation factory to run the supplied creation
// func, and then runs the resulting operation.
//
// This has a number of advantages over having mode funcs use the factory and
// executor directly:
//   * it cuts down on duplicated code in the mode funcs, making the logic easier
//     to parse
//   * it narrows the (conceptual) interface exposed to the mode funcs -- one day
//     we might even be able to use a (real) interface and maybe even approach a
//     point where we can run direct unit tests(!) on the modes themselves.
//   * it opens a path to fixing RunCommands -- all operation creation and
//     execution is done in a single place, and it's much easier to force those
//     onto a single thread.
//       * this can't be done quite yet, though, because relation changes are
//         not yet encapsulated in operations, and that needs to happen before
//         RunCommands will *actually* be goroutine-safe.
func (u *Uniter) runOperation(creator creator) (err error) {
	errorMessage := "creating operation to run"
	defer func() {
		reportAgentError(u, errorMessage, err)
	}()
	op, err := creator(u.operationFactory)
	if err != nil {
		return errors.Annotatef(err, "cannot create operation")
	}
	errorMessage = op.String()
	return u.operationExecutor.Run(op)
}

// acquireExecutionLock acquires the machine-level execution lock, and
// returns a func that must be called to unlock it. It's used by operation.Executor
// when running operations that execute external code.
func (u *Uniter) acquireExecutionLock(message string) (func() error, error) {
	logger.Debugf("lock: %v", message)
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's catacomb into account.
	checkTomb := func() error {
		select {
		case <-u.catacomb.Dying():
			return u.catacomb.ErrDying()
		default:
			return nil
		}
	}
	message = fmt.Sprintf("%s: %s", u.unit.Name(), message)
	if err := u.hookLock.LockWithFunc(message, checkTomb); err != nil {
		return nil, err
	}
	return func() error {
		logger.Debugf("unlock: %v", message)
		return u.hookLock.Unlock()
	}, nil
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
	return setAgentStatus(u, params.StatusError, statusMessage, statusData)
}
