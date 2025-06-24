// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/containermanager"
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
	// GetMachineSpaceConstraints retrieves the positive and negative
	// space constraints for the machine with the input UUID.
	GetMachineSpaceConstraints(
		ctx context.Context, machineUUID string,
	) ([]internal.SpaceName, []internal.SpaceName, error)

	// GetMachineAppBindings retrieves the bound spaces for applications
	// with units assigned to the machine with the input UUID.
	GetMachineAppBindings(ctx context.Context, machineUUID string) ([]internal.SpaceName, error)

	// NICsInSpaces retrieves the link-layer devices on the machine with the
	// input net node UUID that are connected the input spaces.
	NICsInSpaces(ctx context.Context, netNode string, spaces []string) (map[string][]network.NetInterface, error)

	// GetContainerNetworkingMethod returns the model's configured value
	// for container-networking-method.
	GetContainerNetworkingMethod(ctx context.Context) (string, error)
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

	if err := hostUUID.Validate(); err != nil {
		return nil, errors.Errorf("invalid host machine UUID: %w", err)
	}
	if err := guestUUID.Validate(); err != nil {
		return nil, errors.Errorf("invalid guest machine UUID: %w", err)
	}

	spaces, err := s.spaceReqsForMachine(ctx, guestUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	spaceUUIDs := make([]string, len(spaces))
	spaceNames := make([]string, len(spaces))
	for i, space := range spaces {
		spaceUUIDs[i] = space.UUID
		spaceNames[i] = space.Name
	}

	s.logger.Infof(ctx, "machine %q needs spaces %v", guestUUID, spaceNames)

	nics, err := s.nicsInSpaces(ctx, hostUUID, spaceUUIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	toBridge, err := s.devicesToBridge(ctx, hostUUID, spaceUUIDs, nics)
	return toBridge, errors.Capture(err)
}

// spacesForMachine returns UUID-to-name for the *positive*
// space requirements of the machine with the input UUID.
// If the positive and negative space constraints are in conflict,
// an error is returned.
func (s *Service) spaceReqsForMachine(ctx context.Context, machineUUID machine.UUID) ([]internal.SpaceName, error) {
	positive, negative, err := s.st.GetMachineSpaceConstraints(ctx, machineUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving positive space constraints for machine %q: %w", machineUUID, err)
	}

	bound, err := s.st.GetMachineAppBindings(ctx, machineUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving app bindings for machine %q: %w", machineUUID, err)
	}

	// Create a unique list of all positive space requirements.
	for _, boundSpace := range bound {
		var dup bool
		for _, posSpace := range positive {
			if boundSpace.UUID == posSpace.UUID {
				dup = true
				break
			}
		}
		if !dup {
			positive = append(positive, boundSpace)
		}
	}

	// Check for conflicts between positive and negative space constraints.
	for _, posSpace := range positive {
		for _, negSpace := range negative {
			if posSpace.UUID == negSpace.UUID {
				return nil, errors.Errorf(
					"%q is both a positive and negative space requirement for machine %q", negSpace.Name, machineUUID,
				).Add(domainerrors.SpaceRequirementConflict)
			}
		}
	}

	return positive, nil
}

// nicsInSpaces returns a map of space UUID to network devices in that space for
// the machine identified by the input UUID and the list of input space UUIDs.
func (s *Service) nicsInSpaces(
	ctx context.Context, mUUID machine.UUID, spaceUUIDs []string,
) (map[string][]network.NetInterface, error) {
	nodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, mUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving net node for machine %q: %w", mUUID, err)
	}

	nics, err := s.st.NICsInSpaces(ctx, nodeUUID, spaceUUIDs)
	if err != nil {
		return nil, errors.Errorf("retrieving NICs for machine %q in spaces %v: %w", mUUID, spaceUUIDs, err)
	}

	return nics, nil
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

nextSpace:
	for spaceUUID, spaceNics := range nics {
		if !spacesLeftToSatisfy.Contains(spaceUUID) {
			continue
		}

		s.logger.Debugf(ctx, "looking for devices in space %q", spaceUUID)

		// Check all bridges first.
		// If any of these satisfy the space requirement, no action is required.
		for _, nic := range spaceNics {
			if nic.Type != corenetwork.BridgeDevice {
				continue
			}

			// The default LXD bridge can only satisfy a space requirement
			// if the container networking method is "local".
			if netMethod != containermanager.NetworkingMethodLocal.String() &&
				nic.Name == internalNetwork.DefaultLXDBridge {
				continue
			}

			spacesLeftToSatisfy.Remove(spaceUUID)
			continue nextSpace
		}

		// Next, check other interfaces to see if bridging
		// one satisfies the space requirement.
		// This is a second loop iteration, but we're looking
		// at a very small n, usually 1.
		for _, nic := range spaceNics {
			if nic.Type == corenetwork.BridgeDevice {
				continue
			}

			if s.isValidBridgeCandidate(ctx, nic, nics) {
				toBridge = append(toBridge, network.DeviceToBridge{
					DeviceName: nic.Name,
					BridgeName: bridgeNameForDevice(nic.Name),
					MACAddress: *nic.MACAddress,
				})

				spacesLeftToSatisfy.Remove(spaceUUID)
				continue nextSpace
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
