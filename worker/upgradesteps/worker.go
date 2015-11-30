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
	"github.com/juju/juju/wrench"
)

// TODO(mjs) - For mainly historical reasons, there's a lot to not
// like about how this worker is structured. Some things to fix when
// time allows:
//
// 1. The UpgradeWorkerContext holds lots of stuff that is specific to a
// single run of the worker. Create a separate struct for the worker
// leaving only a few things in the context.
//
// 2. The work done by InitializeUsingAgent should probably be done in
// NewUpgradeWorkerContext (so that InitializeUsingAgent can be
// removed).

var logger = loggo.GetLogger("juju.worker.upgradesteps")

// StatusSetter defines the single method required to set an agent's
// status.
type StatusSetter interface {
	SetStatus(status params.Status, info string, data map[string]interface{}) error
}

var (
	PerformUpgrade = upgrades.PerformUpgrade // Allow patching

	// The maximum time a master state server will wait for other
	// state servers to come up and indicate they are ready to begin
	// running upgrade steps.
	UpgradeStartTimeoutMaster = time.Minute * 15

	// The maximum time a secondary state server will wait for other
	// state servers to come up and indicate they are ready to begin
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

// NewUpgradeWorkerContext returns a new UpgradeWorkerContext.
func NewUpgradeWorkerContext() *UpgradeWorkerContext {
	return &UpgradeWorkerContext{
		UpgradeComplete: make(chan struct{}),
	}
}

// UpgradeWorkerContext holds the data used by the upgradesteps
// worker.
type UpgradeWorkerContext struct {
	tomb                tomb.Tomb
	UpgradeComplete     chan struct{}
	fromVersion         version.Number
	toVersion           version.Number
	agent               agent.Agent
	apiConn             api.Connection
	openStateForUpgrade func() (*state.State, func(), error)
	machine             StatusSetter
	tag                 names.MachineTag
	machineId           string
	isMaster            bool
	jobs                []multiwatcher.MachineJob
	agentConfig         agent.Config
	isStateServer       bool
	st                  *state.State
}

// InitialiseUsingAgent sets up a UpgradeWorkerContext from a machine agent instance.
// It may update the agent's configuration.
func (c *UpgradeWorkerContext) InitializeUsingAgent(a agent.Agent) error {
	if wrench.IsActive("machine-agent", "always-try-upgrade") {
		// Always enter upgrade mode. This allows test of upgrades
		// even when there's actually no upgrade steps to run.
		return nil
	}
	return a.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		if !upgrades.AreUpgradesDefined(agentConfig.UpgradedToVersion()) {
			logger.Infof("no upgrade steps required or upgrade steps for %v "+
				"have already been run.", version.Current)
			close(c.UpgradeComplete)

			// Even if no upgrade is required the version number in
			// the agent's config still needs to be bumped.
			agentConfig.SetUpgradedToVersion(version.Current)
		}
		return nil
	})
}

func (c *UpgradeWorkerContext) Worker(
	agent agent.Agent,
	apiConn api.Connection,
	jobs []multiwatcher.MachineJob,
	openStateForUpgrade func() (*state.State, func(), error),
	machine StatusSetter,
) (worker.Worker, error) {
	c.agent = agent
	c.apiConn = apiConn
	c.jobs = jobs
	c.openStateForUpgrade = openStateForUpgrade
	c.machine = machine

	tag, ok := agent.CurrentConfig().Tag().(names.MachineTag)
	if !ok {
		return nil, errors.New("machine agent's tag is not a MachineTag")
	}
	c.tag = tag
	c.machineId = tag.Id()

	go func() {
		defer c.tomb.Done()
		c.tomb.Kill(c.run())
	}()
	return c, nil
}

// Kill is part of the worker.Worker interface.
func (c *UpgradeWorkerContext) Kill() {
	c.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (c *UpgradeWorkerContext) Wait() error {
	return c.tomb.Wait()
}

func (c *UpgradeWorkerContext) IsUpgradeRunning() bool {
	select {
	case <-c.UpgradeComplete:
		return false
	default:
		return true
	}
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

func (c *UpgradeWorkerContext) run() error {
	if wrench.IsActive("machine-agent", "fail-upgrade-start") {
		return nil // Make the worker stop
	}

	select {
	case <-c.UpgradeComplete:
		// Our work is already done (we're probably being restarted
		// because the API connection has gone down), so do nothing.
		return nil
	default:
	}

	c.agentConfig = c.agent.CurrentConfig()

	c.fromVersion = c.agentConfig.UpgradedToVersion()
	c.toVersion = version.Current
	if c.fromVersion == c.toVersion {
		logger.Infof("upgrade to %v already completed.", c.toVersion)
		close(c.UpgradeComplete)
		return nil
	}

	// If the machine agent is a state server, flag that state
	// needs to be opened before running upgrade steps
	for _, job := range c.jobs {
		if job == multiwatcher.JobManageEnviron {
			c.isStateServer = true
		}
	}

	// We need a *state.State for upgrades. We open it independently
	// of StateWorker, because we have no guarantees about when
	// and how often StateWorker might run.
	if c.isStateServer {
		var closer func()
		var err error
		if c.st, closer, err = c.openStateForUpgrade(); err != nil {
			return err
		}
		defer closer()

		if c.isMaster, err = IsMachineMaster(c.st, c.machineId); err != nil {
			return errors.Trace(err)
		}
	}

	if err := c.runUpgrades(); err != nil {
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
		c.reportUpgradeFailure(err, false)

	} else {
		// Upgrade succeeded - signal that the upgrade is complete.
		logger.Infof("upgrade to %v completed successfully.", c.toVersion)
		c.machine.SetStatus(params.StatusStarted, "", nil)
		close(c.UpgradeComplete)
	}
	return nil
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (c *UpgradeWorkerContext) runUpgrades() error {
	upgradeInfo, err := c.prepareForUpgrade()
	if err != nil {
		return err
	}

	if wrench.IsActive("machine-agent", "fail-upgrade") {
		return errors.New("wrench")
	}

	if err := c.agent.ChangeConfig(c.runUpgradeSteps); err != nil {
		return err
	}

	if err := c.finaliseUpgrade(upgradeInfo); err != nil {
		return err
	}
	return nil
}

func (c *UpgradeWorkerContext) prepareForUpgrade() (*state.UpgradeInfo, error) {
	if !c.isStateServer {
		return nil, nil
	}

	logger.Infof("signalling that this state server is ready for upgrade")
	info, err := c.st.EnsureUpgradeInfo(c.machineId, c.fromVersion, c.toVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// State servers need to wait for other state servers to be ready
	// to run the upgrade steps.
	logger.Infof("waiting for other state servers to be ready for upgrade")
	if err := c.waitForOtherStateServers(info); err != nil {
		if err == tomb.ErrDying {
			logger.Warningf(`stopped waiting for other state servers: %v`, err)
			return nil, err
		}
		logger.Errorf(`aborted wait for other state servers: %v`, err)
		// If master, trigger a rollback to the previous agent version.
		if c.isMaster {
			logger.Errorf("downgrading environment agent version to %v due to aborted upgrade",
				c.fromVersion)
			if rollbackErr := c.st.SetEnvironAgentVersion(c.fromVersion); rollbackErr != nil {
				logger.Errorf("rollback failed: %v", rollbackErr)
				return nil, errors.Annotate(rollbackErr, "failed to roll back desired agent version")
			}
		}
		return nil, errors.Annotate(err, "aborted wait for other state servers")
	}
	if c.isMaster {
		logger.Infof("finished waiting - all state servers are ready to run upgrade steps")
	} else {
		logger.Infof("finished waiting - the master has completed its upgrade steps")
	}
	return info, nil
}

func (c *UpgradeWorkerContext) waitForOtherStateServers(info *state.UpgradeInfo) error {
	watcher := info.Watch()
	defer watcher.Stop()

	maxWait := getUpgradeStartTimeout(c.isMaster)
	timeout := time.After(maxWait)
	for {
		select {
		case <-watcher.Changes():
			if err := info.Refresh(); err != nil {
				return errors.Trace(err)
			}
			if c.isMaster {
				if ready, err := info.AllProvisionedStateServersReady(); err != nil {
					return errors.Trace(err)
				} else if ready {
					// All state servers ready to start upgrade
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
			if c.isMaster {
				if err := info.Abort(); err != nil {
					return errors.Annotate(err, "unable to abort upgrade")
				}
			}
			return errors.Errorf("timed out after %s", maxWait)
		case <-c.tomb.Dying():
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
func (c *UpgradeWorkerContext) runUpgradeSteps(agentConfig agent.ConfigSetter) error {
	var upgradeErr error
	c.machine.SetStatus(params.StatusStarted, fmt.Sprintf("upgrading to %v", c.toVersion), nil)

	context := upgrades.NewContext(agentConfig, c.apiConn, c.st)
	logger.Infof("starting upgrade from %v to %v for %q", c.fromVersion, c.toVersion, c.tag)

	targets := jobsToTargets(c.jobs, c.isMaster)
	attempts := getUpgradeRetryStrategy()
	for attempt := attempts.Start(); attempt.Next(); {
		upgradeErr = PerformUpgrade(c.fromVersion, targets, context)
		if upgradeErr == nil {
			break
		}
		if cmdutil.ConnectionIsDead(logger, c.apiConn) {
			// API connection has gone away - abort!
			return &apiLostDuringUpgrade{upgradeErr}
		}
		if attempt.HasNext() {
			c.reportUpgradeFailure(upgradeErr, true)
		}
	}
	if upgradeErr != nil {
		return upgradeErr
	}
	agentConfig.SetUpgradedToVersion(c.toVersion)
	return nil
}

func (c *UpgradeWorkerContext) reportUpgradeFailure(err error, willRetry bool) {
	retryText := "will retry"
	if !willRetry {
		retryText = "giving up"
	}
	logger.Errorf("upgrade from %v to %v for %q failed (%s): %v",
		c.fromVersion, c.toVersion, c.tag, retryText, err)
	c.machine.SetStatus(params.StatusError,
		fmt.Sprintf("upgrade to %v failed (%s): %v", c.toVersion, retryText, err), nil)
}

func (c *UpgradeWorkerContext) finaliseUpgrade(info *state.UpgradeInfo) error {
	if !c.isStateServer {
		return nil
	}

	if c.isMaster {
		// Tell other state servers that the master has completed its
		// upgrade steps.
		if err := info.SetStatus(state.UpgradeFinishing); err != nil {
			return errors.Annotate(err, "upgrade done but")
		}
	}

	if err := info.SetStateServerDone(c.machineId); err != nil {
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
		case multiwatcher.JobManageEnviron:
			targets = append(targets, upgrades.StateServer)
			if isMaster {
				targets = append(targets, upgrades.DatabaseMaster)
			}
		case multiwatcher.JobHostUnits:
			targets = append(targets, upgrades.HostMachine)
		}
	}
	return
}
