// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/retry"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	agenterrors "github.com/juju/juju/cmd/jujud/agent/errors"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/wrench"
)

var logger = loggo.GetLogger("juju.worker.upgradesteps")

// TODO (manadart 2021-05-18): These are exported for tests and in the case of
// the timeout, for feature tests. That especially should be a dependency of the
// worker.
var (
	PerformUpgrade = upgrades.PerformUpgrade

	// UpgradeStartTimeoutController the maximum time a controller will
	// wait for other controllers to come up and indicate they are ready
	// to begin running upgrade steps.
	UpgradeStartTimeoutController = time.Minute * 15
)

// NewLock creates a gate.Lock to be used to synchronise workers which
// need to start after upgrades have completed. The returned Lock should
// be passed to NewWorker. If the agent has already upgraded to the
// current version, then the lock will be returned in the released state.
func NewLock(agentConfig agent.Config) gate.Lock {
	lock := gate.NewLock()

	if wrench.IsActive(wrenchKey(agentConfig), "always-try-upgrade") {
		// Always enter upgrade mode. This allows test of upgrades
		// even when there's actually no upgrade steps to run.
		return lock
	}

	// Build numbers are irrelevant to upgrade steps.
	upgradedToVersion := agentConfig.UpgradedToVersion().ToPatch()
	currentVersion := jujuversion.Current.ToPatch()
	if upgradedToVersion == currentVersion {
		logger.Infof(
			"upgrade steps for %v have already been run.",
			jujuversion.Current,
		)
		lock.Unlock()
	}

	return lock
}

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(setableStatus status.Status, info string, data map[string]interface{}) error
}

// NewWorker returns a new instance of the upgradeSteps worker. It
// will run any required steps to upgrade to the currently running
// Juju version.
func NewWorker(
	upgradeComplete gate.Lock,
	agent agent.Agent,
	apiConn api.Connection,
	isController bool,
	openState func() (*state.StatePool, error),
	preUpgradeSteps upgrades.PreUpgradeStepsFunc,
	retryStrategy retry.CallArgs,
	entity StatusSetter,
	isCaas bool,
) (worker.Worker, error) {
	w := &upgradeSteps{
		upgradeComplete: upgradeComplete,
		agent:           agent,
		apiConn:         apiConn,
		openState:       openState,
		preUpgradeSteps: preUpgradeSteps,
		retryStrategy:   retryStrategy,
		entity:          entity,
		tag:             agent.CurrentConfig().Tag(),
		isController:    isController,
		isCaas:          isCaas,
	}
	w.tomb.Go(w.run)
	return w, nil
}

type upgradeSteps struct {
	tomb            tomb.Tomb
	upgradeComplete gate.Lock
	agent           agent.Agent
	apiConn         api.Connection
	openState       func() (*state.StatePool, error)
	preUpgradeSteps upgrades.PreUpgradeStepsFunc
	entity          StatusSetter
	retryStrategy   retry.CallArgs

	fromVersion version.Number
	toVersion   version.Number
	tag         names.Tag
	// If the agent is a machine agent for a controller, flag that state
	// needs to be opened before running upgrade steps
	isController bool
	isCaas       bool
	pool         *state.StatePool
}

// Kill is part of the worker.Worker interface.
func (w *upgradeSteps) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradeSteps) Wait() error {
	return w.tomb.Wait()
}

type apiLostDuringUpgrade struct {
	err error
}

func (e *apiLostDuringUpgrade) Error() string {
	return fmt.Sprintf("API connection lost during upgrade: %v", e.err)
}

func isAPILostDuringUpgrade(err error) bool {
	_, ok := err.(*apiLostDuringUpgrade)
	return ok
}

func (w *upgradeSteps) wrenchKey() string {
	return wrenchKey(w.agent.CurrentConfig())
}

func wrenchKey(agentConfig agent.Config) string {
	return agentConfig.Tag().Kind() + "-agent"
}

func (w *upgradeSteps) run() error {
	if wrench.IsActive(w.wrenchKey(), "fail-upgrade-start") {
		return nil // Make the worker stop
	}

	if w.upgradeComplete.IsUnlocked() {
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return nil
	}

	w.fromVersion = w.agent.CurrentConfig().UpgradedToVersion()
	w.toVersion = jujuversion.Current
	if w.fromVersion == w.toVersion {
		logger.Infof("upgrade to %v already completed.", w.toVersion)
		w.upgradeComplete.Unlock()
		return nil
	}

	// We need a *state.State for upgrades. We open it independently
	// of StateWorker, because we have no guarantees about when
	// and how often StateWorker might run.
	if w.isController {
		var err error
		if w.pool, err = w.openState(); err != nil {
			return err
		}
		defer func() { _ = w.pool.Close() }()

		st, err := w.pool.SystemState()
		if err != nil {
			return errors.Trace(err)
		}
		model, err := st.Model()
		if err != nil {
			return errors.Trace(err)
		}
		w.isCaas = model.Type() == state.ModelTypeCAAS
	}

	if err := w.runUpgrades(); err != nil {
		// Only return an error from the worker if the connection to
		// state went away (possible mongo primary change). Returning
		// an error when the connection is lost will cause the agent
		// to restart.
		//
		// For other errors, the error is not returned because we want
		// the agent to stay running in an error state waiting
		// for user intervention.
		if isAPILostDuringUpgrade(err) {
			return err
		}
		w.reportUpgradeFailure(err, false)
	} else {
		// Upgrade succeeded - signal that the upgrade is complete.
		logger.Infof("upgrade to %v completed successfully.", w.toVersion)
		_ = w.entity.SetStatus(status.Started, "", nil)
		w.upgradeComplete.Unlock()
	}
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *upgradeSteps) runUpgrades() error {
	upgradeInfo, err := w.prepareForUpgrade()
	if err != nil {
		return err
	}

	if wrench.IsActive(w.wrenchKey(), "fail-upgrade") {
		return errors.New("wrench")
	}

	if err := w.agent.ChangeConfig(w.runUpgradeSteps); err != nil {
		return err
	}

	if err := w.finaliseUpgrade(upgradeInfo); err != nil {
		return err
	}
	return nil
}

func (w *upgradeSteps) prepareForUpgrade() (*state.UpgradeInfo, error) {
	logger.Infof("checking that upgrade can proceed")
	if err := w.preUpgradeSteps(w.pool, w.agent.CurrentConfig(), w.pool != nil, w.isCaas); err != nil {
		return nil, errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.tag))
	}

	if w.isController {
		return w.prepareControllerForUpgrade()
	}
	return nil, nil
}

func (w *upgradeSteps) prepareControllerForUpgrade() (*state.UpgradeInfo, error) {
	logger.Infof("signalling that this controller is ready for upgrade")
	st, err := w.pool.SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	info, err := st.EnsureUpgradeInfo(w.tag.Id(), w.fromVersion, w.toVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// controllers need to wait for other controllers to be ready
	// to run the upgrade steps.
	logger.Infof("waiting for other controllers to be ready for upgrade")
	if err := w.waitForOtherControllers(info); err != nil {
		if err == tomb.ErrDying {
			logger.Warningf("stopped waiting for other controllers: %v", err)
			return nil, err
		}
		logger.Errorf("aborted wait for other controllers: %v", err)
		return nil, errors.Annotate(err, "aborted wait for other controllers")
	}

	logger.Infof("finished waiting - all controllers are ready to run upgrade steps")
	return info, nil
}

func (w *upgradeSteps) waitForOtherControllers(info *state.UpgradeInfo) error {
	watcher := info.Watch()
	defer func() { _ = watcher.Stop() }()

	maxWait := w.getUpgradeStartTimeout()
	timeout := time.After(maxWait)
	for {
		select {
		case <-watcher.Changes():
			if err := info.Refresh(); err != nil {
				return errors.Trace(err)
			}

			allReady, err := info.AllProvisionedControllersReady()
			if err != nil {
				return errors.Trace(err)
			}
			if allReady {
				return errors.Trace(info.SetStatus(state.UpgradeRunning))
			}
		case <-timeout:
			if err := info.Abort(); err != nil {
				return errors.Annotate(err, "unable to abort upgrade")
			}
			return errors.Errorf("timed out after %s", maxWait)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// runUpgradeSteps runs the required upgrade steps for the agent,
// retrying on failure. The agent's UpgradedToVersion is set
// once the upgrade is complete.
//
// This function conforms to the agent.ConfigMutator type and is
// designed to be called via an agent's ChangeConfig method.
func (w *upgradeSteps) runUpgradeSteps(agentConfig agent.ConfigSetter) error {
	if err := w.entity.SetStatus(status.Started, fmt.Sprintf("upgrading to %v", w.toVersion), nil); err != nil {
		return errors.Trace(err)
	}

	stBackend := upgrades.NewStateBackend(w.pool)
	context := upgrades.NewContext(agentConfig, w.apiConn, stBackend)
	logger.Infof("starting upgrade from %v to %v for %q", w.fromVersion, w.toVersion, w.tag)

	targets := upgradeTargets(w.isController)

	retryStrategy := w.retryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		// Abort if API connection has gone away!
		return agenterrors.ConnectionIsDead(logger, w.apiConn)
	}
	retryStrategy.NotifyFunc = func(lastErr error, attempt int) {
		if retryStrategy.Attempts != 0 && attempt != retryStrategy.Attempts {
			w.reportUpgradeFailure(lastErr, true)
		}
	}
	retryStrategy.Func = func() error {
		err := PerformUpgrade(w.fromVersion, targets, context)
		// w.entity.SetStatus(status.Error, fmt.Sprintf("TEST inner %v", err), nil)
		return err
	}

	err := retry.Call(retryStrategy)
	// w.entity.SetStatus(status.Error, fmt.Sprintf("TEST outer %v", err), nil)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
		return err
	}
	if err != nil {
		return &apiLostDuringUpgrade{err}
	}

	agentConfig.SetUpgradedToVersion(w.toVersion)
	return nil
}

func (w *upgradeSteps) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		w.fromVersion, w.toVersion, w.tag, retryText, err)
	_ = w.entity.SetStatus(status.Error,
		fmt.Sprintf("upgrade to %v failed (%s): %v", w.toVersion, retryText, err), nil)
}

func (w *upgradeSteps) finaliseUpgrade(info *state.UpgradeInfo) error {
	if !w.isController {
		return nil
	}

	if err := info.SetControllerDone(w.tag.Id()); err != nil {
		return errors.Annotate(err, "upgrade done but failed to synchronise")
	}

	return nil
}

func (w *upgradeSteps) getUpgradeStartTimeout() time.Duration {
	if wrench.IsActive(w.wrenchKey(), "short-upgrade-timeout") {
		// This duration is fairly arbitrary. During manual testing it
		// avoids the normal long wait but still provides a small
		// window to check the environment status and logs before the
		// timeout is triggered.
		return time.Minute
	}
	return UpgradeStartTimeoutController
}

// upgradeTargets determines the upgrade targets corresponding to the
// role of an agent. This determines the upgrade steps
// which will run during an upgrade.
func upgradeTargets(isController bool) []upgrades.Target {
	var targets []upgrades.Target
	if isController {
		targets = []upgrades.Target{upgrades.Controller}
	}
	return append(targets, upgrades.HostMachine)
}
