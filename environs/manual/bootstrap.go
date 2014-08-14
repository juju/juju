// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/worker/localstorage"
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
func Bootstrap(args BootstrapArgs) (endpoints []network.Address, err error) {
	if args.Host == "" {
		return endpoints, errors.New("host argument is empty")
	}
	if args.Environ == nil {
		return endpoints, errors.New("environ argument is nil")
	}
	if args.DataDir == "" {
		return endpoints, errors.New("data-dir argument is empty")
	}
	if args.Series == "" {
		return endpoints, errors.New("series argument is empty")
	}
	if args.HardwareCharacteristics == nil {
		return endpoints, errors.New("hardware characteristics argument is empty")
	}
	if len(args.PossibleTools) == 0 {
		return endpoints, errors.New("possible tools is empty")
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		return endpoints, fmt.Errorf("failed to check provisioned status: %v", err)
	}
	if provisioned {
		return endpoints, ErrProvisioned
	}

	// Filter tools based on detected series/arch.
	logger.Infof("Filtering possible tools: %v", args.PossibleTools)
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *args.HardwareCharacteristics.Arch,
		Series: args.Series,
	})
	if err != nil {
		return endpoints, err
	}

	// If the tools are on the machine already, get a file:// scheme tools URL.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	bootstrapStorage := args.Environ.Storage()
	if url, _ := bootstrapStorage.URL(toolsStorageName); url == tools.URL {
		tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)
	}

	// Add the local storage configuration.
	agentEnv, err := localstorage.StoreConfig(args.Environ)
	if err != nil {
		return endpoints, err
	}

	privateKey, err := common.GenerateSystemSSHKey(args.Environ)
	if err != nil {
		return endpoints, err
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
		return endpoints, err
	}
	for k, v := range agentEnv {
		mcfg.AgentEnvironment[k] = v
	}
	err = provisionMachineAgent(args.Host, mcfg, args.Context.GetStderr())
	if err != nil {
		return nil, err
	}
	endpoints = network.NewAddresses(args.Host)
	return endpoints, nil
}
