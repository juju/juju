package reboot

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/fslock"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/utils/rebootstate"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.reboot")
var LockDir = paths.MustSucceed(paths.LockDir(version.Current.Series))

const RebootMessage = "preparing for reboot"

var _ worker.NotifyWatchHandler = (*Reboot)(nil)

// The reboot worker listens for changes to the reboot flag and
// exists with worker.ErrRebootMachine if the machine should reboot or
// with worker.ErrShutdownMachine if it should shutdoen. This will be picked
// up by the machine agent as a fatal error and will do the
// right thing (reboot or shutdown)
type Reboot struct {
	tomb tomb.Tomb
	st   *reboot.State
	tag  names.MachineTag
	lock *fslock.Lock
}

func NewReboot(st *reboot.State, agentConfig agent.Config) (worker.Worker, error) {
	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T", agentConfig.Tag())
	}
	lock, err := fslock.NewLock(LockDir, "uniter-hook-execution")
	if err != nil {
		return nil, err
	}
	r := &Reboot{
		st:   st,
		tag:  tag,
		lock: lock,
	}
	return worker.NewNotifyWorker(r), nil
}

func (r *Reboot) breakHookLock() error {
	if r.lock.Message() != RebootMessage {
		// Not a lock held by the machine agent in order to reboot
		return nil
	}
	err := r.lock.BreakLock()
	if err != nil {
		return err
	}
	return nil
}

func (r *Reboot) checkForRebootState() error {
	if !rebootstate.IsPresent() {
		return nil
	}

	// Check if reboot flag is set.
	rAction, err := r.st.GetRebootAction()
	if err != nil {
		return err
	}
	if rAction == params.ShouldDoNothing {
		// Reboot flag is clear.
		logger.Infof("Clearing stale reboot state file")
		err = rebootstate.Remove()
		return err
	}

	// Clear reboot flag
	err = r.st.ClearReboot()
	if err != nil {
		logger.Errorf("Failed to clear reboot flag: %v", err)
		return err
	}
	// Remove reboot state file
	err = rebootstate.Remove()
	if err != nil {
		return err
	}
	// Break the hook lock if held
	return r.breakHookLock()
}

func (r *Reboot) SetUp() (watcher.NotifyWatcher, error) {
	logger.Debugf("Reboot worker setup")
	err := r.checkForRebootState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	watcher, err := r.st.WatchForRebootEvent()
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

func (r *Reboot) Handle() error {
	rAction, err := r.st.GetRebootAction()
	if err != nil {
		return err
	}
	logger.Debugf("Reboot worker got action: %v", rAction)
	switch rAction {
	case params.ShouldReboot:
		r.lock.Lock(RebootMessage)
		return worker.ErrRebootMachine
	case params.ShouldShutdown:
		r.lock.Lock(RebootMessage)
		return worker.ErrShutdownMachine
	}
	return nil
}

func (r *Reboot) TearDown() error {
	// nothing to teardown.
	return nil
}
