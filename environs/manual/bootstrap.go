// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
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
	Host                    string
	DataDir                 string
	Environ                 LocalStorageEnviron
	PossibleTools           tools.List
	Context                 environs.BootstrapContext
	Series                  string
	HardwareCharacteristics *instance.HardwareCharacteristics
}

func errMachineIdInvalid(machineId string) error {
	return fmt.Errorf("%q is not a valid machine ID", machineId)
}

// NewManualBootstrapEnviron wraps a LocalStorageEnviron with another which
// overrides the Bootstrap method; when Bootstrap is invoked, the specified
// host will be manually bootstrapped.
//
// InitUbuntuUser is expected to have been executed successfully against
// the host being bootstrapped.
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
	if args.Series == "" {
		return errors.New("series argument is empty")
	}
	if args.HardwareCharacteristics == nil {
		return errors.New("hardware characteristics argument is empty")
	}
	if len(args.PossibleTools) == 0 {
		return errors.New("possible tools is empty")
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		return fmt.Errorf("failed to check provisioned status: %v", err)
	}
	if provisioned {
		return ErrProvisioned
	}

	// Filter tools based on detected series/arch.
	logger.Infof("Filtering possible tools: %v", args.PossibleTools)
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *args.HardwareCharacteristics.Arch,
		Series: args.Series,
	})
	if err != nil {
		return err
	}

	// Store the state file. If provisioning fails, we'll remove the file.
	logger.Infof("Saving bootstrap state file to bootstrap storage")
	bootstrapStorage := args.Environ.Storage()
	err = bootstrap.SaveState(
		bootstrapStorage,
		&bootstrap.BootstrapState{
			StateInstances: []instance.Id{BootstrapInstanceId},
		},
	)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			logger.Errorf("bootstrapping failed, removing state file: %v", err)
			bootstrapStorage.Remove(bootstrap.StateFile)
		}
	}()

	// If the tools are on the machine already, get a file:// scheme tools URL.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	if url, _ := bootstrapStorage.URL(toolsStorageName); url == tools.URL {
		tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)
	}

	// Add the local storage configuration.
	agentEnv, err := localstorage.StoreConfig(args.Environ)
	if err != nil {
		return err
	}

	privateKey, err := common.GenerateSystemSSHKey(args.Environ)
	if err != nil {
		return err
	}

	// Finally, provision the machine agent.
	mcfg := environs.NewBootstrapMachineConfig(privateKey)
	mcfg.InstanceId = BootstrapInstanceId
	mcfg.HardwareCharacteristics = args.HardwareCharacteristics
	if args.DataDir != "" {
		mcfg.DataDir = args.DataDir
	}
	mcfg.Tools = &tools
	err = environs.FinishMachineConfig(mcfg, args.Environ.Config(), constraints.Value{})
	if err != nil {
		return err
	}
	for k, v := range agentEnv {
		mcfg.AgentEnvironment[k] = v
	}
	return provisionMachineAgent(args.Host, mcfg, args.Context.GetStderr())
}
