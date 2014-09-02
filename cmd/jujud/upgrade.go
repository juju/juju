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
}

var upgradesPerformUpgrade = upgrades.PerformUpgrade // Allow patching for tests

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
	tag := c.agentConfig.Tag().(names.MachineTag)

	isMaster, err := isMachineMaster(c.st, tag)
	if err != nil {
		return errors.Trace(err)
	}

	if c.isStateServer {
		// State servers need to wait for other state servers to be
		// ready to run the upgrade.
		if err := waitForOtherStateServers(c.st, isMaster); err != nil {
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
					return errors.Annotate(rollbackErr, "failed to roll back desired agent version")
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

	logger.Infof("upgrade to %v completed successfully.", version.Current)
	a.setMachineStatus(c.apiState, params.StatusStarted, "")
	return nil
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

var isMachineMaster = func(st *state.State, tag names.MachineTag) (bool, error) {
	if st == nil {
		// If there is no state, we aren't a master.
		return false, nil
	}
	// Not calling the agent openState method as it does other checks
	// we really don't care about here.  All we need here is the machine
	// so we can determine if we are the master or not.
	machine, err := st.Machine(tag.Id())
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

var waitForOtherStateServers = func(st *state.State, isMaster bool) error {
	if wrench.IsActive("machine-agent", "fail-state-server-upgrade-wait") {
		return errors.New("failing other state servers check due to wrench")
	}
	// TODO(mjs) - for now, assume that the other state servers are
	// ready. This function will be fleshed out once the UpgradeInfo
	// work is done.
	return nil
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
