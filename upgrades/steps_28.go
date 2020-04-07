// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v3"
	"gopkg.in/yaml.v2"

	"github.com/juju/collections/set"
	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/upgradesteps"
	"github.com/juju/juju/apiserver/params"
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
			description: "write unit agent state to controller for all running units and remove files",
			targets:     []Target{HostMachine},
			run:         moveUnitAgentStateToController,
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

func moveUnitAgentStateToController(ctx Context) error {
	// Lookup the names of all unit agents installed on this machine.
	agentConf := ctx.AgentConfig()
	ctxTag := agentConf.Tag()
	if ctxTag.Kind() != names.MachineTagKind {
		return nil
	}
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
	//
	// TODO: hml 30-mar-2020
	// Once the final piece of the upgrade is done, this can be changed to
	// remove:
	//     /var/lib/juju/agents/unit-ubuntu-0/state/uniter
	//     /var/lib/juju/agents/unit-ubuntu-0/state/storage
	//     /var/lib/juju/agents/unit-ubuntu-0/state/relations
	//     /var/lib/juju/agents/unit-ubuntu-0/state/metrics
	// and the caas operator file.  Any not found is fine.
	// Keep /var/lib/juju/agents/unit-ubuntu-0/state for non persistent needs.
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
	WriteAgentState([]params.SetUnitStateArg) error
}

func iaasUniterState(ctx Context, unitNames []string) ([]string, error) {
	// Read the uniter state for each unit on this machine.
	// Leave in yaml format as a string to push to the controller.
	fileNames := set.NewStrings()
	uniterStates := make([]params.SetUnitStateArg, len(unitNames))
	dataDir := ctx.AgentConfig().DataDir()
	for i, unitName := range unitNames {
		tag, err := names.ParseUnitTag(unitName)
		if err != nil {
			return nil, errors.Annotatef(err, "unable to parse unit agent tag %q", unitName)
		}

		uniterStates[i].Tag = tag.String()
		// e.g. /var/lib/juju/agents/unit-ubuntu-0/state/uniter
		uniterStateDir := filepath.Join(agent.BaseDir(dataDir), unitName, "state")

		uniterSt, filename, err := readUniterState(uniterStateDir)
		if err != nil && !os.IsNotExist(err) {
			// Note: we may want to error on NotExist.  Something is very
			// broken if the uniter state files does not exist.  On the
			// other hand, all that will happen is that the uniter will act
			// like the unit is just starting with regards to hook execution.
			// With properly written idempotent charms, this is not an issue.
			return nil, err
		} else if err == nil {
			fileNames.Add(filename)
			uniterStates[i].UniterState = &uniterSt
		}

		// TODO(achilleasa): migrate relation data.

		storageData, err := readStorageState(uniterStateDir)
		if err != nil && !os.IsNotExist(err) && !errors.IsNotFound(err) {
			return nil, err
		} else if err == nil {
			uniterStates[i].StorageState = &storageData.dataString
		}
		if storageData.filename != "" {
			fileNames.Add(storageData.filename)
		}

		// NOTE(achilleasa): meter status is transparently migrated to
		// the controller by the meterstatus worker so we don't need
		// to do anything special here.
	}

	// No state files, nothing to do.
	if fileNames.IsEmpty() {
		return nil, nil
	}

	client := getUpgradeStepsClient(ctx.APIState())
	if err := client.WriteAgentState(uniterStates); err != nil {
		return nil, errors.Annotatef(err, "unable to set state for units %q", strings.Join(unitNames, ", "))
	}
	return fileNames.Values(), nil
}

// readUniterState reads uniter state, validates it, then returns a
// yaml string of the dataString and the filename.  If no error is returned,
// the yaml string is valid and non empty.
func readUniterState(uniterStateDir string) (string, string, error) {
	uniterStateFile := filepath.Join(uniterStateDir, "uniter")
	// Read into a State so we can validate the dataString.
	var st operation.State
	if err := utils.ReadYaml(uniterStateFile, &st); err != nil {
		// No file available, move on.
		if os.IsNotExist(err) {
			return "", "", err
		}
		return "", "", errors.Annotatef(err, "failed to read uniter state from %q", uniterStateFile)
	}
	if err := st.Validate(); err != nil {
		return "", "", errors.Annotatef(err, "failed to validate uniter state from %q", uniterStateFile)
	}

	data, err := yaml.Marshal(st)
	if err != nil {
		return "", "", errors.Annotatef(err, "failed to unmarshal uniter state from %q", uniterStateFile)
	}

	return string(data), uniterStateFile, nil
}

func caasOperatorLocalState(ctx Context, unitNames []string) ([]string, error) {
	// TODO: (hml) 27-Mar-2020.
	// Implement when relations, storage, metrics and the operator state are moved.
	return nil, nil
}

type data struct {
	dataString string
	filename   string
}

func readStorageState(uniterStateDir string) (data, error) {
	storageStateDir := filepath.Join(uniterStateDir, "storage")
	storage, err := readAllStorageStateFiles(storageStateDir)
	if err != nil {
		return data{}, err
	}
	if len(storage) < 1 {
		return data{filename: storageStateDir}, errors.NotFoundf("storage state")
	}
	dataYaml, err := yaml.Marshal(storage)
	if err != nil {
		return data{}, errors.Annotatef(err, "failed to unmarshal storage state from %q", storageStateDir)
	}
	return data{dataString: string(dataYaml), filename: storageStateDir}, nil
}

// readAllStorageStateFiles loads and returns every storage stateFile persisted inside
func readAllStorageStateFiles(dirPath string) (map[string]bool, error) {
	if _, err := os.Stat(dirPath); err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	files := make(map[string]bool)
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		storageId := fi.Name()
		i := strings.LastIndex(storageId, "-")
		if i <= 0 {
			// Lack of "-" means it's not a valid storage ID.
			continue
		}
		storageId = storageId[:i] + "/" + storageId[i+1:]
		if !names.IsValidStorage(storageId) {
			logger.Warningf("ignoring storage file with invalid name %q", filepath.Join(dirPath, fi.Name()))
			continue
		}
		tag := names.NewStorageTag(storageId)
		f, err := readStorageStateFile(dirPath, tag)
		if err != nil {
			return nil, err
		}
		files[tag.Id()] = f
	}
	return files, nil
}

// readStorageStateFile loads a storage stateFile from the subdirectory of dirPath named
// is returned.
func readStorageStateFile(dirPath string, tag names.StorageTag) (bool, error) {
	filename := strings.Replace(tag.Id(), "/", "-", -1)
	filePath := filepath.Join(dirPath, filename)

	var info storageDiskInfo
	if err := utils.ReadYaml(filePath, &info); err != nil {
		return false, errors.Annotatef(err, "cannot read from storage state file %q", filePath)
	}
	if info.Attached == nil {
		return false, errors.Errorf("invalid storage state file %q: missing 'attached'", filePath)
	}
	return *info.Attached, nil
}

type storageDiskInfo struct {
	Attached *bool `yaml:"attached,omitempty"`
}
