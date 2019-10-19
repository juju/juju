// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
)

// NewLock creates a gate.Lock to be used to synchronise workers
// that need to start after database upgrades have completed.
// The returned Lock should be passed to NewWorker.
// If the agent has already upgraded to the current version,
// then the lock will be returned in the released state.
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion()
	upgradedToVersion.Build = 0

	currentVersion := jujuversion.Current
	currentVersion.Build = 0

	if upgradedToVersion == currentVersion {
		lock.Unlock()
	}

	return lock
}

// Config is the configuration needed to construct an upgradeDB worker.
type Config struct {
	// UpgradeComplete is a lock used to synchronise workers that must start
	// after database upgrades are verified as completed.
	UpgradeComplete gate.Lock

	// Tag is the current machine tag.
	Tag names.Tag

	// agent is the running machine agent.
	Agent agent.Agent

	// Logger is the logger for this worker.
	Logger Logger

	// Open state is a function pointer for returning a state pool indirection.
	OpenState func() (Pool, error)

	// PerformUpgrade is a function pointer for executing the DB upgrade steps.
	// Context retrieval is lazy because because it requires a real
	// state.StatePool that we cast our Pool indirection back to.
	// We need the concrete type, because we are unable to indirect all the
	// state methods that upgrade steps might require.
	// This is OK for in-theatre operation, but is not suitable for testing.
	PerformUpgrade func(version.Number, []upgrades.Target, func() upgrades.Context) error

	// RetryStrategy is the strategy to use for re-attempting failed upgrades.
	RetryStrategy utils.AttemptStrategy
}

// Validate returns an error if the worker config is not valid.
func (cfg Config) Validate() error {
	if cfg.UpgradeComplete == nil {
		return errors.NotValidf("nil UpgradeComplete lock")
	}
	if cfg.Tag == nil {
		return errors.NotValidf("nil machine tag")
	}
	k := cfg.Tag.Kind()
	if k != names.MachineTagKind {
		return errors.NotValidf("%q tag kind", k)
	}
	if cfg.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.OpenState == nil {
		return errors.NotValidf("nil OpenState function")
	}
	if cfg.PerformUpgrade == nil {
		return errors.NotValidf("nil PerformUpgrade function")
	}
	a := utils.AttemptStrategy{}
	if cfg.RetryStrategy == a {
		return errors.NotValidf("empty RetryStrategy")
	}
	return nil
}

// upgradeDB is a worker that will run on a controller machine.
// It is responsible for running upgrade steps of type `DatabaseMaster` on the
// primary MongoDB instance.
type upgradeDB struct {
	tomb            tomb.Tomb
	upgradeComplete gate.Lock

	tag            names.Tag
	agent          agent.Agent
	logger         Logger
	pool           Pool
	performUpgrade func(version.Number, []upgrades.Target, func() upgrades.Context) error
	retryStrategy  utils.AttemptStrategy

	fromVersion version.Number
	toVersion   version.Number
}

// NewWorker validates the input configuration, then uses it to create,
// start and return an upgradeDB worker.
func NewWorker(cfg Config) (worker.Worker, error) {
	var err error

	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &upgradeDB{
		upgradeComplete: cfg.UpgradeComplete,
		tag:             cfg.Tag,
		agent:           cfg.Agent,
		logger:          cfg.Logger,
		performUpgrade:  cfg.PerformUpgrade,
		retryStrategy:   cfg.RetryStrategy,
	}
	if w.pool, err = cfg.OpenState(); err != nil {
		return nil, err
	}

	w.tomb.Go(w.run)
	return w, nil
}

func (w *upgradeDB) run() error {
	defer func() {
		if err := w.pool.Close(); err != nil {
			w.logger.Errorf("failed closing state pool: %v", err)
		}
	}()

	if w.upgradeDone() {
		return nil
	}

	isPrimary, err := w.pool.IsPrimary(w.tag.Id())
	if err != nil {
		return errors.Trace(err)
	}

	// If we are the primary we need to run the upgrade steps.
	// Otherwise we watch state and unlock once the primary has run the steps.
	if isPrimary {
		w.runUpgrade()
	} else {
		w.watchUpgrade()
	}

	return nil
}

// upgradeDone returns true if this worker
// does not need to run any upgrade logic.
func (w *upgradeDB) upgradeDone() bool {
	// If we are already unlocked, there is nothing to do.
	if w.upgradeComplete.IsUnlocked() {
		return true
	}

	// If we are already on the current version, there is nothing to do.
	w.fromVersion = w.agent.CurrentConfig().UpgradedToVersion()
	w.toVersion = jujuversion.Current
	if w.fromVersion == w.toVersion {
		w.logger.Debugf("database upgrade to %v already completed", w.toVersion)
		w.upgradeComplete.Unlock()
		return true
	}

	return false
}

func (w *upgradeDB) runUpgrade() {
	w.setStatus(status.Started, fmt.Sprintf("upgrading database to %v", w.toVersion))

	if err := w.agent.ChangeConfig(w.runUpgradeSteps); err == nil {
		w.logger.Infof("database upgrade to %v completed successfully.", w.toVersion)
		w.setStatus(status.Started, fmt.Sprintf("database upgrade to %v completed", w.toVersion))
		w.upgradeComplete.Unlock()
	}
}

// runUpgradeSteps runs the required database upgrade steps for the agent,
// retrying on failure.
func (w *upgradeDB) runUpgradeSteps(agentConfig agent.ConfigSetter) error {
	var upgradeErr error
	contextGetter := w.contextGetter(agentConfig)

	for attempt := w.retryStrategy.Start(); attempt.Next(); {
		upgradeErr = w.performUpgrade(w.fromVersion, []upgrades.Target{upgrades.DatabaseMaster}, contextGetter)
		if upgradeErr == nil {
			break
		} else {
			w.reportUpgradeFailure(upgradeErr, attempt.HasNext())
		}
	}

	return errors.Trace(upgradeErr)
}

// contextGetter returns a function that creates an upgrade context.
// Note that the performUpgrade method passed by the manifold calls
// upgrades.PerformStateUpgrade, which only uses the StateContext from this
// context. We can set the API connection to nil - it should never be used.
func (w *upgradeDB) contextGetter(agentConfig agent.ConfigSetter) func() upgrades.Context {
	return func() upgrades.Context {
		return upgrades.NewContext(agentConfig, nil, upgrades.NewStateBackend(w.pool.(*pool).StatePool))
	}
}

func (w *upgradeDB) watchUpgrade() {
	w.setStatus(status.Started, fmt.Sprintf("waiting on primary database upgrade to %v", w.toVersion))

	// TODO: Wire this up.

	w.setStatus(status.Started, fmt.Sprintf("confirmed primary database upgrade to %v", w.toVersion))
	w.upgradeComplete.Unlock()
}

func (w *upgradeDB) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}

	w.logger.Errorf("database upgrade from %v to %v for %q failed (%s): %v",
		w.fromVersion, w.toVersion, w.tag, retryText, err)

	w.setStatus(status.Error, fmt.Sprintf("upgrading database to %v", w.toVersion))
}

func (w *upgradeDB) setStatus(sts status.Status, msg string) {
	if err := w.pool.SetStatus(w.tag.Id(), sts, msg); err != nil {
		w.logger.Errorf("setting agent status: %v", err)
	}
}

// Kill is part of the worker.Worker interface.
func (w *upgradeDB) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradeDB) Wait() error {
	return w.tomb.Wait()
}
