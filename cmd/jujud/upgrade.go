package main

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/upgrades"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker"
)

type upgradingMachineAgent interface {
	ensureMongoServer(agent.Config) error
	setMachineStatus(*api.State, params.Status, string) error
	CurrentConfig() agent.Config
	ChangeConfig(AgentConfigMutator) error
}

var upgradesPerformUpgrade = upgrades.PerformUpgrade // Allow patching for tests

func NewUpgradeWorkerFactory(agent upgradingMachineAgent) *upgradeWorkerFactory {
	return &upgradeWorkerFactory{
		agent:           agent,
		upgradeComplete: make(chan struct{}),
	}
}

type upgradeWorkerFactory struct {
	agent           upgradingMachineAgent
	upgradeComplete chan struct{}
}

func (f *upgradeWorkerFactory) Worker(
	apiState *api.State,
	jobs []params.MachineJob,
) worker.Worker {
	return worker.NewSimpleWorker(func(stop <-chan struct{}) error {
		select {
		case <-f.upgradeComplete:
			// Our work is already done (we're probably being restarted
			// because the API connection has gone down), so do nothing.
			return nil
		default:
		}

		agentConfig := f.agent.CurrentConfig()

		// If the machine agent is a state server, flag that state
		// needs to be opened before running upgrade steps
		needsState := false
		for _, job := range jobs {
			if job == params.JobManageEnviron {
				needsState = true
			}
		}
		// We need a *state.State for upgrades. We open it independently
		// of StateWorker, because we have no guarantees about when
		// and how often StateWorker might run.
		var st *state.State
		if needsState {
			if err := f.agent.ensureMongoServer(agentConfig); err != nil {
				return err
			}
			var err error
			info, ok := agentConfig.MongoInfo()
			if !ok {
				return fmt.Errorf("no state info available")
			}
			st, err = state.Open(info, mongo.DialOpts{}, environs.NewStatePolicy())
			if err != nil {
				return err
			}
			defer st.Close()
		}
		if err := runUpgrades(f.agent, st, apiState, jobs, agentConfig); err == nil {
			// Only signal that the upgrade is complete if no error
			// was returned.
			close(f.upgradeComplete)
		} else if !isFatal(err) {
			// Only non-fatal errors are returned (this will trigger
			// the worker to be restarted).
			//
			// Fatal upgrade errors are not returned because user
			// intervention is required at that point. We don't want
			// the upgrade worker or the agent to restart. Status
			// output and the logs will report that the upgrade has
			// failed.
			return err
		}
		return nil
	})
}

func (f *upgradeWorkerFactory) UpgradeComplete() chan struct{} {
	return f.upgradeComplete
}

// runUpgrades runs the upgrade operations for each job type and
// updates the updatedToVersion on success.
func runUpgrades(
	a upgradingMachineAgent,
	st *state.State,
	apiState *api.State,
	jobs []params.MachineJob,
	agentConfig agent.Config,
) error {
	from := version.Current
	from.Number = agentConfig.UpgradedToVersion()
	if from == version.Current {
		logger.Infof("upgrade to %v already completed.", version.Current)
		return nil
	}

	tag := agentConfig.Tag().(names.MachineTag)

	isMaster, err := isMachineMaster(st, tag)
	if err != nil {
		return errors.Trace(err)
	}

	err = a.ChangeConfig(func(agentConfig agent.ConfigSetter) error {
		var upgradeErr error
		a.setMachineStatus(apiState, params.StatusStarted,
			fmt.Sprintf("upgrading to %v", version.Current))
		context := upgrades.NewContext(agentConfig, apiState, st)
		for _, job := range jobs {
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
				} else {
					retryText := "will retry"
					if !attempt.HasNext() {
						retryText = "giving up"
					}
					logger.Errorf("upgrade from %v to %v for %v %q failed (%s): %v",
						from, version.Current, target, tag, retryText, upgradeErr)
					a.setMachineStatus(apiState, params.StatusError,
						fmt.Sprintf("upgrade to %v failed (%s): %v",
							version.Current, retryText, upgradeErr))
				}
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
		return &fatalError{err.Error()}
	}

	logger.Infof("upgrade to %v completed successfully.", version.Current)
	a.setMachineStatus(apiState, params.StatusStarted, "")
	return nil
}

func isMachineMaster(st *state.State, tag names.MachineTag) (bool, error) {
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
