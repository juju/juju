// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	cmdutil "github.com/juju/juju/cmd/jujud/util"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/wrench"
)

var logger = loggo.GetLogger("juju.worker.upgradesteps")

var (
	PerformUpgrade = upgrades.PerformUpgrade // Allow patching

	// The maximum time a master controller will wait for other
	// controllers to come up and indicate they are ready to begin
	// running upgrade steps.
	UpgradeStartTimeoutMaster = time.Minute * 15

	// The maximum time a secondary controller will wait for other
	// controllers to come up and indicate they are ready to begin
	// running upgrade steps. This is effectively "forever" because we
	// don't really want secondaries to ever give up once they've
	// indicated that they're ready to upgrade. It's up to the master
	// to abort the upgrade if required.
	//
	// This should get reduced when/if master re-elections are
	// introduce in the case a master that failing to come up for
	// upgrade.
	UpgradeStartTimeoutSecondary = time.Hour * 4
)

// NewLock creates a gate.Lock to be used to synchronise workers which
// need to start after upgrades have completed. If no upgrade steps
// are required the Lock is unlocked and the version in agent's
// configuration is updated to the currently running version.
//
// The returned Lock should be passed to NewWorker.
func NewLock(a agent.Agent) (gate.Lock, error) {
	lock := gate.NewLock()

	if wrench.IsActive("machine-agent", "always-try-upgrade") {
		// Always enter upgrade mode. This allows test of upgrades
		// even when there's actually no upgrade steps to run.
		return lock, nil
	}

	err := a.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		if !upgrades.AreUpgradesDefined(agentConfig.UpgradedToVersion()) {
			logger.Infof("no upgrade steps required or upgrade steps for %v "+
				"have already been run.", version.Current)
			lock.Unlock()

			// Even if no upgrade is required the version number in
			// the agent's config still needs to be bumped.
			agentConfig.SetUpgradedToVersion(version.Current)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return lock, nil
}

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(status params.Status, info string, data map[string]interface{}) error
}

// NewWorker returns a new instance of the upgradesteps worker. It
// will run any required steps to upgrade to the currently running
// Juju version.
func NewWorker(
	upgradeComplete gate.Lock,
	agent agent.Agent,
	apiConn api.Connection,
	jobs []multiwatcher.MachineJob,
	openState func() (*state.State, func(), error),
	preUpgradeSteps func(st *state.State, agentConf agent.Config, isController, isMasterServer bool) error,
	machine StatusSetter,
) (worker.Worker, error) {
	tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("machine agent's tag is not a MachineTag")
	}
	w := &upgradesteps{
		upgradeComplete: upgradeComplete,
		agent:           agent,
		apiConn:         apiConn,
		jobs:            jobs,
		openState:       openState,
		preUpgradeSteps: preUpgradeSteps,
		machine:         machine,
		tag:             tag,
	}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.run())
	}()
	return w, nil
}

type upgradesteps struct {
	tomb            tomb.Tomb
	upgradeComplete gate.Lock
	agent           agent.Agent
	apiConn         api.Connection
	jobs            []multiwatcher.MachineJob
	openState       func() (*state.State, func(), error)
	preUpgradeSteps func(st *state.State, agentConf agent.Config, isController, isMaster bool) error
	machine         StatusSetter

	fromVersion  version.Number
	toVersion    version.Number
	tag          names.MachineTag
	isMaster     bool
	isController bool
	st           *state.State
}

// Kill is part of the worker.Worker interface.
func (w *upgradesteps) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *upgradesteps) Wait() error {
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

func (w *upgradesteps) run() error {
	if wrench.IsActive("machine-agent", "fail-upgrade-start") {
		return nil // Make the worker stop
	}

	if w.upgradeComplete.IsUnlocked() {
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return nil
	}

	w.fromVersion = w.agent.CurrentConfig().UpgradedToVersion()
	w.toVersion = version.Current
	if w.fromVersion == w.toVersion {
		logger.Infof("upgrade to %v already completed.", w.toVersion)
		w.upgradeComplete.Unlock()
		return nil
	}

	// If the machine agent is a controller, flag that state
	// needs to be opened before running upgrade steps
	for _, job := range w.jobs {
		if job == multiwatcher.JobManageModel {
			w.isController = true
		}
	}

	// We need a *state.State for upgrades. We open it independently
	// of StateWorker, because we have no guarantees about when
	// and how often StateWorker might run.
	if w.isController {
		var closer func()
		var err error
		if w.st, closer, err = w.openState(); err != nil {
			return err
		}
		defer closer()

		if w.isMaster, err = IsMachineMaster(w.st, w.tag.Id()); err != nil {
			return errors.Trace(err)
		}
	}

	if err := w.runUpgrades(); err != nil {
		// Only return an error from the worker if the connection to
		// state went away (possible mongo master change). Returning
		// an error when the connection is lost will cause the agent
		// to restart.
		//
		// For other errors, the error is not returned because we want
		// the machine agent to stay running in an error state waiting
		// for user intervention.
		if isAPILostDuringUpgrade(err) {
			return err
		}
		w.reportUpgradeFailure(err, false)

	} else {
		// Upgrade succeeded - signal that the upgrade is complete.
		logger.Infof("upgrade to %v completed successfully.", w.toVersion)
		w.machine.SetStatus(params.StatusStarted, "", nil)
		w.upgradeComplete.Unlock()
	}
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (w *upgradesteps) runUpgrades() error {
	upgradeInfo, err := w.prepareForUpgrade()
	if err != nil {
		return err
	}

	if wrench.IsActive("machine-agent", "fail-upgrade") {
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

func (w *upgradesteps) prepareForUpgrade() (*state.UpgradeInfo, error) {
	logger.Infof("checking that upgrade can proceed")
	if err := w.preUpgradeSteps(w.st, w.agent.CurrentConfig(), w.st != nil, w.isMaster); err != nil {
		return nil, errors.Annotatef(err, "%s cannot be upgraded", names.ReadableString(w.tag))
	}

	if !w.isController {
		return nil, nil
	}

	logger.Infof("signalling that this controller is ready for upgrade")
	info, err := w.st.EnsureUpgradeInfo(w.tag.Id(), w.fromVersion, w.toVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// controllers need to wait for other controllers to be ready
	// to run the upgrade steps.
	logger.Infof("waiting for other controllers to be ready for upgrade")
	if err := w.waitForOtherControllers(info); err != nil {
		if err == tomb.ErrDying {
			logger.Warningf(`stopped waiting for other controllers: %v`, err)
			return nil, err
		}
		logger.Errorf(`aborted wait for other controllers: %v`, err)
		// If master, trigger a rollback to the previous agent version.
		if w.isMaster {
			logger.Errorf("downgrading model agent version to %v due to aborted upgrade",
				w.fromVersion)
			if rollbackErr := w.st.SetModelAgentVersion(w.fromVersion); rollbackErr != nil {
				logger.Errorf("rollback failed: %v", rollbackErr)
				return nil, errors.Annotate(rollbackErr, "failed to roll back desired agent version")
			}
		}
		return nil, errors.Annotate(err, "aborted wait for other controllers")
	}
	if w.isMaster {
		logger.Infof("finished waiting - all controllers are ready to run upgrade steps")
	} else {
		logger.Infof("finished waiting - the master has completed its upgrade steps")
	}
	return info, nil
}

func (w *upgradesteps) waitForOtherControllers(info *state.UpgradeInfo) error {
	watcher := info.Watch()
	defer watcher.Stop()

	maxWait := getUpgradeStartTimeout(w.isMaster)
	timeout := time.After(maxWait)
	for {
		select {
		case <-watcher.Changes():
			if err := info.Refresh(); err != nil {
				return errors.Trace(err)
			}
			if w.isMaster {
				if ready, err := info.AllProvisionedControllersReady(); err != nil {
					return errors.Trace(err)
				} else if ready {
					// All controllers ready to start upgrade
					err := info.SetStatus(state.UpgradeRunning)
					return errors.Trace(err)
				}
			} else {
				if info.Status() == state.UpgradeFinishing {
					// Master is done, ok to proceed
					return nil
				}
			}
		case <-timeout:
			if w.isMaster {
				if err := info.Abort(); err != nil {
					return errors.Annotate(err, "unable to abort upgrade")
				}
			}
			return errors.Errorf("timed out after %s", maxWait)
		case <-w.tomb.Dying():
			return tomb.ErrDying
		}

	}
}

// runUpgradeSteps runs the required upgrade steps for the machine
// agent, retrying on failure. The agent's UpgradedToVersion is set
// once the upgrade is complete.
//
// This function conforms to the agent.ConfigMutator type and is
// designed to be called via a machine agent's ChangeConfig method.
func (w *upgradesteps) runUpgradeSteps(agentConfig agent.ConfigSetter) error {
	var upgradeErr error
	w.machine.SetStatus(params.StatusStarted, fmt.Sprintf("upgrading to %v", w.toVersion), nil)

	context := upgrades.NewContext(agentConfig, w.apiConn, w.st)
	logger.Infof("starting upgrade from %v to %v for %q", w.fromVersion, w.toVersion, w.tag)

	targets := jobsToTargets(w.jobs, w.isMaster)
	attempts := getUpgradeRetryStrategy()
	for attempt := attempts.Start(); attempt.Next(); {
		upgradeErr = PerformUpgrade(w.fromVersion, targets, context)
		if upgradeErr == nil {
			break
		}
		if cmdutil.ConnectionIsDead(logger, w.apiConn) {
			// API connection has gone away - abort!
			return &apiLostDuringUpgrade{upgradeErr}
		}
		if attempt.HasNext() {
			w.reportUpgradeFailure(upgradeErr, true)
		}
	}
	if upgradeErr != nil {
		return upgradeErr
	}
	agentConfig.SetUpgradedToVersion(w.toVersion)
	return nil
}

func (w *upgradesteps) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		w.fromVersion, w.toVersion, w.tag, retryText, err)
	w.machine.SetStatus(params.StatusError,
		fmt.Sprintf("upgrade to %v failed (%s): %v", w.toVersion, retryText, err), nil)
}

func (w *upgradesteps) finaliseUpgrade(info *state.UpgradeInfo) error {
	if !w.isController {
		return nil
	}

	if w.isMaster {
		// Tell other controllers that the master has completed its
		// upgrade steps.
		if err := info.SetStatus(state.UpgradeFinishing); err != nil {
			return errors.Annotate(err, "upgrade done but")
		}
	}

	if err := info.SetControllerDone(w.tag.Id()); err != nil {
		return errors.Annotate(err, "upgrade done but failed to synchronise")
	}

	return nil
}

func getUpgradeStartTimeout(isMaster bool) time.Duration {
	if wrench.IsActive("machine-agent", "short-upgrade-timeout") {
		// This duration is fairly arbitrary. During manual testing it
		// avoids the normal long wait but still provides a small
		// window to check the environment status and logs before the
		// timeout is triggered.
		return time.Minute
	}

	if isMaster {
		return UpgradeStartTimeoutMaster
	}
	return UpgradeStartTimeoutSecondary
}

var IsMachineMaster = func(st *state.State, machineId string) (bool, error) {
	if st == nil {
		// If there is no state, we aren't a master.
		return false, nil
	}
	// Not calling the agent openState method as it does other checks
	// we really don't care about here.  All we need here is the machine
	// so we can determine if we are the master or not.
	machine, err := st.Machine(machineId)
	if err != nil {
		// This shouldn't happen, and if it does, the state worker will have
		// found out before us, and already errored, or is likely to error out
		// very shortly.  All we do here is return the error. The state worker
		// returns an error that will cause the agent to be terminated.
		return false, errors.Trace(err)
	}
	isMaster, err := mongo.IsMaster(st.MongoSession(), machine)
	if err != nil {
		return false, errors.Trace(err)
	}
	return isMaster, nil
}

var getUpgradeRetryStrategy = func() utils.AttemptStrategy {
	return utils.AttemptStrategy{
		Delay: 2 * time.Minute,
		Min:   5,
	}
}

// jobsToTargets determines the upgrade targets corresponding to the
// jobs assigned to a machine agent. This determines the upgrade steps
// which will run during an upgrade.
func jobsToTargets(jobs []multiwatcher.MachineJob, isMaster bool) (targets []upgrades.Target) {
	for _, job := range jobs {
		switch job {
		case multiwatcher.JobManageModel:
			targets = append(targets, upgrades.Controller)
			if isMaster {
				targets = append(targets, upgrades.DatabaseMaster)
			}
		case multiwatcher.JobHostUnits:
			targets = append(targets, upgrades.HostMachine)
		}
	}
	return
}
