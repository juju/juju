// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// NewLock returns a new gate.Lock that is unlocked if the agent has not the same version as juju
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion().ToPatch()
	currentVersion := jujuversion.Current.ToPatch()

	if upgradedToVersion == currentVersion {
		lock.Unlock()
	}

	return lock
}

type Worker struct {
	tomb            tomb.Tomb
	upgradeComplete gate.Lock

	logger Logger
}

type Config struct {
	// UpgradeComplete is a lock used to synchronise workers that must start
	// after database upgrades are verified as completed.
	UpgradeComplete gate.Lock

	// Logger is the logger for this worker.
	Logger Logger
}

// Validate validates the worker configuration.
func (c Config) Validate() error {
	if c.UpgradeComplete == nil {
		return errors.NotValidf("nil UpgradeComplete lock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// NewUpgradeDatabaseWorker returns a new Worker.
func NewUpgradeDatabaseWorker(config Config) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		upgradeComplete: config.UpgradeComplete,
		logger:          config.Logger,
	}

	w.tomb.Go(w.loop)
	return w, nil
}

// upgradeDone returns true if this worker
// does not need to run any upgrade logic.
func (w *Worker) upgradeDone() bool {
	// If we are already unlocked, there is nothing to do.
	if w.upgradeComplete.IsUnlocked() {
		return true
	}

	return false
}

func (w *Worker) runUpgrade() error {
	w.logger.Infof("running database upgrade database")

	// This dummy worker, so we can just Unlock...
	w.logger.Infof("database upgrade already completed")
	w.upgradeComplete.Unlock()

	return nil
}

// Kill implements worker.Worker.Kill.
func (w *Worker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *Worker) Wait() error {
	return w.tomb.Wait()
}

func (w *Worker) loop() error {
	if w.upgradeDone() {
		return nil
	}

	err := w.runUpgrade()
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}
