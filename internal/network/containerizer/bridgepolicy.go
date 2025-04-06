// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"context"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/containermanager"
	corenetwork "github.com/juju/juju/core/network"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/network"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.network.containerizer")

var skippedDeviceNames = set.NewStrings(
	network.DefaultLXDBridge,
)

// namedNICsBySpace is a type alias for a map of link-layer devices
// keyed by name, keyed in turn by the space they are in.
type namedNICsBySpace = map[string]map[string]LinkLayerDevice

// BridgePolicy defines functionality that helps us create and define bridges
// for guests inside a host machine, along with the creation of network
// devices on those bridges for the containers to use.
type BridgePolicy struct {
	// allSpaces is the list of all available spaces.
	allSpaces corenetwork.SpaceInfos

	// allSubnets is the list of all available subnets.
	allSubnets corenetwork.SubnetInfos

	// netBondReconfigureDelay is how much of a delay to inject if we see that
	// one of the devices being bridged is a BondDevice. This exists because of
	// https://bugs.launchpad.net/juju/+bug/1657579
	netBondReconfigureDelay int

	// containerNetworkingMethod defines the way containers are networked.
	// It's one of:
	//  - provider
	//  - local
	containerNetworkingMethod containermanager.NetworkingMethod

	// networkService provides the network domain functionality.
	networkService NetworkService
}

// NewBridgePolicy returns a new BridgePolicy for the input environ config
// getter and state indirection.
func NewBridgePolicy(ctx context.Context,
	networkService NetworkService,
	containerNetworkingMethod containermanager.NetworkingMethod,
) (*BridgePolicy, error) {

	allSpaces, err := networkService.GetAllSpaces(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting space infos")
	}
	allSubnets, err := networkService.GetAllSubnets(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "getting subnet infos")
	}

	return &BridgePolicy{
		allSpaces:                 allSpaces,
		allSubnets:                allSubnets,
		containerNetworkingMethod: containerNetworkingMethod,
		networkService:            networkService,
	}, nil
}

// FindMissingBridgesForContainer looks at the spaces that the container should
// have access to, and returns any host devices need to be bridged for use as
// the container network.
// This will return an Error if the container requires a space that the host
// machine cannot provide.
func (p *BridgePolicy) FindMissingBridgesForContainer(
	host Machine, guest Container, allSubnets corenetwork.SubnetInfos,
) ([]network.DeviceToBridge, error) {
	guestSpaceInfos, devicesPerSpace, err := p.findSpacesAndDevicesForContainer(host, guest)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "FindMissingBridgesForContainer(%q) spaces %s devices %v",
		guest.Id(), guestSpaceInfos, formatDeviceMap(devicesPerSpace))

	spacesFound := make(corenetwork.SpaceInfos, 0)
	for spaceID, devices := range devicesPerSpace {
		for _, device := range devices {
			if device.Type() == corenetwork.BridgeDevice {
				if p.containerNetworkingMethod != containermanager.NetworkingMethodLocal && skippedDeviceNames.Contains(device.Name()) {
					continue
				}
				addInfo := p.allSpaces.GetByID(spaceID)
				spacesFound = append(spacesFound, *addInfo)
			}
		}
	}

	notFound := guestSpaceInfos.Minus(spacesFound)

	if len(notFound) == 0 {
		// Nothing to do; just return success.
		return nil, nil
	}

	hostDeviceNamesToBridge := make([]string, 0)
	hostDeviceByName := make(map[string]LinkLayerDevice, 0)
	for _, spaceInfo := range notFound {
		hostDeviceNames := make([]string, 0)
		for _, hostDevice := range devicesPerSpace[spaceInfo.ID] {
			possible, err := possibleBridgeTarget(hostDevice)
			if err != nil {
				return nil, err
			}
			if !possible {
				continue
			}
			hostDeviceNames = append(hostDeviceNames, hostDevice.Name())
			hostDeviceByName[hostDevice.Name()] = hostDevice
			spacesFound = append(spacesFound, spaceInfo)
		}
		if len(hostDeviceNames) > 0 {
			if spaceInfo.ID == corenetwork.AlphaSpaceId {
				// When we are bridging unknown space devices, we bridge all
				// of them. Both because this is a fallback, and because we
				// don't know what the exact spaces are going to be.
				for _, deviceName := range hostDeviceNames {
					hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, deviceName)
				}
			} else {
				// This should already be sorted from
				// LinkLayerDevicesForSpaces but sorting to be sure we stably
				// pick the host device
				hostDeviceNames = network.NaturallySortDeviceNames(hostDeviceNames...)
				hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, hostDeviceNames[0])
			}
		}
	}
	notFound = notFound.Minus(spacesFound)
	if len(notFound) != 0 {
		hostSpaces, err := host.AllSpaces(allSubnets)
		if err != nil {
			// log it, but we're returning another error right now
			logger.Warningf(context.TODO(), "got error looking for spaces for host machine %q: %v", host.Id(), err)
		}
		notFoundNames := notFound.String()
		logger.Warningf(context.TODO(), "container %q wants spaces %s, but host machine %q has %s missing %s",
			guest.Id(), guestSpaceInfos,
			host.Id(), p.spaceNamesForPrinting(hostSpaces), notFoundNames)
		return nil, errors.Errorf("host machine %q has no available device in space(s) %s", host.Id(), notFoundNames)
	}

	hostToBridge := make([]network.DeviceToBridge, 0, len(hostDeviceNamesToBridge))
	for _, hostName := range network.NaturallySortDeviceNames(hostDeviceNamesToBridge...) {
		hostToBridge = append(hostToBridge, network.DeviceToBridge{
			DeviceName: hostName,
			BridgeName: BridgeNameForDevice(hostName),
			MACAddress: hostDeviceByName[hostName].MACAddress(),
		})
	}
	return hostToBridge, nil
}

// findSpacesAndDevicesForContainer looks up what spaces the container wants
// to be in, and what spaces the host machine is already in, and tries to
// find the devices on the host that are useful for the container.
func (p *BridgePolicy) findSpacesAndDevicesForContainer(
	host Machine, guest Container,
) (corenetwork.SpaceInfos, map[string][]LinkLayerDevice, error) {
	containerSpaces, err := p.determineContainerSpaces(host, guest)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	devicesPerSpace, err := p.linkLayerDevicesForSpaces(host, containerSpaces)
	if err != nil {
		logger.Errorf(context.TODO(), "findSpacesAndDevicesForContainer(%q) got error looking for host spaces: %v",
			guest.Id(), err)
		return nil, nil, errors.Trace(err)
	}

	// OVS bridges expose one of the internal ports as a device with the
	// same name as the bridge. These special interfaces are not detected
	// as bridge devices but rather appear as regular NICs. If the configured
	// networking method is "provider", we need to patch the type of these
	// devices so they appear as bridges to allow the bridge policy logic
	// to make use of them.
	if p.containerNetworkingMethod == containermanager.NetworkingMethodProvider {
		for spaceID, devsInSpace := range devicesPerSpace {
			for devIdx, dev := range devsInSpace {
				if dev.VirtualPortType() != corenetwork.OvsPort {
					continue
				}

				devicesPerSpace[spaceID][devIdx] = ovsBridgeDevice{
					wrappedDev: dev,
				}
			}
		}
	}

	return containerSpaces, devicesPerSpace, nil
}

// linkLayerDevicesForSpaces takes a list of SpaceInfos, and returns
// the devices on this machine that are in those spaces that we feel
// would be useful for containers to know about.  (eg, if there is a
// host device that has been bridged, we return the bridge, rather
// than the underlying device, but if we have only the host device,
// we return that.)
// Note that devices like 'lxdbr0' that are bridges that might not be
// externally accessible may be returned if the default space is
// listed as one of the desired spaces.
func (p *BridgePolicy) linkLayerDevicesForSpaces(host Machine, spaces corenetwork.SpaceInfos) (map[string][]LinkLayerDevice, error) {
	deviceByName, err := linkLayerDevicesByName(host)
	if err != nil {
		return nil, errors.Trace(err)
	}

	addresses, err := host.AllDeviceAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Iterate all addresses and key them by the address device name.
	addressByDeviceName := make(map[string]Address)
	for _, addr := range addresses {
		addressByDeviceName[addr.DeviceName()] = addr
	}

	// Iterate the devices by name, lookup the associated spaces, and
	// gather the devices.
	spaceToDevices := make(namedNICsBySpace, 0)
	for _, device := range deviceByName {
		addr, ok := addressByDeviceName[device.Name()]
		if !ok {
			logger.Infof(context.TODO(), "device %q has no addresses, ignoring", device.Name())
			continue
		}

		// Loopback devices are not considered part of the empty space.
		if device.Type() == corenetwork.LoopbackDevice {
			continue
		}

		spaceID := corenetwork.AlphaSpaceId

		subnets, err := p.allSubnets.GetByCIDR(addr.SubnetCIDR())
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(subnets) > 0 {
			// Take the first subnet.
			subnet := subnets[0]
			if err != nil {
				return nil, errors.Trace(err)
			} else {
				spaceID = subnet.SpaceID
			}
		}
		spaceToDevices = includeDevice(spaceToDevices, spaceID, device)
	}

	result := make(map[string][]LinkLayerDevice, len(spaceToDevices))
	for spaceID, deviceMap := range spaceToDevices {
		if !spaces.ContainsID(spaceID) {
			continue
		}
		result[spaceID] = deviceMapToSortedList(deviceMap)
	}
	return result, nil
}

func linkLayerDevicesByName(host Machine) (map[string]LinkLayerDevice, error) {
	devices, err := host.AllLinkLayerDevices()
	if err != nil {
		return nil, errors.Trace(err)
	}
	deviceByName := make(map[string]LinkLayerDevice, len(devices))
	for _, dev := range devices {
		deviceByName[dev.Name()] = dev
	}
	return deviceByName, nil
}

func includeDevice(spaceToDevices namedNICsBySpace, spaceID string, device LinkLayerDevice) namedNICsBySpace {
	spaceInfo, ok := spaceToDevices[spaceID]
	if !ok {
		spaceInfo = make(map[string]LinkLayerDevice)
		spaceToDevices[spaceID] = spaceInfo
	}
	spaceInfo[device.Name()] = device
	return spaceToDevices
}

// deviceMapToSortedList takes a map from device name to LinkLayerDevice
// object, and returns the list of LinkLayerDevice object using
// NaturallySortDeviceNames
func deviceMapToSortedList(deviceMap map[string]LinkLayerDevice) []LinkLayerDevice {
	names := make([]string, 0, len(deviceMap))
	for name := range deviceMap {
		// name must == device.Name()
		names = append(names, name)
	}
	sortedNames := network.NaturallySortDeviceNames(names...)
	result := make([]LinkLayerDevice, len(sortedNames))
	for i, name := range sortedNames {
		result[i] = deviceMap[name]
	}
	return result
}

// determineContainerSpaces tries to use the direct information about a
// container to find what spaces it should be in, and then falls back to what
// we know about the host machine.
func (p *BridgePolicy) determineContainerSpaces(
	host Machine, guest Container,
) (corenetwork.SpaceInfos, error) {
	// Gather any *positive* space constraints for the guest.
	cons, err := guest.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}

	spaces := make(corenetwork.SpaceInfos, 0)
	// Constraints have been left in space name form,
	// as they are human-readable and can be changed.
	for _, spaceName := range cons.IncludeSpaces() {
		if space := p.allSpaces.GetByName(spaceName); space != nil {
			spaces = append(spaces, *space)
		}
	}

	logger.Debugf(context.TODO(), "for container %q, found desired spaces: %s", guest.Id(), spaces)

	if len(spaces) == 0 {
		// We have determined that the container doesn't have any useful
		// constraints set on it. So lets see if we can come up with
		// something useful.
		spaces, err = p.inferContainerSpaces(host, guest.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return spaces, nil
}

// spaceNamesForPrinting return a sorted, comma delimited, string containing
// the space names for each of the given space ids.
func (p *BridgePolicy) spaceNamesForPrinting(ids set.Strings) string {
	if ids.Size() == 0 {
		return "<none>"
	}
	names := set.NewStrings()
	for _, id := range ids.Values() {
		if info := p.allSpaces.GetByID(id); info != nil {
			names.Add(fmt.Sprintf("%q", info.Name))
		} else {
			// fallback, in case we do not have a name for the given
			// id.
			names.Add(fmt.Sprintf("%q", id))
		}
	}
	return strings.Join(names.SortedValues(), ", ")
}

// inferContainerSpaces tries to find a valid space for the container to be in.
// This should only be used when the container itself doesn't have any valid
// constraints on what spaces it should be in.
// If containerNetworkingMethod is 'local' we fall back to the default space
// and use lxdbr0.
// If this machine is in a single space, then that space is used.
// If the machine is in multiple spaces, we return an error with the possible
// spaces that the user can use to constrain connectivity.
func (p *BridgePolicy) inferContainerSpaces(host Machine, containerId string) (corenetwork.SpaceInfos, error) {
	if p.containerNetworkingMethod == containermanager.NetworkingMethodLocal {
		alphaInfo := p.allSpaces.GetByID(corenetwork.AlphaSpaceId)
		return corenetwork.SpaceInfos{*alphaInfo}, nil
	}

	hostSpaces, err := host.AllSpaces(p.allSubnets)
	if err != nil {
		return nil, errors.Trace(err)
	}
	namesHostSpaces := p.spaceNamesForPrinting(hostSpaces)
	logger.Debugf(context.TODO(), "container %q not qualified to a space, host machine %q is using spaces %s",
		containerId, host.Id(), namesHostSpaces)

	if len(hostSpaces) == 1 {
		hostInfo := p.allSpaces.GetByID(hostSpaces.Values()[0])
		return corenetwork.SpaceInfos{*hostInfo}, nil
	}
	if len(hostSpaces) == 0 {
		logger.Debugf(context.TODO(), "container has no desired spaces, "+
			"and host has no known spaces, triggering fallback "+
			"to bridge all devices")
		alphaInfo := p.allSpaces.GetByID(corenetwork.AlphaSpaceId)
		return corenetwork.SpaceInfos{*alphaInfo}, nil
	}
	return nil, errors.Errorf("no obvious space for container %q, host machine has spaces: %s",
		containerId, namesHostSpaces)
}

func possibleBridgeTarget(dev LinkLayerDevice) (bool, error) {
	// LoopbackDevices can never be bridged
	if dev.Type() == corenetwork.LoopbackDevice || dev.Type() == corenetwork.BridgeDevice {
		return false, nil
	}
	// Devices that have no parent entry are direct host devices that can be
	// bridged.
	if dev.ParentName() == "" {
		return true, nil
	}
	// TODO(jam): 2016-12-22 This feels dirty, but it falls out of how we are
	// currently modeling VLAN objects.  see bug https://pad.lv/1652049
	if dev.Type() != corenetwork.VLAN8021QDevice {
		// Only VLAN8021QDevice have parents that still allow us to
		// bridge them.
		// When anything else has a parent set, it shouldn't be used.
		return false, nil
	}
	parentDevice, err := dev.ParentDevice()
	if err != nil {
		// If we got an error here, we have some sort of
		// database inconsistency error.
		return false, err
	}
	if parentDevice.Type() == corenetwork.EthernetDevice || parentDevice.Type() == corenetwork.BondDevice {
		// A plain VLAN device with a direct parent
		// of its underlying ethernet device.
		return true, nil
	}
	return false, nil
}

// BridgeNameForDevice returns a name to use for a new device that bridges the
// device with the input name. The policy is to:
//  1. Add br- to device name (to keep current behaviour),
//     if it does not fit in 15 characters then:
//  2. Add b- to device name, if it doesn't fit in 15 characters then:
//
// 3a. For devices starting in 'en' remove 'en' and add 'b-'
// 3b. For all other devices
//
//		'b-' + 6-char hash of name + '-' + last 6 chars of name
//	 4. If using the device name directly always replace '.' with '-'
//	    to make sure that bridges from VLANs won't break
func BridgeNameForDevice(device string) string {
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

// PopulateContainerLinkLayerDevices generates and returns link-layer devices
// for the input guest, setting each device to be a child of the corresponding
// bridge on the host machine.
// It also records when one of the desired spaces is available on the host
// machine, but not currently bridged.
func (p *BridgePolicy) PopulateContainerLinkLayerDevices(
	host Machine, guest Container, askProviderForAddress bool,
) (corenetwork.InterfaceInfos, error) {
	guestSpaces, devicesPerSpace, err := p.findSpacesAndDevicesForContainer(host, guest)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "for container %q, found host devices spaces: %s", guest.Id(), formatDeviceMap(devicesPerSpace))
	spacesFound := make(corenetwork.SpaceInfos, 0)
	devicesByName := make(map[string]LinkLayerDevice)
	bridgeDeviceNames := make([]string, 0)

	for spaceID, hostDevices := range devicesPerSpace {
		for _, hostDevice := range hostDevices {
			name := hostDevice.Name()
			if hostDevice.Type() == corenetwork.BridgeDevice && !skippedDeviceNames.Contains(name) {
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spaceInfo := p.allSpaces.GetByID(spaceID)
				spacesFound = append(spacesFound, *spaceInfo)
			}
		}
	}

	missingSpaces := guestSpaces.Minus(spacesFound)

	// Check if we are missing the default space and can fill it in with a local bridge
	if len(missingSpaces) == 1 &&
		missingSpaces.ContainsID(corenetwork.AlphaSpaceId) &&
		p.containerNetworkingMethod == containermanager.NetworkingMethodLocal {

		localBridgeName := network.DefaultLXDBridge

		for _, hostDevice := range devicesPerSpace[corenetwork.AlphaSpaceId] {
			name := hostDevice.Name()
			if hostDevice.Type() == corenetwork.BridgeDevice && name == localBridgeName {
				alphaInfo := p.allSpaces.GetByID(corenetwork.AlphaSpaceId)
				missingSpaces = missingSpaces.Minus(corenetwork.SpaceInfos{*alphaInfo})
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spacesFound = append(spacesFound, *alphaInfo)
			}
		}
	}

	if len(missingSpaces) > 0 && len(bridgeDeviceNames) == 0 {
		missingSpacesNames := missingSpaces.String()
		logger.Warningf(context.TODO(), "container %q wants spaces %s could not find host %q bridges for %s, found bridges %s",
			guest.Id(), guestSpaces,
			host.Id(), missingSpacesNames, bridgeDeviceNames)
		return nil, errors.Errorf("unable to find host bridge for space(s) %s for container %q",
			missingSpacesNames, guest.Id())
	}

	sortedBridgeDeviceNames := network.NaturallySortDeviceNames(bridgeDeviceNames...)
	logger.Debugf(context.TODO(), "for container %q using host machine %q bridge devices: %s",
		guest.Id(), host.Id(), network.QuoteSpaces(sortedBridgeDeviceNames))

	interfaces := make(corenetwork.InterfaceInfos, len(bridgeDeviceNames))

	for i, hostBridgeName := range sortedBridgeDeviceNames {
		hostBridge := devicesByName[hostBridgeName]
		newDevice, err := hostBridge.EthernetDeviceForBridge(fmt.Sprintf("eth%d", i), askProviderForAddress, p.allSubnets)
		if err != nil {
			return nil, errors.Trace(err)
		}
		interfaces[i] = newDevice
	}

	logger.Debugf(context.TODO(), "prepared container %q network config: %+v", guest.Id(), interfaces)
	return interfaces, nil
}

func formatDeviceMap(spacesToDevices map[string][]LinkLayerDevice) string {
	spaceIDs := make([]string, len(spacesToDevices))
	i := 0
	for spaceID := range spacesToDevices {
		spaceIDs[i] = spaceID
		i++
	}
	sort.Strings(spaceIDs)
	var out []string
	for _, id := range spaceIDs {
		start := fmt.Sprintf("%q:[", id)
		devices := spacesToDevices[id]
		deviceNames := make([]string, len(devices))
		for i, dev := range devices {
			deviceNames[i] = dev.Name()
		}
		deviceNames = network.NaturallySortDeviceNames(deviceNames...)
		quotedNames := make([]string, len(deviceNames))
		for i, name := range deviceNames {
			quotedNames[i] = fmt.Sprintf("%q", name)
		}
		out = append(out, start+strings.Join(quotedNames, ",")+"]")
	}
	return "map{" + strings.Join(out, ", ") + "}"
}

// ovsBridgeDevice wraps a LinkLayerDevice and overrides its reported type to
// BridgeDevice.
type ovsBridgeDevice struct {
	wrappedDev LinkLayerDevice
}

// Type ensures the wrapped device's type is always reported as bridge.
func (dev ovsBridgeDevice) Type() corenetwork.LinkLayerDeviceType { return corenetwork.BridgeDevice }
func (dev ovsBridgeDevice) Name() string                          { return dev.wrappedDev.Name() }
func (dev ovsBridgeDevice) MACAddress() string                    { return dev.wrappedDev.MACAddress() }
func (dev ovsBridgeDevice) ParentName() string                    { return dev.wrappedDev.ParentName() }
func (dev ovsBridgeDevice) ParentDevice() (LinkLayerDevice, error) {
	return dev.wrappedDev.ParentDevice()
}
func (dev ovsBridgeDevice) Addresses() ([]*state.Address, error) { return dev.wrappedDev.Addresses() }
func (dev ovsBridgeDevice) MTU() uint                            { return dev.wrappedDev.MTU() }
func (dev ovsBridgeDevice) IsUp() bool                           { return dev.wrappedDev.IsUp() }
func (dev ovsBridgeDevice) IsAutoStart() bool                    { return dev.wrappedDev.IsAutoStart() }
func (dev ovsBridgeDevice) EthernetDeviceForBridge(
	name string, askForProviderAddress bool,
	allSubnets corenetwork.SubnetInfos,
) (corenetwork.InterfaceInfo, error) {
	return dev.wrappedDev.EthernetDeviceForBridge(name, askForProviderAddress, allSubnets)
}
func (dev ovsBridgeDevice) VirtualPortType() corenetwork.VirtualPortType {
	return dev.wrappedDev.VirtualPortType()
}
