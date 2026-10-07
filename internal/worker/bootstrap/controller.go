// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"sort"
	"strconv"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
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

// InitialiseAPIHostPorts publishes the initial API address projection for the
// supplied controller.
func InitialiseAPIHostPorts(
	ctx context.Context,
	controllerNodeService ControllerNodeService,
	networkService NetworkService,
	serviceManagerGetter ServiceManagerGetterFunc,
	controllerID string,
	modelType coremodel.ModelType,
	controllerConfig controller.Config,
	providerAddresses network.ProviderAddresses,
	apiPort int,
) error {
	spaces, err := networkService.GetAllSpaces(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	addresses, err := providerAddresses.ToSpaceAddresses(spaces)
	if err != nil {
		return errors.Capture(err)
	}

	clients := orderBootstrapAddresses(addresses, network.ScopePublic)
	agents := orderBootstrapAddresses(addresses, network.ScopeCloudLocal)
	peers := agents

	switch modelType {
	case coremodel.IAAS:
		managementSpaceName := controllerConfig.JujuManagementSpace()
		managementSpace, err := networkService.SpaceByName(ctx, managementSpaceName)
		if err != nil && !errors.Is(err, networkerrors.SpaceNotFound) {
			return errors.Capture(err)
		}
		if managementSpace != nil {
			agents, _ = addresses.InSpaces(*managementSpace)
			agents = orderBootstrapAddresses(agents, network.ScopeCloudLocal)
			peers = agents
		}
	case coremodel.CAAS:
		serviceManager, err := serviceManagerGetter(ctx)
		if err != nil {
			return errors.Capture(err)
		}
		ordinal, err := strconv.Atoi(controllerID)
		if err != nil {
			return errors.Errorf("parsing controller ID %q as an ordinal: %w", controllerID, err)
		}
		peerFQDN := serviceManager.ControllerUnitFQDN(ordinal)
		if peerFQDN == "" {
			return errors.New("controller peer FQDN is empty")
		}
		peers = network.SpaceAddresses{
			network.NewSpaceAddress(peerFQDN, network.WithScope(network.ScopeCloudLocal)),
		}
	default:
		return errors.Errorf("unsupported controller model type %q", modelType)
	}

	args := controllernode.SetAPIAddressArgs{
		APIPort: apiPort,
		Addresses: map[string]controllernode.APIAddressSet{
			controllerID: {
				Clients: clients,
				Agents:  agents,
				Peers:   peers,
			},
		},
	}
	if modelType == coremodel.CAAS {
		args.Addresses[controllerID] = controllernode.APIAddressSet{Peers: peers}
		args.SharedAddresses = controllernode.SharedAPIAddressSet{
			Clients: clients,
			Agents:  agents,
		}
	}
	return errors.Capture(controllerNodeService.SetAPIAddresses(ctx, args))
}

func orderBootstrapAddresses(addresses network.SpaceAddresses, preferredScope network.Scope) network.SpaceAddresses {
	result := append(network.SpaceAddresses(nil), addresses...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Scope != b.Scope {
			if a.Scope == preferredScope || b.Scope == preferredScope {
				return a.Scope == preferredScope
			}
			return a.Scope < b.Scope
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return a.SpaceID < b.SpaceID
	})
	return result
}
