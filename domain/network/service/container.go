// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"cmp"
	"context"
	"fmt"
	"hash/crc32"
	"slices"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/containermanager"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/modelconfig"
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

	// GetMachineInstanceID returns the cloud instance ID of the machine with
	// the input UUID. If the machine has no cloud instance, or the instance
	// has not yet been created, an error satisfying
	// [domainerrors.HostNotProvisioned] is returned.
	GetMachineInstanceID(ctx context.Context, machineUUID string) (string, error)

	// GetMachineAppBindings returns the bound spaces for applications
	// with units assigned to the machine with the input UUID.
	GetMachineAppBindings(ctx context.Context, machineUUID string) ([]internal.SpaceName, error)

	// NICsInSpaces returns the link-layer devices on the machine with the
	// input net node UUID, indexed by the UUIDs of the spaces that they
	// are in. Devices that are not associated with any space, e.g. because
	// their subnet is not registered with Juju, are indexed under the
	// empty-string key. This convention is relied upon to locate the
	// default LXD bridge when using local container networking.
	NICsInSpaces(ctx context.Context, nodeUUID string) (map[string][]network.NetInterface, error)

	// GetContainerNetworkingMethod returns the model's raw configured
	// value for container-networking-method. The value is not resolved:
	// an empty value indicates "auto", which is resolved against the
	// model's provider by the service.
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
// When the container networking method resolves to "local" (explicitly
// configured, or "auto" on a provider without container address support),
// the default LXD bridge satisfies all space requirements, wherever its
// addresses are reported, and no host devices are selected for bridging.
// Note that negative space constraints are not enforced in this mode: the
// default LXD bridge is used regardless of whether the space (if any) in
// which its addresses are reported is negatively constrained.
func (s *ProviderService) DevicesToBridge(
	ctx context.Context, hostUUID, guestUUID machine.UUID,
) ([]network.DeviceToBridge, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	_, spaceUUIDs, nics, err := s.spacesAndDevicesForMachine(ctx, guestUUID, hostUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.devicesToBridge(ctx, hostUUID, spaceUUIDs, nics)
}

// NetworkConfigForGuest returns the network configuration to apply to the
// guest machine with the input UUID and name, hosted by the host machine with
// the input UUID. It is based on the host machine's bridges.
// When the container networking method resolves to "local" (explicitly
// configured, or "auto" on a provider without container address support),
// the default LXD bridge is used for spaces that have no in-space bridge.
// In this mode all devices are configured for DHCP, as the container's
// networking is provided by the host machine, and at most one device is
// created with the default LXD bridge as its parent.
// When the effective method is provider networking and the provider supports
// container address allocation, the provider is asked to allocate an address
// for each device, and the returned configuration includes them.
// The provider is never consulted for addresses in any other case.
// Note that negative space constraints are not enforced in local mode: the
// default LXD bridge is used regardless of whether the space (if any) in
// which its addresses are reported is negatively constrained.
// If provider address allocation is required, but the host machine has not
// been provisioned, an error satisfying
// [domainerrors.HostNotProvisioned] is returned.
func (s *ProviderService) NetworkConfigForGuest(
	ctx context.Context, hostUUID, guestUUID machine.UUID, guestName machine.Name,
) (corenetwork.InterfaceInfos, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	nodeUUID, spaceUUIDs, nics, err := s.spacesAndDevicesForMachine(ctx, guestUUID, hostUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	isLocal, supportsAddresses, err := s.containerNetworking(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Addresses are allocated by the provider only when it is the provider
	// that is responsible for container networking, and it is capable.
	allocateAddresses := !isLocal && supportsAddresses

	configMethod := corenetwork.ConfigDHCP
	if allocateAddresses {
		configMethod = corenetwork.ConfigStatic
	}

	devices, err := s.guestDevices(ctx, hostUUID, nodeUUID, spaceUUIDs, nics, isLocal, configMethod)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// TODO (manadart 2025-07-23): I so, so do not want to use
	//  InterfaceInfos anymore, but changing it would flow deep into the
	//  MAAS provider, which is not going to be undertaken under the Dqlite
	//  rewrite. Ideally we would use NetInterface from the network domain
	//  everywhere. Note that NetInterface and NetAddr do not yet carry
	//  everything that providers return (routes, provider VLAN IDs and
	//  device indices), which would first need to be added.
	preparedInfo := toInterfaceInfos(devices)

	if !allocateAddresses || len(preparedInfo) == 0 {
		return preparedInfo, nil
	}

	hostInstanceID, err := s.st.GetMachineInstanceID(ctx, hostUUID.String())
	if err != nil {
		return nil, errors.Errorf("retrieving instance ID for host machine %q: %w", hostUUID, err)
	}

	info, err := s.allocateContainerAddresses(ctx, instance.Id(hostInstanceID), guestName.String(), preparedInfo)
	if err != nil {
		return nil, errors.Errorf("allocating addresses for guest machine %q: %w", guestName, err)
	}
	return info, nil
}

// allocateContainerAddresses asks the provider to allocate a static address
// for each of the container NICs in preparedInfo, hosted by the
// hostInstanceID. It returns the network config including all allocated
// addresses on success.
func (s *ProviderService) allocateContainerAddresses(
	ctx context.Context,
	hostInstanceID instance.Id,
	containerName string,
	preparedInfo corenetwork.InterfaceInfos,
) (corenetwork.InterfaceInfos, error) {
	provider, err := s.providerWithNetworking(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	newInfo, err := provider.AllocateContainerAddresses(ctx, hostInstanceID, containerName, preparedInfo)
	return newInfo, errors.Capture(err)
}

// toInterfaceInfos transforms network domain interfaces to the type used by
// providers and the wire.
func toInterfaceInfos(netInterfaces []network.NetInterface) corenetwork.InterfaceInfos {
	res := make(corenetwork.InterfaceInfos, len(netInterfaces))
	for i, netInterface := range netInterfaces {
		var mtu int
		if netInterface.MTU != nil {
			mtu = int(*netInterface.MTU)
		}
		var mac string
		if netInterface.MACAddress != nil {
			mac = *netInterface.MACAddress
		}

		var (
			addrs         corenetwork.ProviderAddresses
			nicConfigType corenetwork.AddressConfigType
		)

		// There is a single address populated for each interface.
		// The *device* config type is populated from the address.
		// Note that we populate the *CIDR* from the address value.
		if len(netInterface.Addrs) > 0 {
			a := netInterface.Addrs[0]
			addrs = corenetwork.ProviderAddresses{{MachineAddress: corenetwork.MachineAddress{
				ConfigType: a.ConfigType,
				CIDR:       a.AddressValue,
			}}}
			nicConfigType = a.ConfigType
		}

		res[i] = corenetwork.InterfaceInfo{
			MACAddress:          mac,
			ConfigType:          nicConfigType,
			VLANTag:             int(netInterface.VLANTag),
			InterfaceName:       netInterface.Name,
			ParentInterfaceName: netInterface.ParentDeviceName,
			InterfaceType:       netInterface.Type,
			Disabled:            !netInterface.IsEnabled,
			NoAutoStart:         !netInterface.IsAutoStart,
			Addresses:           addrs,
			DNSServers:          netInterface.DNSAddresses,
			MTU:                 mtu,
		}
	}
	return res
}

// spacesAndDevicesForMachine returns the net node UUID of the host machine,
// the guest's positive space requirement UUIDs in sorted name order, and the
// host's devices indexed by space. The ordering of the returned space UUIDs
// is relied upon by guestDevices for deterministic device naming and
// default-bridge deduplication.
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

	hostNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, hostUUID.String())
	if err != nil {
		return "", nil, nil, errors.Errorf("retrieving net node for machine %q: %w", hostUUID, err)
	}

	nics, err := s.st.NICsInSpaces(ctx, hostNodeUUID)
	if err != nil {
		return "", nil, nil, errors.Errorf("retrieving NICs for machine %q: %w", hostUUID, err)
	}

	s.logger.Debugf(ctx, "devices by space for host machine %q: %#v", hostUUID, nics)
	return hostNodeUUID, spaceUUIDs, nics, nil
}

// spaceRequirementsForMachine returns UUID-to-name for the *positive*
// space requirements of the machine with the input UUID, sorted by name.
// Callers rely on this order for deterministic guest device naming and
// default-bridge deduplication.
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

	// Sort the spaces by name, so that requirements are satisfied in a
	// deterministic order and guest device naming is reproducible.
	slices.SortFunc(positive, func(a, b internal.SpaceName) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return positive, nil
}

func (s *ProviderService) devicesToBridge(
	ctx context.Context, mUUID machine.UUID,
	spaceUUIDs []string, nics map[string][]network.NetInterface,
) ([]network.DeviceToBridge, error) {
	isLocal, err := s.isLocalContainerNetworking(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// When using local container networking, the default LXD bridge
	// satisfies all space requirements, regardless of the space (if any)
	// in which its addresses are reported. No host devices are bridged.
	if isLocal && defaultLXDBridge(nics) != nil {
		return nil, nil
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
		// The default LXD bridge is never selected here: with local
		// networking all requirements are already satisfied by the check
		// above, and with provider networking it is never a valid parent.
		// For practical purposes, OVS devices are treated as bridges.
		if slices.ContainsFunc(spaceNics, func(nic network.NetInterface) bool {
			if nic.Type != corenetwork.BridgeDevice && nic.VirtualPortType != corenetwork.OvsPort {
				return false
			}
			return nic.Name != internalNetwork.DefaultLXDBridge
		}) {
			spacesLeftToSatisfy.Remove(spaceUUID)
			continue
		}

		// Host devices are never bridged when using local networking.
		// Spaces that have no bridge remain unsatisfied and are reported
		// below, so that the caller can retry.
		if isLocal {
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

// isLocalContainerNetworking reports whether the effective container
// networking method for the model is local. "local" and "provider" are
// used as configured, while the unset "auto" value is resolved against
// the model's provider: local unless it supports allocating container
// addresses. Explicitly configured values never consult the provider.
// See containerNetworking for the variant that also reports the provider's
// capability.
func (s *ProviderService) isLocalContainerNetworking(ctx context.Context) (bool, error) {
	netMethod, err := s.st.GetContainerNetworkingMethod(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	method, err := containermanager.ResolveNetworkingMethodWithCapability(
		modelconfig.ContainerNetworkingMethod(netMethod),
		func() (bool, error) { return s.supportsContainerAddresses(ctx) },
	)
	if err != nil {
		return false, errors.Capture(err)
	}
	return method == containermanager.NetworkingMethodLocal, nil
}

// containerNetworking resolves the effective container networking method for
// the model, reporting whether it is local, and whether the model's provider
// supports allocating container addresses.
// The provider capability is consulted at most once, and only when it is
// needed: the unset "auto" value resolves against it, and the "provider"
// value needs it to decide whether addresses are allocated by the provider.
// An explicitly configured "local" value does not depend on the provider, so
// it is never consulted in that case. See isLocalContainerNetworking for the
// lazy variant used when only the local/provider decision is required.
func (s *ProviderService) containerNetworking(ctx context.Context) (bool, bool, error) {
	netMethod, err := s.st.GetContainerNetworkingMethod(ctx)
	if err != nil {
		return false, false, errors.Capture(err)
	}

	method := modelconfig.ContainerNetworkingMethod(netMethod)
	var supportsAddresses bool
	if method != modelconfig.ContainerNetworkingMethodLocal {
		supportsAddresses, err = s.supportsContainerAddresses(ctx)
		if err != nil {
			return false, false, errors.Capture(err)
		}
	}

	resolved, err := containermanager.ResolveNetworkingMethod(method, supportsAddresses)
	if err != nil {
		return false, false, errors.Capture(err)
	}
	return resolved == containermanager.NetworkingMethodLocal, supportsAddresses, nil
}

// supportsContainerAddresses reports whether the model's provider
// supports allocating container addresses. A provider without the
// networking capability cannot allocate them either, so false is
// reported for it without error.
func (s *ProviderService) supportsContainerAddresses(ctx context.Context) (bool, error) {
	provider, err := s.providerWithNetworking(ctx)
	if err != nil && !errors.Is(err, coreerrors.NotSupported) {
		return false, errors.Errorf("retrieving networking provider: %w", err)
	}
	return provider != nil && provider.SupportsContainerAddresses(), nil
}

// defaultLXDBridge returns the default LXD bridge observed on the host,
// in whatever space (if any) its addresses are reported.
// This accommodates the common case where the subnet of the default LXD
// bridge is not registered with Juju, so the device is not associated with
// any space.
// A nil result indicates that the bridge has not been observed on the host.
func defaultLXDBridge(nics map[string][]network.NetInterface) *network.NetInterface {
	for _, spaceNics := range nics {
		if idx := slices.IndexFunc(spaceNics, func(nic network.NetInterface) bool {
			return nic.Name == internalNetwork.DefaultLXDBridge
		}); idx >= 0 {
			return &spaceNics[idx]
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
	device = strings.ReplaceAll(device, ".", "-")
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

// guestDevices returns the devices to configure in a guest, one for each of
// the requested spaces, parented to a bridge on the host.
// Devices are configured for addressing in the input manner, except for
// devices parented to the default LXD bridge via the local-networking
// fallback, which always use DHCP.
func (s *ProviderService) guestDevices(
	ctx context.Context,
	mUUID machine.UUID,
	nodeUUID string,
	spaceUUIDs []string,
	nics map[string][]network.NetInterface,
	isLocal bool,
	configMethod corenetwork.AddressConfigType,
) ([]network.NetInterface, error) {
	var (
		guestDevices []network.NetInterface
		deviceIndex  int
	)

	// lxdBridgeUsed indicates that a guest device has already been created
	// with the default LXD bridge as its parent.
	lxdBridgeUsed := false
	lxdBridge := defaultLXDBridge(nics)

	// Iterate the required spaces, so that requirements are satisfied in a
	// deterministic order, and so that spaces without observed devices can
	// still be satisfied by the default LXD bridge when using local
	// networking.
	for _, spaceUUID := range spaceUUIDs {
		spaceNics := nics[spaceUUID]
		if len(spaceNics) == 0 && !isLocal {
			// Without local networking, spaces for which the host has no
			// observed devices are not considered. With local networking,
			// such spaces fall through to the default LXD bridge fallback
			// below.
			continue
		}

		s.logger.Debugf(ctx, "looking for bridges in space %q", spaceUUID)

		var bridgeToUse *network.NetInterface
		fromLocalBridge := false
		for _, nic := range spaceNics {
			if nic.Type == corenetwork.BridgeDevice || nic.VirtualPortType == corenetwork.OvsPort {
				bridgeToUse = &nic
				break
			}
		}

		if bridgeToUse == nil && isLocal {
			if lxdBridge == nil {
				return nil, errors.Errorf(
					"no bridge found in space %q for machine %q; the default LXD bridge %q has not been observed",
					spaceUUID, mUUID, internalNetwork.DefaultLXDBridge,
				).Add(domainerrors.SpaceRequirementsUnsatisfiable)
			}
			bridgeToUse = lxdBridge
			fromLocalBridge = true
		}

		if bridgeToUse == nil {
			return nil, errors.Errorf(
				"no bridge found in space %q for machine %q", spaceUUID, mUUID,
			).Add(domainerrors.SpaceRequirementsUnsatisfiable)
		}

		// With local networking, a single device parented to the default
		// LXD bridge suffices for all spaces that would select it, whether
		// the bridge is observed in those spaces or used as the fallback.
		if isLocal && bridgeToUse.Name == internalNetwork.DefaultLXDBridge {
			if lxdBridgeUsed {
				// The device parented to the default LXD bridge satisfies
				// the requirements of this space too.
				s.logger.Debugf(ctx, "space %q for machine %q satisfied by the existing device on default LXD bridge %q",
					spaceUUID, mUUID, bridgeToUse.Name)
				continue
			}
			lxdBridgeUsed = true
		}

		if fromLocalBridge {
			// The fallback was used, so no bridge was observed in the
			// space itself: "found" would be misleading here.
			s.logger.Debugf(ctx, "using default LXD bridge %q for space %q for machine %q; no in-space bridge observed",
				bridgeToUse.Name, spaceUUID, mUUID)
		} else {
			s.logger.Debugf(ctx, "found bridge %q in space %q for machine %q",
				bridgeToUse.Name, spaceUUID, mUUID)
		}

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

		if fromLocalBridge {
			// The device is parented to the default LXD bridge selected
			// via the local-networking fallback, so no CIDR is looked up
			// for it. Addresses are obtained via DHCP served on the bridge
			// itself.
			newDev.Addrs = []network.NetAddr{{
				ConfigType: corenetwork.ConfigDHCP,
			}}
		} else {
			cidr, err := s.st.GetSubnetCIDRForDevice(ctx, nodeUUID, bridgeToUse.Name, spaceUUID)
			if err != nil {
				return nil, errors.Errorf(
					"retrieving CIDR for device %q in space %q on machine %q: %w", bridgeToUse.Name, spaceUUID, mUUID, err)
			}

			newDev.Addrs = []network.NetAddr{{
				AddressValue: cidr,
				ConfigType:   configMethod,
			}}
		}

		deviceIndex++
		guestDevices = append(guestDevices, newDev)
	}

	return guestDevices, nil
}
