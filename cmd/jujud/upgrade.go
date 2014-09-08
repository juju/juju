package main

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/wrench"
)

type upgradingMachineAgent interface {
	ensureMongoServer(agent.Config) error
	setMachineStatus(*api.State, params.Status, string) error
	CurrentConfig() agent.Config
	ChangeConfig(AgentConfigMutator) error
	Dying() <-chan struct{}
}

var (
	upgradesPerformUpgrade = upgrades.PerformUpgrade // Allow patching

	// The maximum time a master state server will wait for other
	// state servers to come up and indicate they are ready to begin
	// running upgrade steps.
	upgradeStartTimeoutMaster = time.Minute * 15

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
	upgradeStartTimeoutSecondary = time.Hour * 4
)

func NewUpgradeWorkerContext() *upgradeWorkerContext {
	return &upgradeWorkerContext{
		UpgradeComplete: make(chan struct{}),
	}
}

type upgradeWorkerContext struct {
	UpgradeComplete chan struct{}
	agent           upgradingMachineAgent
	apiState        *api.State
	jobs            []params.MachineJob
	agentConfig     agent.Config
	isStateServer   bool
	st              *state.State
}

// InitialiseUsingAgent sets up a upgradeWorkerContext from a machine agent instance.
// It may update the agent's configuration.
func (c *upgradeWorkerContext) InitializeUsingAgent(a upgradingMachineAgent) error {
	if wrench.IsActive("machine-agent", "always-try-upgrade") {
		// Always enter upgrade mode. This allows test of upgrades
		// even when there's actually no upgrade steps to run.
		return nil
	}
	return a.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		if !upgrades.AreUpgradesDefined(agentConfig.UpgradedToVersion()) {
			logger.Infof("no upgrade steps required or upgrade steps for %v "+
				"have already been run.", version.Current.Number)
			close(c.UpgradeComplete)

			// Even if no upgrade is required the version number in
			// the agent's config still needs to be bumped.
			agentConfig.SetUpgradedToVersion(version.Current.Number)
		}
		return nil
	})
}

func (c *upgradeWorkerContext) Worker(
	agent upgradingMachineAgent,
	apiState *api.State,
	jobs []params.MachineJob,
) worker.Worker {
	c.agent = agent
	c.apiState = apiState
	c.jobs = jobs
	return worker.NewSimpleWorker(c.run)
}

func (c *upgradeWorkerContext) IsUpgradeRunning() bool {
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

func (c *upgradeWorkerContext) run(stop <-chan struct{}) error {
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

	// If the machine agent is a state server, flag that state
	// needs to be opened before running upgrade steps
	for _, job := range c.jobs {
		if job == params.JobManageEnviron {
			c.isStateServer = true
		}
	}
	// We need a *state.State for upgrades. We open it independently
	// of StateWorker, because we have no guarantees about when
	// and how often StateWorker might run.
	if c.isStateServer {
		var err error
		c.st, err = openStateForUpgrade(c.agent, c.agentConfig)
		if err != nil {
			return err
		}
		defer c.st.Close()
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
	} else {
		// Upgrade succeeded - signal that the upgrade is complete.
		close(c.UpgradeComplete)
	}
	return nil
}

var agentTerminating = errors.New("machine agent is terminating")

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func (c *upgradeWorkerContext) runUpgrades() error {
	from := version.Current
	from.Number = c.agentConfig.UpgradedToVersion()
	if from == version.Current {
		logger.Infof("upgrade to %v already completed.", version.Current)
		return nil
	}

	a := c.agent
	tag, ok := c.agentConfig.Tag().(names.MachineTag)
	if !ok {
		return errors.New("machine agent's tag is not a MachineTag")
	}
	machineId := tag.Id()

	isMaster, err := isMachineMaster(c.st, machineId)
	if err != nil {
		return errors.Trace(err)
	}

	var upgradeInfo *state.UpgradeInfo
	if c.isStateServer {
		upgradeInfo, err = c.st.EnsureUpgradeInfo(machineId, from.Number, version.Current.Number)
		if err != nil {
			return errors.Trace(err)
		}

		// State servers need to wait for other state servers to be
		// ready to run the upgrade.
		logger.Infof("waiting for other state servers to be ready for upgrade")
		if err := c.waitForOtherStateServers(upgradeInfo, isMaster); err != nil {
			if err == agentTerminating {
				logger.Warningf(`stopped waiting for other state servers: %v`, err)
			} else {
				logger.Errorf(`other state servers failed to come up for upgrade `+
					`to %s - aborting: %v`, version.Current, err)
				a.setMachineStatus(c.apiState, params.StatusError,
					fmt.Sprintf("upgrade to %v aborted while waiting for other "+
						"state servers: %v", version.Current, err))
				// If master, trigger a rollback to the previous agent version.
				if isMaster {
					logger.Errorf("downgrading environment agent version to %v due to aborted upgrade",
						from.Number)
					if rollbackErr := c.st.SetEnvironAgentVersion(from.Number); rollbackErr != nil {
						logger.Errorf("rollback failed: %v", rollbackErr)
						return errors.Annotate(rollbackErr, "failed to roll back desired agent version")
					}
				}
			}
			return err
		}
	}

	err = a.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		var upgradeErr error
		a.setMachineStatus(c.apiState, params.StatusStarted,
			fmt.Sprintf("upgrading to %v", version.Current))

		context := upgrades.NewContext(agentConfig, c.apiState, c.st)
		for _, job := range c.jobs {
			target := upgradeTarget(job, isMaster)
			if target == "" {
				continue
			}
			logger.Infof("starting upgrade from %v to %v for %v %q",
				from, version.Current, target, tag)

			attempts := getUpgradeRetryStrategy()
			for attempt := attempts.Start(); attempt.Next(); {
				upgradeErr = upgradesPerformUpgrade(from.Number, target, context)
				if upgradeErr == nil {
					break
				}
				if connectionIsDead(c.apiState) {
					// API connection has gone away - abort!
					return &apiLostDuringUpgrade{upgradeErr}
				}
				retryText := "will retry"
				if !attempt.HasNext() {
					retryText = "giving up"
				}
				logger.Errorf("upgrade from %v to %v for %v %q failed (%s): %v",
					from, version.Current, target, tag, retryText, upgradeErr)
				a.setMachineStatus(c.apiState, params.StatusError,
					fmt.Sprintf("upgrade to %v failed (%s): %v",
						version.Current, retryText, upgradeErr))
			}
		}
		if upgradeErr != nil {
			return upgradeErr
		}
		agentConfig.SetUpgradedToVersion(version.Current.Number)
		return nil
	})
	if err != nil {
		logger.Errorf("upgrade to %v failed: %v", version.Current, err)
		return err
	}

	if c.isStateServer {
		if isMaster {
			if err := upgradeInfo.SetStatus(state.UpgradeFinishing); err != nil {
				logger.Errorf("upgrade done but failed to set status: %v", err)
				return err
			}
		}
		if err := upgradeInfo.SetStateServerDone(machineId); err != nil {
			logger.Errorf("upgrade done but failed to synchronise: %v", err)
			return err
		}
	}

	logger.Infof("upgrade to %v completed successfully.", version.Current)
	a.setMachineStatus(c.apiState, params.StatusStarted, "")
	return nil
}

func (c *upgradeWorkerContext) waitForOtherStateServers(info *state.UpgradeInfo, isMaster bool) error {
	watcher := info.Watch()

	maxWait := getUpgradeStartTimeout(isMaster)
	timeout := time.After(maxWait)
	for {
		select {
		case <-watcher.Changes():
			if err := info.Refresh(); err != nil {
				return errors.Trace(err)
			}
			if isMaster {
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
			if isMaster {
				if err := info.Abort(); err != nil {
					return errors.Annotate(err, "unable to abort upgrade")
				}
			}
			return errors.Errorf("timed out after %s", maxWait)
		case <-c.agent.Dying():
			return agentTerminating
		}

	}
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
		return upgradeStartTimeoutMaster
	}
	return upgradeStartTimeoutSecondary
}

var openStateForUpgrade = func(
	agent upgradingMachineAgent,
	agentConfig agent.Config,
) (*state.State, error) {
	if err := agent.ensureMongoServer(agentConfig); err != nil {
		return nil, err
	}
	var err error
	info, ok := agentConfig.MongoInfo()
	if !ok {
		return nil, fmt.Errorf("no state info available")
	}
	st, err := state.Open(info, mongo.DefaultDialOpts(), environs.NewStatePolicy())
	if err != nil {
		return nil, err
	}
	return st, nil
}

var isMachineMaster = func(st *state.State, machineId string) (bool, error) {
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

func upgradeTarget(job params.MachineJob, isMaster bool) upgrades.Target {
	switch job {
	case params.JobManageEnviron:
		if isMaster {
			return upgrades.DatabaseMaster
		}
		return upgrades.StateServer
	case params.JobHostUnits:
		return upgrades.HostMachine
	}
	return ""
}
