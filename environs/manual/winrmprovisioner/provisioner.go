// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package winrmprovisioner

import (
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/manual"
)

var logger = loggo.GetLogger("juju.environs.manual.winrmprovisioner")

// ProvisionMachine returns a new machineId and nil if the provision process is done successfully
// The function will manual provision a windows machine using as comunication protocol WinRM(windows remote manager)
func ProvisionMachine(args manual.ProvisionMachineArgs) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			if cleanupErr := args.Client.ForceDestroyMachines(machineId); cleanupErr != nil {
				logger.Errorf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	if err = InitAdministratorUser(&args); err != nil {
		return "", errors.Annotatef(err,
			"Cannot provision machine because no WinRM http/https standard listener is enabled for user %q, on host %q",
			args.User, args.Host)
	}

	machineParams, err := gatherMachineParams(args.Host, args.WinRM.Client)
	if err != nil {
		return "", err
	}

	machineId, err = manual.RecordMachineInState(args.Client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := args.Client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId:              machineId,
		Nonce:                  machineParams.Nonce,
		DisablePackageCommands: true,
	})

	if err != nil {
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, args.WinRM.Client, args.Stdout, args.Stderr)
	if err != nil {
		return "", err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}
