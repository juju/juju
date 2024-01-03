// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/api/agent/reboot"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/watcher"
	jworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.worker.reboot")

// The reboot worker listens for changes to the reboot flag and
// exists with worker.ErrRebootMachine if the machine should reboot or
// with worker.ErrShutdownMachine if it should shutdown. This will be picked
// up by the machine agent as a fatal error and will do the
// right thing (reboot or shutdown)
type Reboot struct {
	client      reboot.Client
	tag         names.MachineTag
	machineLock machinelock.Lock
}

func NewReboot(client reboot.Client, agentTag names.Tag, machineLock machinelock.Lock) (worker.Worker, error) {
	tag, ok := agentTag.(names.MachineTag)
	if !ok {
		return nil, errors.Errorf("Expected names.MachineTag, got %T: %v", agentTag, agentTag)
	}
	r := &Reboot{
		client:      client,
		tag:         tag,
		machineLock: machineLock,
	}
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: r,
	})
	return w, errors.Trace(err)
}

func (r *Reboot) SetUp(_ context.Context) (watcher.NotifyWatcher, error) {
	watcher, err := r.client.WatchForRebootEvent()
	return watcher, errors.Trace(err)
}

func (r *Reboot) Handle(_ context.Context) error {
	rAction, err := r.client.GetRebootAction()
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("Reboot worker got action: %v", rAction)

	// NOTE: Here we explicitly avoid stopping on the abort channel as we are
	// wanting to make sure that we grab the lock and return an error
	// sufficiently heavyweight to get the agent to restart.
	spec := machinelock.Spec{
		Worker:   "reboot",
		NoCancel: true,
	}

	switch rAction {
	case params.ShouldReboot:
		spec.Comment = "reboot"
		if _, err := r.machineLock.Acquire(spec); err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("machine lock will not be released manually")
		err = jworker.ErrRebootMachine
	case params.ShouldShutdown:
		spec.Comment = "shutdown"
		if _, err := r.machineLock.Acquire(spec); err != nil {
			return errors.Trace(err)
		}
		logger.Debugf("machine lock will not be released manually")
		err = jworker.ErrShutdownMachine
	}

	if err != nil {
		// We clear the reboot flag here rather than when we are attempting to
		// handle the reboot error in the machine agent code as it is possible
		// that the machine agent is also a controller, and the apiserver has been
		// shut down. It is better to clear the flag and not reboot on a weird
		// error rather than get into a reboot loop because we can't shutdown.
		if err := r.client.ClearReboot(); err != nil {
			logger.Infof("unable to clear reboot flag: %v", err)
		}
	}
	return err
}

func (r *Reboot) TearDown() error {
	// nothing to teardown.
	return nil
}
