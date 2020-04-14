// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
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
		logger.Infof("skipping agent %q, not a machine", ctxTag.String())
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

	fileNames, err := uniterState(ctx, unitNames)
	if err != nil {
		return err
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
	WriteAgentState([]params.SetUnitStateArg) error
}

func uniterState(ctx Context, unitNames []string) ([]string, error) {
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

		storageData, err := readStorageState(uniterStateDir)
		if err != nil && !errors.IsNotFound(err) {
			return nil, err
		} else if err == nil {
			uniterStates[i].StorageState = &storageData.dataString
		}
		if storageData.filename != "" {
			fileNames.Add(storageData.filename)
		}

		relationData, err := readRelationState(uniterStateDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		} else if err == nil && len(relationData.data) > 0 {
			uniterStates[i].RelationState = &relationData.data
			fileNames.Add(relationData.filename)
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

type storageData struct {
	dataString string
	filename   string
}

func readStorageState(uniterStateDir string) (storageData, error) {
	storageStateDir := filepath.Join(uniterStateDir, "storage")
	storage, err := readAllStorageStateFiles(storageStateDir)
	if err != nil {
		return storageData{}, err
	}
	if len(storage) < 1 {
		return storageData{filename: storageStateDir}, errors.NotFoundf("storage state")
	}
	dataYaml, err := yaml.Marshal(storage)
	if err != nil {
		return storageData{}, errors.Annotatef(err, "failed to unmarshal storage state from %q", storageStateDir)
	}
	return storageData{dataString: string(dataYaml), filename: storageStateDir}, nil
}

// readAllStorageStateFiles loads and returns every storage stateFile persisted inside
func readAllStorageStateFiles(dirPath string) (map[string]bool, error) {
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return nil, errors.NotFoundf("storage state file")
		}
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
		if os.IsNotExist(err) {
			return false, errors.NotFoundf("storage state file %q", filePath)
		}
		return false, errors.Annotatef(err, "cannot read from storage state file %q", filePath)
	}
	if info.Attached == nil {
		return false, errors.Errorf("invalid storage state file %q: missing 'attached'", filePath)
	}
	return *info.Attached, nil
}

// storageDiskInfo defines the storage unit storageData serialization.
type storageDiskInfo struct {
	Attached *bool `yaml:"attached,omitempty"`
}

type relationData struct {
	data     map[int]string
	filename string
}

func readRelationState(uniterStateDir string) (relationData, error) {
	relationStateDir := filepath.Join(uniterStateDir, "relations")

	rSt, err := readAllRelationStateDirs(relationStateDir)
	if err != nil {
		return relationData{}, err
	}
	retData := relationData{data: make(map[int]string, len(rSt)), filename: relationStateDir}
	for k, v := range rSt {
		dataYaml, err := yaml.Marshal(v)
		if err != nil {
			return relationData{}, errors.Annotatef(err, "failed to unmarshal relation state from %q", relationStateDir)
		}
		retData.data[k] = string(dataYaml)
	}
	return retData, nil
}

// readAllRelationStateDirs loads and returns every relationState persisted directly inside
// the supplied dirPath. If dirPath does not exist, no error is returned.
func readAllRelationStateDirs(dirPath string) (dirs map[int]*relationState, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot load relations state from %q", dirPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	dirs = map[int]*relationState{}
	for _, fi := range fis {
		// Entries with integer names must be directories containing StateDir
		// storageData; all other names will be ignored.
		relationId, err := strconv.Atoi(fi.Name())
		if err != nil {
			// This doesn't look like a relation.
			continue
		}
		rState, err := readRelationStateDir(filepath.Join(dirPath, fi.Name()), relationId)
		if err != nil {
			return nil, err
		}
		dirs[relationId] = rState
	}
	return dirs, nil
}

// readRelationStateDir loads a relationState from the subdirectory of dirPath named
// for the supplied RelationId. If the directory does not exist, no error
// is returned,
func readRelationStateDir(dirPath string, relationId int) (d *relationState, err error) {
	d = &relationState{
		RelationId:         relationId,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	defer errors.DeferredAnnotatef(&err, "cannot load relation state from %q", dirPath)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return d, nil
	} else if err != nil {
		return nil, err
	}
	fis, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	for _, fi := range fis {
		// Entries with names ending in "-" followed by an integer must be
		// files containing valid unit data; all other names are ignored.
		name := fi.Name()
		i := strings.LastIndex(name, "-")
		if i == -1 {
			continue
		}
		svcName := name[:i]
		unitId := name[i+1:]
		isApp := false
		unitOrAppName := ""
		if unitId == "app" {
			isApp = true
			unitOrAppName = svcName
		} else {
			if _, err := strconv.Atoi(unitId); err != nil {
				continue
			}
			unitOrAppName = svcName + "/" + unitId
		}
		var info relationDiskInfo
		if err = utils.ReadYaml(filepath.Join(dirPath, name), &info); err != nil {
			return nil, fmt.Errorf("invalid unit file %q: %v", name, err)
		}
		if info.ChangeVersion == nil {
			return nil, fmt.Errorf(`invalid unit file %q: "changed-version" not set`, name)
		}
		if isApp {
			d.ApplicationMembers[unitOrAppName] = *info.ChangeVersion
		} else {
			d.Members[unitOrAppName] = *info.ChangeVersion
		}
		if info.ChangedPending {
			if d.ChangedPending != "" {
				return nil, fmt.Errorf("%q and %q both have pending changed hooks", d.ChangedPending, unitOrAppName)
			}
			d.ChangedPending = unitOrAppName
		}
	}
	return d, nil
}

type relationState struct {
	RelationId         int              `yaml:"id"`
	Members            map[string]int64 `yaml:"members,omitempty"`
	ApplicationMembers map[string]int64 `yaml:"application-members,omitempty"`
	ChangedPending     string           `yaml:"changed-pending,omitempty"`
}

// relationDiskInfo defines the relation unit storageData serialization.
type relationDiskInfo struct {
	ChangeVersion  *int64 `yaml:"change-version"`
	ChangedPending bool   `yaml:"changed-pending,omitempty"`
}
