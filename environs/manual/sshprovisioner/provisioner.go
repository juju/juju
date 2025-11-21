// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/rpc/params"
)

var (
	logger = loggo.GetLogger("juju.environs.manual.sshprovisioner")
)

// ProvisionMachine returns a new machineId and nil if the provision process is done successfully
// The func will manual provision a linux machine using as it's default protocol SSH
func ProvisionMachine(args manual.ProvisionMachineArgs) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf("provisioning failed, removing machine %v: %v", machineId, err)
			results, cleanupErr := args.Client.DestroyMachinesWithParams(false, false, false, nil, machineId)
			if cleanupErr == nil {
				cleanupErr = results[0].Error
			}
			if cleanupErr != nil {
				logger.Errorf("error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	// Optionally, ubuntuUserPrivateKey may be supplied to use when we connect to the
	// the machine as the ubuntu user (for provisioning check or hardware characteristics),
	// this is useful for API clients that manage their own SSH keys and the key isn't present
	// on the machine that is running this code. I.e., the juju terraform provider.
	ubuntuUserPrivateKey := args.UbuntuUserPrivateKey

	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	if err = InitUbuntuUser(args.Host, args.User,
		args.AuthorizedKeys, args.PrivateKey, ubuntuUserPrivateKey, args.Stdin, args.Stdout); err != nil {
		return "", err
	}

	machineParams, err := gatherMachineParams(args.Host, ubuntuUserPrivateKey)
	if err != nil {
		return "", err
	}

	// Inform Juju that the machine exists.
	machineId, err = manual.RecordMachineInState(args.Client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := args.Client.ProvisioningScript(params.ProvisioningScriptParams{
		MachineId:              machineId,
		Nonce:                  machineParams.Nonce,
		DisablePackageCommands: !args.EnableOSRefreshUpdate && !args.EnableOSUpgrade,
	})

	if err != nil {
		logger.Errorf("cannot obtain provisioning script")
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, args.Host, args.Stderr, ubuntuUserPrivateKey)
	if err != nil {
		return machineId, err
	}

	logger.Infof("Provisioned machine %v", machineId)
	return machineId, nil
}
