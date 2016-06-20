package reboot

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/names"
	"github.com/juju/utils/clock"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.reboot")

const RebootMessage = "preparing for reboot"

var _ worker.NotifyWatchHandler = (*Reboot)(nil)

// The reboot worker listens for changes to the reboot flag and
// exists with worker.ErrRebootMachine if the machine should reboot or
// with worker.ErrShutdownMachine if it should shutdown. This will be picked
// up by the machine agent as a fatal error and will do the
// right thing (reboot or shutdown)
type Reboot struct {
	tomb            tomb.Tomb
	st              reboot.State
	tag             names.MachineTag
	machineLockName string
	clock           clock.Clock
}

func NewReboot(st reboot.State, agentConfig agent.Config, machineLockName string, clock clock.Clock) (worker.Worker, error) {
	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T: %v", agentConfig.Tag(), agentConfig.Tag())
	}
	r := &Reboot{
		st:              st,
		tag:             tag,
		machineLockName: machineLockName,
		clock:           clock,
	}
	return worker.NewNotifyWorker(r), nil
}

func (r *Reboot) SetUp() (watcher.NotifyWatcher, error) {
	logger.Debugf("Reboot worker setup")
	watcher, err := r.st.WatchForRebootEvent()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return watcher, nil
}

func (r *Reboot) Handle(_ <-chan struct{}) error {
	rAction, err := r.st.GetRebootAction()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Reboot worker got action: %v", rAction)

	// NOTE: Here we explicitly avoid stopping on the abort channel as we are
	// wanting to make sure that we grab the lock and return an error
	// sufficiently heavyweight to get the agent to restart.
	spec := mutex.Spec{
		Name:  r.machineLockName,
		Clock: r.clock,
		Delay: 250 * time.Millisecond,
	}
	switch rAction {
	case params.ShouldReboot:
		if _, err := mutex.Acquire(spec); err != nil {
			return errors.Trace(err)
		}
		return worker.ErrRebootMachine
	case params.ShouldShutdown:
		if _, err := mutex.Acquire(spec); err != nil {
			return errors.Trace(err)
		}
		return worker.ErrShutdownMachine
	default:
		return nil
	}
}

func (r *Reboot) TearDown() error {
	// nothing to teardown.
	return nil
}
