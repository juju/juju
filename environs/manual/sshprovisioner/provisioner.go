// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner

import (
	"context"

	"github.com/juju/juju/environs/manual"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
)

var (
	logger = internallogger.GetLogger("juju.environs.manual.sshprovisioner")
)

// ProvisionMachine returns a new machineId and nil if the provision process is done successfully
// The func will manual provision a linux machine using as it's default protocol SSH
func ProvisionMachine(ctx context.Context, args manual.ProvisionMachineArgs) (machineId string, err error) {
	defer func() {
		if machineId != "" && err != nil {
			logger.Errorf(ctx, "provisioning failed, removing machine %v: %v", machineId, err)
			results, cleanupErr := args.Client.DestroyMachinesWithParams(ctx, false, false, false, nil, machineId)
			if cleanupErr == nil {
				cleanupErr = results[0].Error
			}
			if cleanupErr != nil {
				logger.Errorf(ctx, "error cleaning up machine: %s", cleanupErr)
			}
			machineId = ""
		}
	}()

	// Create the "ubuntu" user and initialise passwordless sudo. We populate
	// the ubuntu user's authorized_keys file with the public keys in the current
	// user's ~/.ssh directory. The authenticationworker will later update the
	// ubuntu user's authorized_keys.
	if err = InitUbuntuUser(args.Host, args.User,
		args.AuthorizedKeys, args.PrivateKey, args.Stdin, args.Stdout); err != nil {
		return "", err
	}

	machineParams, err := gatherMachineParams(args.Host, "ubuntu")
	if err != nil {
		return "", err
	}

	// Inform Juju that the machine exists.
	machineId, err = manual.RecordMachineInState(ctx, args.Client, *machineParams)
	if err != nil {
		return "", err
	}

	provisioningScript, err := args.Client.ProvisioningScript(ctx, params.ProvisioningScriptParams{
		MachineId:              machineId,
		Nonce:                  machineParams.Nonce,
		DisablePackageCommands: !args.EnableOSRefreshUpdate && !args.EnableOSUpgrade,
	})

	if err != nil {
		logger.Errorf(ctx, "cannot obtain provisioning script")
		return "", err
	}

	// Finally, provision the machine agent.
	err = runProvisionScript(provisioningScript, args.Host, args.Stderr)
	if err != nil {
		return machineId, err
	}

	logger.Infof(ctx, "Provisioned machine %v", machineId)
	return machineId, nil
}
