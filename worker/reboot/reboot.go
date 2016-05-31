// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/fslock"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/reboot"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.reboot")

const RebootMessage = "preparing for reboot"

// The reboot worker listens for changes to the reboot flag and
// exists with worker.ErrRebootMachine if the machine should reboot or
// with worker.ErrShutdownMachine if it should shutdown. This will be picked
// up by the machine agent as a fatal error and will do the
// right thing (reboot or shutdown)
type Reboot struct {
	tomb        tomb.Tomb
	st          reboot.State
	tag         names.MachineTag
	machineLock *fslock.Lock
}

func NewReboot(st reboot.State, agentConfig agent.Config, machineLock *fslock.Lock) (worker.Worker, error) {
	tag, ok := agentConfig.Tag().(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T: %v", agentConfig.Tag(), agentConfig.Tag())
	}
	r := &Reboot{
		st:          st,
		tag:         tag,
		machineLock: machineLock,
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: r,
	})
	return w, errors.Trace(err)
}

func (r *Reboot) checkForRebootState() error {
	if r.machineLock.IsLocked() == false {
		return nil
	}

	msg, err := r.machineLock.Message()
	if err != nil {
		return errors.Trace(err)
	}
	if msg != RebootMessage {
		return nil
	}
	// Not a lock held by the machne agent in order to reboot
	err = r.machineLock.BreakLock()
	return errors.Trace(err)
}

func (r *Reboot) SetUp() (watcher.NotifyWatcher, error) {
	if err := r.checkForRebootState(); err != nil {
		return nil, errors.Trace(err)
	}
	watcher, err := r.st.WatchForRebootEvent()
	return watcher, errors.Trace(err)
}

func (r *Reboot) Handle(_ <-chan struct{}) error {
	rAction, err := r.st.GetRebootAction()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Reboot worker got action: %v", rAction)
	switch rAction {
	case params.ShouldReboot:
		r.machineLock.Lock(RebootMessage)
		return worker.ErrRebootMachine
	case params.ShouldShutdown:
		r.machineLock.Lock(RebootMessage)
		return worker.ErrShutdownMachine
	}
	return nil
}

func (r *Reboot) TearDown() error {
	// nothing to teardown.
	return nil
}
