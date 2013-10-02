// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/worker/localstorage"
)

const BootstrapInstanceId = instance.Id(manualInstancePrefix)

// LocalStorageEnviron is an Environ where the bootstrap node
// manages its own local storage.
type LocalStorageEnviron interface {
	environs.Environ
	localstorage.LocalStorageConfig
}

type BootstrapArgs struct {
	Host          string
	DataDir       string
	Environ       LocalStorageEnviron
	MachineId     string
	PossibleTools tools.List
}

func errMachineIdInvalid(machineId string) error {
	return fmt.Errorf("%q is not a valid machine ID", machineId)
}

// NewManualBootstrapEnviron wraps a LocalStorageEnviron with another which
// overrides the Bootstrap method; when Bootstrap is invoked, the specified
// host will be manually bootstrapped.
func Bootstrap(args BootstrapArgs) (err error) {
	if args.Host == "" {
		return errors.New("host argument is empty")
	}
	if args.Environ == nil {
		return errors.New("environ argument is nil")
	}
	if args.DataDir == "" {
		return errors.New("data-dir argument is empty")
	}
	if !names.IsMachine(args.MachineId) {
		return errMachineIdInvalid(args.MachineId)
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		return fmt.Errorf("failed to check provisioned status: %v", err)
	}
	if provisioned {
		return ErrProvisioned
	}

	hc, series, err := detectSeriesAndHardwareCharacteristics(args.Host)
	if err != nil {
		return fmt.Errorf("error detecting hardware characteristics: %v", err)
	}

	// Filter tools based on detected series/arch.
	logger.Infof("Filtering possible tools: %v", args.PossibleTools)
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *hc.Arch,
		Series: series,
	})
	if err != nil {
		return err
	}

	// Store the state file. If provisioning fails, we'll remove the file.
	logger.Infof("Saving bootstrap state file to bootstrap storage")
	bootstrapStorage := args.Environ.Storage()
	err = common.SaveState(
		bootstrapStorage,
		&common.BootstrapState{
			StateInstances:  []instance.Id{BootstrapInstanceId},
			Characteristics: []instance.HardwareCharacteristics{hc},
		},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			logger.Errorf("bootstrapping failed, removing state file: %v", err)
			bootstrapStorage.Remove(common.StateFile)
		}
	}()

	// Get a file:// scheme tools URL for the tools, which will have been
	// copied to the remote machine's storage directory.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)

	// Add the local storage configuration.
	agentEnv := map[string]string{
		agent.StorageAddr:       args.Environ.StorageAddr(),
		agent.StorageDir:        storageDir,
		agent.SharedStorageAddr: args.Environ.SharedStorageAddr(),
		agent.SharedStorageDir:  args.Environ.SharedStorageDir(),
	}

	// Finally, provision the machine agent.
	stateFileURL := fmt.Sprintf("file://%s/%s", storageDir, common.StateFile)
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:          args.Host,
		dataDir:       args.DataDir,
		environConfig: args.Environ.Config(),
		stateFileURL:  stateFileURL,
		machineId:     args.MachineId,
		bootstrap:     true,
		nonce:         state.BootstrapNonce,
		tools:         &tools,
		agentEnv:      agentEnv,
	})
	return err
}
