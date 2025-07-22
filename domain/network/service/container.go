// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"hash/crc32"
	"slices"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/containermanager"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/network"
	domainerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
	internalNetwork "github.com/juju/juju/internal/network"
)

// ContainerState describes methods for determining and
// satisfying container networking requirements.
type ContainerState interface {
	// GetMachineSpaceConstraints returns the positive and negative
	// space constraints for the machine with the input UUID.
	GetMachineSpaceConstraints(
		ctx context.Context, machineUUID string,
	) ([]internal.SpaceName, []internal.SpaceName, error)

	// GetMachineAppBindings returns the bound spaces for applications
	// with units assigned to the machine with the input UUID.
	GetMachineAppBindings(ctx context.Context, machineUUID string) ([]internal.SpaceName, error)

	// NICsInSpaces returns the link-layer devices on the machine with the
	// input net node UUID, indexed by the spaces that they are in.
	NICsInSpaces(ctx context.Context, nodeUUID string) (map[string][]network.NetInterface, error)

	// GetContainerNetworkingMethod returns the model's configured value
	// for container-networking-method.
	GetContainerNetworkingMethod(ctx context.Context) (string, error)

	// GetSubnetCIDRForDevice uses the device identified by the input node UUID
	// and device name to locate the CIDR of the subnet that it is connected to,
	// in the input space.
	GetSubnetCIDRForDevice(ctx context.Context, nodeUUID, deviceName, spaceUUID string) (string, error)
}

// DevicesToBridge accepts the UUID of a host machine and a guest container/VM.
// It returns the information needed for creating network bridges that will be
// parents of the guest's virtual network devices.
// This determination is made based on the guest's space constraints, bindings
// of applications to run on the guest, and any host bridges that already exist.
func (s *Service) DevicesToBridge(
	ctx context.Context, hostUUID, guestUUID machine.UUID,
) ([]network.DeviceToBridge, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	_, spaceUUIDs, nics, err := s.spacesAndDevicesForMachine(ctx, guestUUID, hostUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	toBridge, err := s.devicesToBridge(ctx, hostUUID, spaceUUIDs, nics)
	return toBridge, errors.Capture(err)
}

// AllocateContainerAddresses allocates a static address for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID, if the
// provider supports it. Returns the network config including all allocated
// addresses on success.
// Returns [domainerrors.ContainerAddressesNotSupported] if the provider
// does not support container addressing.
func (s *ProviderService) AllocateContainerAddresses(ctx context.Context,
	hostInstanceID instance.Id,
	containerName string,
	preparedInfo corenetwork.InterfaceInfos,
) (corenetwork.InterfaceInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerWithNetworking(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	if !provider.SupportsContainerAddresses() {
		return nil, domainerrors.ContainerAddressesNotSupported
	}

	return provider.AllocateContainerAddresses(ctx, hostInstanceID, containerName, preparedInfo)
}

// DevicesForGuest returns the network devices that should be configured in the
// guest machine with the input UUID, based on the host machine's bridges.
func (s *ProviderService) DevicesForGuest(
	ctx context.Context, hostUUID, guestUUID machine.UUID,
) ([]network.NetInterface, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	nodeUUID, spaceUUIDs, nics, err := s.spacesAndDevicesForMachine(ctx, guestUUID, hostUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	guestDevices, err := s.guestDevices(ctx, hostUUID, nodeUUID, spaceUUIDs, nics)
	return guestDevices, errors.Capture(err)
}

func (s *Service) spacesAndDevicesForMachine(
	ctx context.Context, guestUUID, hostUUID machine.UUID,
) (string, []string, map[string][]network.NetInterface, error) {
	if err := hostUUID.Validate(); err != nil {
		return "", nil, nil, errors.Errorf("invalid host machine UUID: %w", err)
	}
	if err := guestUUID.Validate(); err != nil {
		return "", nil, nil, errors.Errorf("invalid guest machine UUID: %w", err)
	}

	spaces, err := s.spaceRequirementsForMachine(ctx, guestUUID)
	if err != nil {
		return "", nil, nil, errors.Capture(err)
	}

	spaceUUIDs := make([]string, len(spaces))
	spaceNames := make([]string, len(spaces))
	for i, space := range spaces {
		spaceUUIDs[i] = space.UUID
		spaceNames[i] = space.Name
	}

	s.logger.Infof(ctx, "machine %q needs spaces %v", guestUUID, spaceNames)

	nodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, hostUUID.String())
	if err != nil {
		return "", nil, nil, errors.Errorf("retrieving net node for machine %q: %w", hostUUID, err)
	}

	nics, err := s.st.NICsInSpaces(ctx, nodeUUID)
	if err != nil {
		return "", nil, nil, errors.Errorf("retrieving NICs for machine %q: %w", hostUUID, err)
	}

	s.logger.Debugf(ctx, "devices by space for machine %q: %#v", guestUUID, nics)
	return nodeUUID, spaceUUIDs, nics, nil
}

// spaceRequirementsForMachine returns UUID-to-name for the *positive*
// space requirements of the machine with the input UUID.
// If the positive and negative space constraints are in conflict,
// an error is returned.
func (s *Service) spaceRequirementsForMachine(
	ctx context.Context, machineUUID machine.UUID,
) ([]internal.SpaceName, error) {
	positive, negative, err := s.st.GetMachineSpaceConstraints(ctx, machineUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving positive space constraints for machine %q: %w", machineUUID, err)
	}

	bound, err := s.st.GetMachineAppBindings(ctx, machineUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving app bindings for machine %q: %w", machineUUID, err)
	}

	posUUIDs := transform.SliceToMap(positive, func(s internal.SpaceName) (string, struct{}) {
		return s.UUID, struct{}{}
	})

	// Create a unique list of all positive space requirements.
	for _, boundSpace := range bound {
		if _, ok := posUUIDs[boundSpace.UUID]; !ok {
			positive = append(positive, boundSpace)
			posUUIDs[boundSpace.UUID] = struct{}{}
		}
	}

	// Check for conflicts between positive and negative space constraints.
	for _, negSpace := range negative {
		if _, ok := posUUIDs[negSpace.UUID]; ok {
			return nil, errors.Errorf(
				"%q is both a positive and negative space requirement for machine %q", negSpace.Name, machineUUID,
			).Add(domainerrors.SpaceRequirementConflict)
		}
	}

	return positive, nil
}

func (s *Service) devicesToBridge(
	ctx context.Context, mUUID machine.UUID, spaceUUIDs []string, nics map[string][]network.NetInterface,
) ([]network.DeviceToBridge, error) {
	netMethod, err := s.st.GetContainerNetworkingMethod(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	spacesLeftToSatisfy := set.NewStrings(spaceUUIDs...)
	var toBridge []network.DeviceToBridge

	for spaceUUID, spaceNics := range nics {
		// We retrieved all the machine's NICs in order to locate parents if
		// required, so only consider those that can satisfy the determined
		// requirements.
		if !spacesLeftToSatisfy.Contains(spaceUUID) {
			continue
		}

		s.logger.Debugf(ctx, "looking for devices in space %q", spaceUUID)

		// Check all bridges first.
		// If any of these satisfy the space requirement, no action is required.
		// The default LXD bridge can only satisfy a space requirement if the
		// container networking method is "local".
		// For practical purposes, OVS devices are treated as bridges.
		if slices.ContainsFunc(spaceNics, func(nic network.NetInterface) bool {
			if nic.Type != corenetwork.BridgeDevice && nic.VirtualPortType != corenetwork.OvsPort {
				return false
			}
			return netMethod == containermanager.NetworkingMethodLocal.String() ||
				nic.Name != internalNetwork.DefaultLXDBridge
		}) {
			spacesLeftToSatisfy.Remove(spaceUUID)
			continue
		}

		// Next, check other interfaces to see if bridging
		// one satisfies the space requirement.
		// This is a second loop iteration, but we're looking
		// at a very small n, usually 1.
		for _, nic := range spaceNics {
			if nic.Type == corenetwork.BridgeDevice || nic.VirtualPortType == corenetwork.OvsPort {
				continue
			}

			if s.isValidBridgeCandidate(ctx, nic, nics) {
				toBridge = append(toBridge, network.DeviceToBridge{
					DeviceName: nic.Name,
					BridgeName: bridgeNameForDevice(nic.Name),
					MACAddress: *nic.MACAddress,
				})

				spacesLeftToSatisfy.Remove(spaceUUID)
				break
			}
		}
	}

	if spacesLeftToSatisfy.Size() != 0 {
		return nil, errors.Errorf(
			"host %q has no available device in space(s) %v", mUUID, spacesLeftToSatisfy.SortedValues(),
		).Add(domainerrors.SpaceRequirementsUnsatisfiable)
	}

	return toBridge, nil
}

func (s *Service) isValidBridgeCandidate(
	ctx context.Context, nic network.NetInterface, nics map[string][]network.NetInterface,
) bool {
	// LoopbackDevices can never be bridged.
	if nic.Type == corenetwork.LoopbackDevice {
		return false
	}

	// Devices that have no parent entry are direct
	// host devices that can be bridged.
	if nic.ParentDeviceName == "" {
		return true
	}

	// If we get to here, only a VLAN device can have
	// a parent that will allow us to bridge it.
	if nic.Type != corenetwork.VLAN8021QDevice {
		return false
	}

	parentDevice := findParent(nic.ParentDeviceName, nics)
	if parentDevice == nil {
		// Referential integrity should make this impossible, but we'll note it.
		s.logger.Warningf(ctx, "no parent device %q found for %q", nic.ParentDeviceName, nic.Name)
		return false
	}

	if parentDevice.Type == corenetwork.EthernetDevice || parentDevice.Type == corenetwork.BondDevice {
		// The VLAN is connected to a device that we can bridge.
		return true
	}

	return false
}

func findParent(parentName string, nics map[string][]network.NetInterface) *network.NetInterface {
	for _, spaceNics := range nics {
		for _, nic := range spaceNics {
			if nic.ParentDeviceName == parentName {
				return &nic
			}
		}
	}
	return nil
}

// bridgeNameForDevice returns a name to use for a new
// device that bridges the device with the input name.
//
// The policy in order of preference is:
// - Add "br-" to device name (to keep current behaviour).
// - If it does not fit in 15 characters then add "b-" to device name.
// - If it still doesn't fit in 15 characters then:
//   - For devices starting in "en" remove "en" and add "b-".
//   - For all other devices use "b-" + 6-char hash of name + "-"
//   - last 6 chars of name.
//   - If using the device name directly, always replace "." with "-"
//     to make sure that bridges from VLANs won't break.
func bridgeNameForDevice(device string) string {
	device = strings.Replace(device, ".", "-", -1)
	switch {
	case len(device) < 13:
		return fmt.Sprintf("br-%s", device)
	case len(device) == 13:
		return fmt.Sprintf("b-%s", device)
	case device[:2] == "en":
		return fmt.Sprintf("b-%s", device[2:])
	default:
		hash := crc32.Checksum([]byte(device), crc32.IEEETable) & 0xffffff
		return fmt.Sprintf("b-%0.6x-%s", hash, device[len(device)-6:])
	}
}

func (s *ProviderService) guestDevices(
	ctx context.Context, mUUID machine.UUID, nodeUUID string, spaceUUIDs []string, nics map[string][]network.NetInterface,
) ([]network.NetInterface, error) {
	var (
		guestDevices []network.NetInterface
		deviceIndex  int
	)
	spacesToSatisfy := set.NewStrings(spaceUUIDs...)

	networkingProvider, err := s.providerWithNetworking(ctx)
	if err != nil {
		return nil, errors.Errorf("retrieving networking provider: %w", err)
	}

	// In most cases, the container will rely on DHCP assigned addresses.
	// If the provider supports allocating addresses to containers,
	// each device's address will be obtained downstream, and we indicate
	// that said address is configured statically.
	configMethod := corenetwork.ConfigDHCP
	if networkingProvider.SupportsContainerAddresses() {
		configMethod = corenetwork.ConfigStatic
	}

	for spaceUUID, spaceNics := range nics {
		if !spacesToSatisfy.Contains(spaceUUID) {
			continue
		}

		s.logger.Debugf(ctx, "looking for bridges in space %q", spaceUUID)

		var bridgeToUse *network.NetInterface
		for _, nic := range spaceNics {
			if nic.Type == corenetwork.BridgeDevice || nic.VirtualPortType == corenetwork.OvsPort {
				bridgeToUse = &nic
				break
			}
		}

		if bridgeToUse == nil {
			return nil, errors.Errorf(
				"no bridge found in space %q for machine %q", spaceUUID, mUUID,
			).Add(domainerrors.SpaceRequirementsUnsatisfiable)
		}

		s.logger.Debugf(ctx, "found bridge %q in space %q for machine %q", bridgeToUse.Name, spaceUUID, mUUID)

		newDev := network.NetInterface{
			Name: fmt.Sprintf("eth%d", deviceIndex),
			// When using the Fan, we used to locate the VXLAN device
			// associated with the bridge and use that MTU.
			// We no longer support Fan networking, but this is worth being
			// aware of in situations where the MTU set turns out to be
			// incompatible with the bridged network.
			MTU:              bridgeToUse.MTU,
			Type:             corenetwork.EthernetDevice,
			ParentDeviceName: bridgeToUse.Name,
			VirtualPortType:  bridgeToUse.VirtualPortType,
			IsEnabled:        true,
			IsAutoStart:      true,
		}

		mac := corenetwork.GenerateVirtualMACAddress()
		newDev.MACAddress = &mac

		cidr, err := s.st.GetSubnetCIDRForDevice(ctx, nodeUUID, bridgeToUse.Name, spaceUUID)
		if err != nil {
			return nil, errors.Errorf(
				"retrieving CIDR for device %q in space %q on machine %q: %w", bridgeToUse.Name, spaceUUID, mUUID, err)
		}

		newDev.Addrs = []network.NetAddr{{
			AddressValue: cidr,
			ConfigType:   configMethod,
		}}

		deviceIndex++
		guestDevices = append(guestDevices, newDev)
	}

	return guestDevices, nil
}
