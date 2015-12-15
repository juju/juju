// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	uc "github.com/juju/utils/clock"
	"launchpad.net/tomb"

	apiundertaker "github.com/juju/juju/api/undertaker"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.undertaker")

const (
	// ripTime is the time to wait after an environment has been set to dead,
	// before removing all environment docs.
	ripTime = 24 * time.Hour
)

// NewUndertaker returns a worker which processes a dying environment.
func NewUndertaker(client apiundertaker.UndertakerClient, clock uc.Clock) worker.Worker {
	f := func(stopCh <-chan struct{}) error {
		result, err := client.EnvironInfo()
		if err != nil {
			return errors.Trace(err)
		}
		if result.Error != nil {
			return errors.Trace(result.Error)
		}
		envInfo := result.Result

		if envInfo.Life == params.Alive {
			return errors.Errorf("undertaker worker should not be started for an alive environment: %q", envInfo.GlobalName)
		}

		if envInfo.Life == params.Dying {
			// Process the dying environment. This blocks until the environment
			// is dead.
			processDyingEnv(client, clock, stopCh)
		}

		// If environ is not alive or dying, it must be dead.

		if envInfo.IsSystem {
			// Nothing to do. We don't remove environment docs for a state server
			// environment.
			return nil
		}

		tod := clock.Now()
		if envInfo.TimeOfDeath != nil {
			// If TimeOfDeath is not nil, the environment was already dead
			// before the worker was started. So we use the recorded time of
			// death. This may happen if the system is rebooted after an
			// environment is set to dead, but before the environ docs are
			// removed.
			tod = *envInfo.TimeOfDeath
		}

		// Process the dead environment
		return processDeadEnv(client, clock, tod, stopCh)
	}
	return worker.NewSimpleWorker(f)
}

func processDyingEnv(client apiundertaker.UndertakerClient, clock uc.Clock, stopCh <-chan struct{}) error {
	// ProcessDyingEnviron will fail quite a few times before it succeeds as
	// it is being woken up as every machine or service changes. We ignore the
	// error here and rely on the logging inside the ProcessDyingEnviron.
	if err := client.ProcessDyingEnviron(); err == nil {
		return nil
	}
	watcher, err := client.WatchEnvironResources()
	if err != nil {
		return errors.Trace(err)
	}
	defer watcher.Stop()

	for {
		select {
		case _, ok := <-watcher.Changes():
			if !ok {
				return watcher.Err()
			}

			err := client.ProcessDyingEnviron()
			if err != nil {
				// Yes, we ignore the error. See comment above.
				continue
			}
			return nil
		case <-stopCh:
			return tomb.ErrDying
		}
	}
}

func processDeadEnv(client apiundertaker.UndertakerClient, clock uc.Clock, tod time.Time, stopCh <-chan struct{}) error {
	timeDead := clock.Now().Sub(tod)
	wait := ripTime - timeDead
	if wait < 0 {
		wait = 0
	}

	select {
	case <-clock.After(wait):
		err := client.RemoveEnviron()
		return errors.Annotate(err, "could not remove all docs for dead environment")
	case <-stopCh:
		return tomb.ErrDying
	}
}
