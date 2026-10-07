// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/controllernode"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
)

// FinaliseIAASAgent records the password and provisioned instance for the named
// machine. The caller supplies the agent nonce for that machine.
func FinaliseIAASAgent(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	machineService MachineService,
	machineName machine.Name,
	machineNonce string,
	bootstrapParams instancecfg.StateInitializationParams,
	agentPassword string,
) error {
	// Locate the machine whose instance data will be recorded.
	machineUUID, err := machineService.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Capture(err)
	}

	// Set the agent password for the machine.
	if err := agentPasswordService.SetMachinePassword(ctx, machineName, agentPassword); err != nil {
		return errors.Capture(err)
	}

	// If this data exists, we consider the machine as provisioned.
	if err := machineService.SetMachineCloudInstance(
		ctx,
		machineUUID,
		bootstrapParams.BootstrapMachineInstanceId,
		bootstrapParams.BootstrapMachineDisplayName,
		machineNonce,
		bootstrapParams.BootstrapMachineHardwareCharacteristics,
	); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// FinaliseK8sAgent sets the controller password and reads its introduction
// nonce from nonceFilePath. Nonce initialisation is skipped if the file cannot
// be read, preserving the behaviour for controllers without that file.
func FinaliseK8sAgent(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	controllerID, agentPassword, nonceFilePath string,
) error {
	// Set the controller node password.
	if err := agentPasswordService.SetControllerNodePassword(ctx, controllerID, agentPassword); err != nil {
		return errors.Capture(err)
	}

	// Read the introduction nonce from disk. It is written by the
	// controller-config-seed init container from the ConfigMap.
	// If the nonce file is missing (e.g. non-k8s bootstrap or older
	// charm), skip silently. The UnitIntroduction facade will reject
	// missing nonces for controller applications.
	nonceBytes, err := os.ReadFile(nonceFilePath)
	if err != nil {
		return nil
	}
	nonce := string(nonceBytes)
	if _, err := agentPasswordService.EnsureControllerNodeNonce(ctx, controllerID, nonce); err != nil {
		return errors.Capture(err)
	}

	return nil
}

// InitialiseAPIHostPorts publishes API addresses for controllerID, resolving
// their spaces and honouring the configured management space.
func InitialiseAPIHostPorts(
	ctx context.Context,
	controllerNodeService ControllerNodeService,
	networkService NetworkService,
	controllerID string,
	controllerConfig controller.Config,
	pAddrs network.ProviderAddresses,
	apiPort int,
) error {
	allSpaces, err := networkService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	addrs, err := pAddrs.ToSpaceAddresses(allSpaces)
	if err != nil {
		return errors.Capture(err)
	}
	hostPorts := network.SpaceAddressesWithPort(addrs, apiPort)

	mgmtSpaceCfg := controllerConfig.JujuManagementSpace()
	mgmtSpace, err := networkService.SpaceByName(ctx, mgmtSpaceCfg)
	if err != nil && !errors.Is(err, networkerrors.SpaceNotFound) {
		return errors.Capture(err)
	}

	args := controllernode.SetAPIAddressArgs{
		MgmtSpace: mgmtSpace,
		APIAddresses: map[string]network.SpaceHostPorts{
			controllerID: hostPorts,
		},
	}
	return errors.Capture(controllerNodeService.SetAPIAddresses(ctx, args))
}
