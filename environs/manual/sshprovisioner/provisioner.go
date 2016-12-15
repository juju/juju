// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/manual"
)

var (
	logger = loggo.GetLogger("juju.environs.manual.sshprovisioner")
)

// Provision returns a new machineId and nil if the provision process is done successfully
// The func will manual provision a linux machine using as it's default protocol SSH
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

	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	if err = InitUbuntuUser(args.Host, args.User,
		args.AuthorizedKeys, args.Stdin, args.Stdout); err != nil {
		return "", err
	}

	machineParams, err := gatherMachineParams(args.Host)
	if err != nil {
		return "", err
	}

	// Inform Juju that the machine exists.
	machineId, err = manual.RecordMachineInState(args.Client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := args.Client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId: machineId,
		Nonce:     machineParams.Nonce,
		DisablePackageCommands: !args.EnableOSRefreshUpdate && !args.EnableOSUpgrade,
	})

	if err != nil {
		logger.Errorf("cannot obtain provisioning script")
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, args.Host, args.Stderr)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}
