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
	corecharm "gopkg.in/juju/charm.v5"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/leadership"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/filter"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/uniter/storage"
)

var logger = loggo.GetLogger("juju.worker.uniter")

// leadershipGuarantee defines the period of time for which a successful call
// to the is-leader hook tool guarantees continued leadership.
var leadershipGuarantee = 30 * time.Second

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
	tomb      tomb.Tomb
	st        *uniter.State
	paths     Paths
	f         filter.Filter
	unit      *uniter.Unit
	relations Relations
	cleanups  []cleanup
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

	hookLock    *fslock.Lock
	runListener *RunListener

	ranLeaderSettingsChanged bool
	ranConfigChanged         bool

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// metricsTimerChooser is a struct that allows metrics to switch between
	// active and inactive timers.
	metricsTimerChooser *timerChooser

	// collectMetricsAt defines a function that will be used to generate signals
	// for the collect-metrics hook.
	collectMetricsAt TimedSignal

	// sendMetricsAt defines a function that will be used to generate signals
	// to send metrics to the state server.
	sendMetricsAt TimedSignal

	// updateStatusAt defines a function that will be used to generate signals for
	// the update-status hook
	updateStatusAt TimedSignal
}

// UniterParams hold all the necessary parameters for a new Uniter.
type UniterParams struct {
	UniterFacade         *uniter.State
	UnitTag              names.UnitTag
	LeadershipTracker    leadership.Tracker
	DataDir              string
	MachineLock          *fslock.Lock
	MetricsTimerChooser  *timerChooser
	UpdateStatusSignal   TimedSignal
	NewOperationExecutor NewExecutorFunc
}

type NewExecutorFunc func(string, func() (*corecharm.URL, error), func(string) (func() error, error)) (operation.Executor, error)

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(uniterParams *UniterParams) *Uniter {
	u := &Uniter{
		st:                   uniterParams.UniterFacade,
		paths:                NewPaths(uniterParams.DataDir, uniterParams.UnitTag),
		hookLock:             uniterParams.MachineLock,
		leadershipTracker:    uniterParams.LeadershipTracker,
		metricsTimerChooser:  uniterParams.MetricsTimerChooser,
		collectMetricsAt:     uniterParams.MetricsTimerChooser.inactive,
		sendMetricsAt:        uniterParams.MetricsTimerChooser.inactive,
		updateStatusAt:       uniterParams.UpdateStatusSignal,
		newOperationExecutor: uniterParams.NewOperationExecutor,
	}
	go func() {
		defer u.tomb.Done()
		defer u.runCleanups()
		u.tomb.Kill(u.loop(uniterParams.UnitTag))
	}()
	return u
}

type cleanup func() error

func (u *Uniter) addCleanup(cleanup cleanup) {
	u.cleanups = append(u.cleanups, cleanup)
}

func (u *Uniter) runCleanups() {
	for _, cleanup := range u.cleanups {
		u.tomb.Kill(cleanup())
	}
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
		if err == worker.ErrTerminateAgent {
			return err
		}
		return fmt.Errorf("failed to initialize uniter for %q: %v", unitTag, err)
	}
	logger.Infof("unit %q started", u.unit)

	// Start filtering state change events for consumption by modes.
	u.f, err = filter.NewFilter(u.st, unitTag)
	if err != nil {
		return err
	}
	u.addCleanup(u.f.Stop)

	// Stop the uniter if the filter fails.
	go func() { u.tomb.Kill(u.f.Wait()) }()

	// Start handling leader settings events, or not, as appropriate.
	u.f.WantLeaderSettingsEvents(!u.operationState().Leader)

	// Run modes until we encounter an error.
	mode := ModeContinue
	for err == nil {
		select {
		case <-u.tomb.Dying():
			err = tomb.ErrDying
		default:
			mode, err = mode(u)
			switch cause := errors.Cause(err); cause {
			case operation.ErrNeedsReboot:
				err = worker.ErrRebootMachine
			case tomb.ErrDying, worker.ErrTerminateAgent:
				err = cause
			case operation.ErrHookFailed:
				mode, err = ModeHookError, nil
			default:
				charmURL, ok := operation.DeployConflictCharmURL(cause)
				if ok {
					mode, err = ModeConflicted(charmURL), nil
				}
			}
		}
	}

	logger.Infof("unit %q shutting down: %s", u.unit, err)
	return err
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
	relations, err := newRelations(u.st, unitTag, u.paths, u.tomb.Dying())
	if err != nil {
		return errors.Annotatef(err, "cannot create relations")
	}
	u.relations = relations
	storageAttachments, err := storage.NewAttachments(
		u.st, unitTag, u.paths.State.StorageDir, u.tomb.Dying(),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create storage hook source")
	}
	u.storage = storageAttachments
	u.addCleanup(storageAttachments.Stop)

	deployer, err := charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create deployer")
	}
	u.deployer = &deployerProxy{deployer}
	contextFactory, err := runner.NewContextFactory(
		u.st, unitTag, u.leadershipTracker, u.relations.GetInfo, u.storage, u.paths,
	)
	if err != nil {
		return err
	}
	runnerFactory, err := runner.NewFactory(
		u.st, u.paths, contextFactory,
	)
	if err != nil {
		return err
	}
	u.operationFactory = operation.NewFactory(operation.FactoryParams{
		Deployer:       u.deployer,
		RunnerFactory:  runnerFactory,
		Callbacks:      &operationCallbacks{u},
		StorageUpdater: u.storage,
		Abort:          u.tomb.Dying(),
		MetricSender:   u.unit,
		MetricSpoolDir: u.paths.GetMetricsSpoolDir(),
	})

	operationExecutor, err := u.newOperationExecutor(u.paths.State.OperationsFile, u.getServiceCharmURL, u.acquireExecutionLock)
	if err != nil {
		return err
	}
	u.operationExecutor = operationExecutor

	logger.Debugf("starting juju-run listener on unix:%s", u.paths.Runtime.JujuRunSocket)
	u.runListener, err = NewRunListener(u, u.paths.Runtime.JujuRunSocket)
	if err != nil {
		return err
	}
	u.addCleanup(func() error {
		// TODO(fwereade): RunListener returns no error on Close. This seems wrong.
		u.runListener.Close()
		return nil
	})
	// The socket needs to have permissions 777 in order for other users to use it.
	if version.Current.OS != version.Windows {
		return os.Chmod(u.paths.Runtime.JujuRunSocket, 0777)
	}
	return nil
}

func (u *Uniter) Kill() {
	u.tomb.Kill(nil)
}

func (u *Uniter) Wait() error {
	return u.tomb.Wait()
}

func (u *Uniter) Stop() error {
	u.tomb.Kill(nil)
	return u.Wait()
}

func (u *Uniter) Dead() <-chan struct{} {
	return u.tomb.Dead()
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

// initializeMetricsTimers enables the periodic collect-metrics hook
// and periodic sending of collected metrics for charms that declare metrics.
func (u *Uniter) initializeMetricsTimers() error {
	charm, err := corecharm.ReadCharmDir(u.paths.State.CharmDir)
	if err != nil {
		return err
	}
	u.collectMetricsAt = u.metricsTimerChooser.getCollectMetricsTimer(charm)
	u.sendMetricsAt = u.metricsTimerChooser.getSendMetricsTimer(charm)
	return nil
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
	err = u.runOperation(newCommandsOp(commandArgs, sendResponse))
	if err == nil {
		select {
		case response := <-responseChan:
			results, err = response.response, response.err
		default:
			err = errors.New("command response never sent")
		}
	}
	if errors.Cause(err) == operation.ErrNeedsReboot {
		u.tomb.Kill(worker.ErrRebootMachine)
		err = nil
	}
	if err != nil {
		u.tomb.Kill(err)
	}
	return results, err
}

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
	before := u.operationState()
	defer func() {
		// Check that if we lose leadership as a result of this
		// operation, we want to start getting leader settings events,
		// or if we gain leadership we want to stop receiving those
		// events.
		if after := u.operationState(); before.Leader != after.Leader {
			u.f.WantLeaderSettingsEvents(before.Leader)
		}
	}()
	return u.operationExecutor.Run(op)
}

// acquireExecutionLock acquires the machine-level execution lock, and
// returns a func that must be called to unlock it. It's used by operation.Executor
// when running operations that execute external code.
func (u *Uniter) acquireExecutionLock(message string) (func() error, error) {
	// We want to make sure we don't block forever when locking, but take the
	// Uniter's tomb into account.
	checkTomb := func() error {
		select {
		case <-u.tomb.Dying():
			return tomb.ErrDying
		default:
			return nil
		}
	}
	message = fmt.Sprintf("%s: %s", u.unit.Name(), message)
	if err := u.hookLock.LockWithFunc(message, checkTomb); err != nil {
		return nil, err
	}
	return func() error { return u.hookLock.Unlock() }, nil
}
