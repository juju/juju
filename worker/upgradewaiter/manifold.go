// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradewaiter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"launchpad.net/tomb"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

var logger = loggo.GetLogger("juju.worker.upgradewaiter")

type ManifoldConfig struct {
	// UpgradeStepsWaiterName is the name of a gate.Waiter which
	// reports when upgrade steps have been run.
	UpgradeStepsWaiterName string

	// UpgradeCheckWaiter name is the name of a gate.Waiter which
	// reports when the initial check for the need to upgrade has been
	// done.
	UpgradeCheckWaiterName string
}

// Manifold returns a dependency.Manifold which aggregates the
// upgradesteps lock and the upgrader's "initial check" lock into a
// single boolean output. The output is false until both locks are
// unlocked. To make it easy to depend on this manifold, the
// manifold's worker restarts when the output value changes, causing
// dependent workers to be restarted.
func Manifold(config ManifoldConfig) dependency.Manifold {

	// This lock is unlocked when both the upgradesteps and upgrader
	// locks are unlocked. It exists outside of the start func and
	// worker code so that the state can be maintained beyond restart
	// of the manifold's worker.
	done := gate.NewLock()

	return dependency.Manifold{
		Inputs: []string{
			config.UpgradeStepsWaiterName,
			config.UpgradeCheckWaiterName,
		},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var stepsWaiter gate.Waiter
			if err := getResource(config.UpgradeStepsWaiterName, &stepsWaiter); err != nil {
				return nil, err
			}
			var checkWaiter gate.Waiter
			if err := getResource(config.UpgradeCheckWaiterName, &checkWaiter); err != nil {
				return nil, err
			}

			w := &upgradeWaiter{
				done:        done,
				stepsWaiter: stepsWaiter,
				checkWaiter: checkWaiter,
			}
			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.wait())
			}()
			return w, nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inWorker, _ := in.(*upgradeWaiter)
			if inWorker == nil {
				return errors.Errorf("in should be a *upgradeWaiter; is %T", in)
			}
			switch outPointer := out.(type) {
			case *bool:
				*outPointer = done.IsUnlocked()
			default:
				return errors.Errorf("out should be a *bool; is %T", out)
			}
			return nil
		},
	}
}

type upgradeWaiter struct {
	tomb        tomb.Tomb
	stepsWaiter gate.Waiter
	checkWaiter gate.Waiter
	done        gate.Lock
}

func (w *upgradeWaiter) wait() error {
	stepsCh := getWaiterChannel(w.stepsWaiter)
	checkCh := getWaiterChannel(w.checkWaiter)

	for {
		// If both waiters have unlocked and the aggregate gate to
		// signal upgrade completion hasn't been unlocked yet, unlock
		// it and trigger an upgradeWaiter restart so that dependent
		// manifolds notice.
		if stepsCh == nil && checkCh == nil && !w.done.IsUnlocked() {
			logger.Infof("startup upgrade operations complete")
			w.done.Unlock()
			return dependency.ErrBounce
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-stepsCh:
			stepsCh = nil
		case <-checkCh:
			checkCh = nil
		}
	}
}

func getWaiterChannel(waiter gate.Waiter) <-chan struct{} {
	// If a gate is unlocked, don't select on it.
	if waiter.IsUnlocked() {
		return nil
	}
	return waiter.Unlocked()
}

// Kill is part of the worker.Worker interface.
func (w *upgradeWaiter) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradeWaiter) Wait() error {
	return w.tomb.Wait()
}
