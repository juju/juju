// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/fslock"
	corecharm "gopkg.in/juju/charm.v4"
	"launchpad.net/tomb"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/charm"
	"github.com/juju/juju/worker/uniter/filter"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
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
	tomb      tomb.Tomb
	st        *uniter.State
	paths     Paths
	f         filter.Filter
	unit      *uniter.Unit
	relations Relations

	deployer          *deployerProxy
	operationFactory  operation.Factory
	operationExecutor operation.Executor

	hookLock    *fslock.Lock
	runListener *RunListener

	ranConfigChanged bool

	// The execution observer is only used in tests at this stage. Should this
	// need to be extended, perhaps a list of observers would be needed.
	observer UniterExecutionObserver

	// collectMetricsAt defines a function that will be used to generate signals
	// for the collect-metrics hook.
	collectMetricsAt CollectMetricsSignal
}

// NewUniter creates a new Uniter which will install, run, and upgrade
// a charm on behalf of the unit with the given unitTag, by executing
// hooks and operations provoked by changes in st.
func NewUniter(st *uniter.State, unitTag names.UnitTag, dataDir string, hookLock *fslock.Lock) *Uniter {
	u := &Uniter{
		st:               st,
		paths:            NewPaths(dataDir, unitTag),
		hookLock:         hookLock,
		collectMetricsAt: inactiveMetricsTimer,
	}
	go func() {
		defer u.tomb.Done()
		u.tomb.Kill(u.loop(unitTag))
	}()
	return u
}

func (u *Uniter) loop(unitTag names.UnitTag) (err error) {
	if err := u.init(unitTag); err != nil {
		if err == worker.ErrTerminateAgent {
			return err
		}
		return fmt.Errorf("failed to initialize uniter for %q: %v", unitTag, err)
	}
	defer u.runListener.Close()
	logger.Infof("unit %q started", u.unit)

	// Start filtering state change events for consumption by modes.
	u.f, err = filter.NewFilter(u.st, unitTag)
	if err != nil {
		return err
	}
	defer watcher.Stop(u.f, &u.tomb)
	go func() {
		u.tomb.Kill(u.f.Wait())
	}()

	// Run modes until we encounter an error.
	mode := ModeContinue
	for err == nil {
		select {
		case <-u.tomb.Dying():
			err = tomb.ErrDying
		default:
			mode, err = mode(u)
			switch cause := errors.Cause(err); cause {
			case operation.ErrHookFailed:
				mode, err = ModeHookError, nil
			case operation.ErrNeedsReboot:
				err = worker.ErrRebootMachine
			case tomb.ErrDying, worker.ErrTerminateAgent:
				err = cause
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
		// before unlocking, and have been restarted by upstart.
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

	deployer, err := charm.NewDeployer(
		u.paths.State.CharmDir,
		u.paths.State.DeployerDir,
		charm.NewBundlesDir(u.paths.State.BundlesDir),
	)
	if err != nil {
		return errors.Annotatef(err, "cannot create deployer")
	}
	u.deployer = &deployerProxy{deployer}
	runnerFactory, err := runner.NewFactory(
		u.st, unitTag, u.relations.GetInfo, u.paths,
	)
	if err != nil {
		return err
	}
	u.operationFactory = operation.NewFactory(
		u.deployer,
		runnerFactory,
		&operationCallbacks{u},
		u.tomb.Dying(),
	)

	operationExecutor, err := operation.NewExecutor(
		u.paths.State.OperationsFile, u.getServiceCharmURL,
	)
	if err != nil {
		return err
	}
	u.operationExecutor = operationExecutor

	logger.Debugf("starting juju-run listener on unix:%s", u.paths.Runtime.JujuRunSocket)
	u.runListener, err = NewRunListener(u, u.paths.Runtime.JujuRunSocket)
	if err != nil {
		return err
	}
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

// initializeMetricsCollector enables the periodic collect-metrics hook
// for charms that declare metrics.
func (u *Uniter) initializeMetricsCollector() error {
	charm, err := corecharm.ReadCharmDir(u.paths.State.CharmDir)
	if err != nil {
		return err
	}
	u.collectMetricsAt = getMetricsTimer(charm)
	return nil
}

// creator exists primarily to make the implementation of the Mode funcs more
// readable -- the general pattern is to switch to get a creator func (which
// doesn't allow for the possibility of error) and then to pass the chosen
// creator down to runOperation (which can then consistently create and run
// all the operations in the same way).
type creator func(factory operation.Factory) (operation.Operation, error)

// runOperation uses the uniter's operation factory to run the supplied creation
// func, and then runs the resulting operation. This is more complex than strictly
// necessary -- we could easily use the factory and executor directly in the Mode
// funcs -- but it's in service of having more readable Mode funcs and I think it's
// worth the cost.
func (u *Uniter) runOperation(creator creator) error {
	op, err := creator(u.operationFactory)
	if err != nil {
		return errors.Annotatef(err, "cannot create operation")
	}
	return u.operationExecutor.Run(op)
}

// RunCommands executes the supplied commands in a hook context.
func (u *Uniter) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	// TODO(fwereade): this is *still* all sorts of messed-up and not especially
	// goroutine-safe, but that's not what I'm fixing at the moment. We'll deal
	// with that when we get a sane ops queue and are no longer depending on the
	// uniter mode funcs for all the rest of our scheduling.
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
	op, err := u.operationFactory.NewCommands(commandArgs, sendResponse)
	if err != nil {
		return nil, err
	}
	err = u.operationExecutor.Run(op)
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
