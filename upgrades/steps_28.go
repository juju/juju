// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgradesteps"
	k8sprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/service"
	"github.com/juju/juju/worker/common/reboot"
	"github.com/juju/juju/worker/uniter/operation"
)

// stateStepsFor28 returns upgrade steps for Juju 2.8.0.
func stateStepsFor28() []Step {
	return []Step{
		&upgradeStep{
			description: "drop old presence database",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().DropPresenceDatabase()
			},
		},
		&upgradeStep{
			description: "increment tasks sequence by 1",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().IncrementTasksSequence()
			},
		},
		&upgradeStep{
			description: "add machine ID to subordinate units",
			targets:     []Target{DatabaseMaster},
			run: func(context Context) error {
				return context.State().AddMachineIDToSubordinates()
			},
		},
	}
}

// stepsFor28 returns upgrade steps for Juju 2.8.0.
func stepsFor28() []Step {
	return []Step{
		// This step pre-populates the reboot-handled flag for all
		// running units so they do not accidentally trigger a start
		// hook once they get restarted after the upgrade is complete.
		&upgradeStep{
			description: "ensure currently running units do not fire start hooks thinking a reboot has occurred",
			targets:     []Target{HostMachine},
			run:         prepopulateRebootHandledFlagsForDeployedUnits,
		},
		&upgradeStep{
			description: "write uniter state to controller for all running units and remove files",
			targets:     []Target{HostMachine},
			run:         moveUniterStateToController,
		},
	}
}

func prepopulateRebootHandledFlagsForDeployedUnits(ctx Context) error {
	// Lookup the names of all unit agents installed on this machine.
	agentConf := ctx.AgentConfig()
	_, unitNames, _, err := service.FindAgents(agentConf.DataDir())
	if err != nil {
		return errors.Annotate(err, "looking up unit agents")
	}

	// Pre-populate reboot-handled flag files.
	monitor := reboot.NewMonitor(agentConf.TransientDataDir())
	for _, unitName := range unitNames {
		// This should never fail as it is already validated by the
		// FindAgents call. However, since this is an upgrade step
		// it's fine to be a bit extra paranoid.
		tag, err := names.ParseTag(unitName)
		if err != nil {
			return errors.Annotatef(err, "unable to parse unit agent tag %q", unitName)
		}

		// Querying the reboot monitor will set up the flag for this
		// unit if it doesn't already exist.
		if _, err = monitor.Query(tag); err != nil {
			return errors.Annotatef(err, "querying reboot monitor for %q", unitName)
		}
	}
	return nil
}

func moveUniterStateToController(ctx Context) error {
	// Lookup the names of all unit agents installed on this machine.
	agentConf := ctx.AgentConfig()
	_, unitNames, _, err := service.FindAgents(agentConf.DataDir())
	if err != nil {
		return errors.Annotate(err, "looking up unit agents")
	}
	if len(unitNames) == 0 {
		// No units, nothing to do.
		return nil
	}

	var fileNames []string
	if ctx.AgentConfig().Value(agent.ProviderType) == k8sprovider.CAASProviderType {
		if fileNames, err = caasOperatorLocalState(ctx, unitNames); err != nil {
			return err
		}
	} else {
		if fileNames, err = iaasUniterState(ctx, unitNames); err != nil {
			return err
		}
	}

	// Saving uniter state in the controller succeeded, now clean up
	// the now unused state files.
	for _, name := range fileNames {
		if err = os.RemoveAll(name); err != nil {
			// Log the error, but no need to fail the upgrade.  Juju
			// will not use the file any longer.
			logger.Errorf("failed to remove %q: %s", name, err)
		}
	}

	return nil
}

// getUpgradeStepsClient is to facilitate mocking the unit tests.
var getUpgradeStepsClient = func(caller base.APICaller) UpgradeStepsClient {
	return upgradesteps.NewClient(caller)
}

//go:generate mockgen -package mocks -destination mocks/upgradestepsclient_mock.go github.com/juju/juju/upgrades UpgradeStepsClient
type UpgradeStepsClient interface {
	WriteUniterState(uniterStates map[names.Tag]string) error
}

func iaasUniterState(ctx Context, unitNames []string) ([]string, error) {
	// Read the uniter state for each unit on this machine.
	// Leave in yaml format as a string to push to the controller.
	fileNames := []string{}
	uniterStates := make(map[names.Tag]string, len(unitNames))
	dataDir := ctx.AgentConfig().DataDir()
	for _, unitName := range unitNames {
		tag, err := names.ParseUnitTag(unitName)
		if err != nil {
			return nil, errors.Annotatef(err, "unable to parse unit agent tag %q", unitName)
		}

		// e.g. /var/lib/juju/agents/unit-ubuntu-0/state/uniter
		uniterStateFile := filepath.Join(agent.BaseDir(dataDir), unitName, "state", "uniter")

		// Read into a State as we can validate the data.
		var st operation.State
		if err = utils.ReadYaml(uniterStateFile, &st); err != nil {
			// No file available, move on.
			if os.IsNotExist(err) {
				continue
			}
			return nil, errors.Annotatef(err, "failed to read uniter state from %q", uniterStateFile)
		}
		fileNames = append(fileNames, uniterStateFile)
		if err = st.Validate(); err != nil {
			return nil, errors.Annotatef(err, "failed to validate uniter state from %q", uniterStateFile)
		}

		data, err := yaml.Marshal(st)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to unmarshal uniter state from %q", uniterStateFile)
		}
		uniterStates[tag] = string(data)
	}

	// No state files, nothing to do.
	if len(fileNames) == 0 {
		return nil, nil
	}

	client := getUpgradeStepsClient(ctx.APIState())
	if err := client.WriteUniterState(uniterStates); err != nil {
		return nil, errors.Annotatef(err, "unable to set state for units %q", strings.Join(unitNames, ", "))
	}
	return fileNames, nil
}

func caasOperatorLocalState(ctx Context, unitNames []string) ([]string, error) {
	// TODO: (hml) 27-Mar-2020.
	// Implement when relations, storage, metrics and the operator state are moved.
	return nil, nil
}
