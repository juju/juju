// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"errors"
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/provider"
	localstorage "launchpad.net/juju-core/provider/local/storage"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/tools"
)

const BootstrapInstanceId = instance.Id(manualInstancePrefix)

// LocalStorageEnviron is an Environ where the bootstrap node
// manages its own local storage.
type LocalStorageEnviron interface {
	environs.Environ
	bootstrap.BootstrapStorage
	localstorage.LocalStorageConfig
}

type BootstrapArgs struct {
	Host          string
	Environ       LocalStorageEnviron
	MachineId     string
	PossibleTools tools.List
}

// TODO(axw) make this configurable?
const dataDir = "/var/lib/juju"

var errHostEmpty = errors.New("Host argument is empty")
var errEnvironNil = errors.New("Environ argument is nil")

func errMachineIdInvalid(machineId string) error {
	return fmt.Errorf("%q is not a valid machine ID", machineId)
}

// NewManualBootstrapEnviron wraps a LocalStorageEnviron with another which
// overrides the Bootstrap method; when Bootstrap is invoked, the specified
// host will manually bootstrapped.
func Bootstrap(args BootstrapArgs) (err error) {
	if args.Host == "" {
		return errHostEmpty
	}
	if args.Environ == nil {
		return errEnvironNil
	}
	if !names.IsMachine(args.MachineId) {
		return errMachineIdInvalid(args.MachineId)
	}

	provisioned, err := checkProvisioned(args.Host)
	if err != nil {
		err = fmt.Errorf("error checking if provisioned: %v", err)
		return err
	}
	if provisioned {
		return ErrProvisioned
	}

	bootstrapStorage, err := args.Environ.BootstrapStorage()
	if err != nil {
		return err
	}

	var savedState bool
	defer func() {
		if savedState && err != nil {
			logger.Errorf("bootstrapping failed, removing state file: %v", err)
			bootstrapStorage.Remove(provider.StateFile)
		}
	}()

	hc, series, err := detectSeriesAndHardwareCharacteristics(args.Host)
	if err != nil {
		err = fmt.Errorf("error detecting hardware characteristics: %v", err)
		return err
	}

	// Filter tools based on detected series/arch.
	possibleTools, err := args.PossibleTools.Match(tools.Filter{
		Arch:   *hc.Arch,
		Series: series,
	})
	if err != nil {
		return err
	}

	// Store the state file. If provisioning fails, we'll remove the file.
	err = provider.SaveState(
		bootstrapStorage,
		&provider.BootstrapState{
			StateInstances:  []instance.Id{BootstrapInstanceId},
			Characteristics: []instance.HardwareCharacteristics{hc},
		},
	)
	if err != nil {
		return err
	}
	savedState = true

	// Get a file:// scheme tools URL for the tools, which will have been
	// copied to the remote machine's storage directory.
	tools := *possibleTools[0]
	storageDir := args.Environ.StorageDir()
	toolsStorageName := envtools.StorageName(tools.Version)
	tools.URL = fmt.Sprintf("file://%s/%s", storageDir, toolsStorageName)

	// Add the local storage configuration.
	machineenv := map[string]string{
		osenv.JujuStorageAddr:       args.Environ.StorageAddr(),
		osenv.JujuStorageDir:        storageDir,
		osenv.JujuSharedStorageAddr: args.Environ.SharedStorageAddr(),
		osenv.JujuSharedStorageDir:  args.Environ.SharedStorageDir(),
	}

	// Finally, provision the machine agent.
	stateFileURL := fmt.Sprintf("file://%s/%s", storageDir, provider.StateFile)
	err = provisionMachineAgent(provisionMachineAgentArgs{
		host:         args.Host,
		dataDir:      dataDir,
		envcfg:       args.Environ.Config(),
		stateFileURL: stateFileURL,
		machineId:    args.MachineId,
		bootstrap:    true,
		nonce:        state.BootstrapNonce,
		tools:        &tools,
		machineenv:   machineenv,
	})
	if err != nil {
		return err
	}

	// TODO(axw) connect to bootstrapped machine's state server,
	// get the *state.Machine?
	return nil
}
