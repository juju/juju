// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerizer

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"

	// Used for some constants and things like LinkLayerDevice[Args]
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.network.containerizer")

// BridgePolicy defines functionality that helps us create and define bridges
// for guests inside of a host machine, along with the creation of network
// devices on those bridges for the containers to use.
// Ideally BridgePolicy would be defined outside of the 'state' package as it
// doesn't deal directly with DB content, but not quite enough of State is exposed
type BridgePolicy struct {
	// NetBondReconfigureDelay is how much of a delay to inject if we see that
	// one of the devices being bridged is a BondDevice. This exists because of
	// https://bugs.launchpad.net/juju/+bug/1657579
	NetBondReconfigureDelay int
	// ContainerNetworkingMethod defines the way containers are networked.
	// It's one of:
	//  - fan
	//  - provider
	//  - local
	ContainerNetworkingMethod string
}

// inferContainerSpaces tries to find a valid space for the container to be
// on. This should only be used when the container itself doesn't have any
// valid constraints on what spaces it should be in.
// If ContainerNetworkingMethod is 'local' we fall back to "" and use lxdbr0.
// If this machine is in a single space, then that space is used. Else, if
// the machine has the default space, then that space is used.
// If neither of those conditions is true, then we return an error.
func (p *BridgePolicy) inferContainerSpaces(m Machine, containerId, defaultSpaceName string) (set.Strings, error) {
	if p.ContainerNetworkingMethod == "local" {
		return set.NewStrings(""), nil
	}
	hostSpaces, err := m.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("container %q not qualified to a space, host machine %q is using spaces %s",
		containerId, m.Id(), network.QuoteSpaceSet(hostSpaces))
	if len(hostSpaces) == 1 {
		return hostSpaces, nil
	}
	if defaultSpaceName != "" && hostSpaces.Contains(defaultSpaceName) {
		return set.NewStrings(defaultSpaceName), nil
	}
	if len(hostSpaces) == 0 {
		logger.Debugf("container has no desired spaces, " +
			"and host has no known spaces, triggering fallback " +
			"to bridge all devices")
		return set.NewStrings(""), nil
	}
	return nil, errors.Errorf("no obvious space for container %q, host machine has spaces: %s",
		containerId, network.QuoteSpaceSet(hostSpaces))
}

// determineContainerSpaces tries to use the direct information about a
// container to find what spaces it should be in, and then falls back to what
// we know about the host machine.
func (p *BridgePolicy) determineContainerSpaces(m Machine, containerMachine Container, defaultSpaceName string) (set.Strings, error) {
	containerSpaces, err := containerMachine.DesiredSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("for container %q, found desired spaces: %s",
		containerMachine.Id(), network.QuoteSpaceSet(containerSpaces))
	if len(containerSpaces) == 0 {
		// We have determined that the container doesn't have any useful
		// constraints set on it. So lets see if we can come up with
		// something useful.
		containerSpaces, err = p.inferContainerSpaces(m, containerMachine.Id(), defaultSpaceName)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return containerSpaces, nil
}

// findSpacesAndDevicesForContainer looks up what spaces the container wants
// to be in, and what spaces the host machine is already in, and tries to
// find the devices on the host that are useful for the container.
func (p *BridgePolicy) findSpacesAndDevicesForContainer(m Machine, containerMachine Container) (set.Strings, map[string][]LinkLayerDevice, error) {
	containerSpaces, err := p.determineContainerSpaces(m, containerMachine, "")
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	devicesPerSpace, err := m.LinkLayerDevicesForSpaces(containerSpaces.Values())
	if err != nil {
		logger.Errorf("findSpacesAndDevicesForContainer(%q) got error looking for host spaces: %v",
			containerMachine.Id(), err)
		return nil, nil, errors.Trace(err)
	}
	return containerSpaces, devicesPerSpace, nil
}

func possibleBridgeTarget(dev LinkLayerDevice) (bool, error) {
	// LoopbackDevices can never be bridged
	if dev.Type() == state.LoopbackDevice || dev.Type() == state.BridgeDevice {
		return false, nil
	}
	// Devices that have no parent entry are direct host devices that can be
	// bridged.
	if dev.ParentName() == "" {
		return true, nil
	}
	// TODO(jam): 2016-12-22 This feels dirty, but it falls out of how we are
	// currently modeling VLAN objects.  see bug https://pad.lv/1652049
	if dev.Type() != state.VLAN_8021QDevice {
		// Only state.VLAN_8021QDevice have parents that still allow us to bridge
		// them. When anything else has a parent set, it shouldn't be used
		return false, nil
	}
	parentDevice, err := dev.ParentDevice()
	if err != nil {
		// If we got an error here, we have some sort of
		// database inconsistency error.
		return false, err
	}
	if parentDevice.Type() == state.EthernetDevice || parentDevice.Type() == state.BondDevice {
		// A plain VLAN device with a direct parent of its underlying
		// ethernet device
		return true, nil
	}
	return false, nil
}

func formatDeviceMap(spacesToDevices map[string][]LinkLayerDevice) string {
	spaceNames := make([]string, len(spacesToDevices))
	i := 0
	for spaceName := range spacesToDevices {
		spaceNames[i] = spaceName
		i++
	}
	sort.Strings(spaceNames)
	out := []string{}
	for _, name := range spaceNames {
		start := fmt.Sprintf("%q:[", name)
		devices := spacesToDevices[name]
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

var skippedDeviceNames = set.NewStrings(
	network.DefaultLXCBridge,
	network.DefaultLXDBridge,
	network.DefaultKVMBridge,
)

// The general policy is to:
// 1. Add br- to device name (to keep current behaviour), if it doesn fit in 15 characters then:
// 2. Add b- to device name, if it doesn't fit in 15 characters then:
// 3a. For devices starting in 'en' remove 'en' and add 'b-'
// 3b. For all other devices 'b-' + 6-char hash of name + '-' + last 6 chars of name
func BridgeNameForDevice(device string) string {
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

// FindMissingBridgesForContainer looks at the spaces that the container
// wants to be in, and sees if there are any host devices that should be
// bridged.
// This will return an Error if the container wants a space that the host
// machine cannot provide.
func (b *BridgePolicy) FindMissingBridgesForContainer(m Machine, containerMachine Container) ([]network.DeviceToBridge, int, error) {
	reconfigureDelay := 0
	containerSpaces, devicesPerSpace, err := b.findSpacesAndDevicesForContainer(m, containerMachine)
	hostDeviceByName := make(map[string]LinkLayerDevice, 0)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	logger.Debugf("FindMissingBridgesForContainer(%q) spaces %s devices %v",
		containerMachine.Id(), network.QuoteSpaceSet(containerSpaces),
		formatDeviceMap(devicesPerSpace))
	spacesFound := set.NewStrings()
	fanSpacesFound := set.NewStrings()
	for spaceName, devices := range devicesPerSpace {
		for _, device := range devices {
			if device.Type() == state.BridgeDevice {
				if b.ContainerNetworkingMethod != "local" && skippedDeviceNames.Contains(device.Name()) {
					continue
				}
				if strings.HasPrefix(device.Name(), "fan-") {
					fanSpacesFound.Add(spaceName)
				} else {
					spacesFound.Add(spaceName)
				}
			}
		}
	}
	notFound := containerSpaces.Difference(spacesFound)
	fanNotFound := containerSpaces.Difference(fanSpacesFound)
	if b.ContainerNetworkingMethod == "fan" {
		if fanNotFound.IsEmpty() {
			// Nothing to do, just return success
			return nil, 0, nil
		} else {
			return nil, 0, errors.Errorf("host machine %q has no available FAN devices in space(s) %s",
				m.Id(), network.QuoteSpaceSet(fanNotFound))
		}
	} else {
		if notFound.IsEmpty() {
			// Nothing to do, just return success
			return nil, 0, nil
		}
	}
	hostDeviceNamesToBridge := make([]string, 0)
	for _, spaceName := range notFound.Values() {
		hostDeviceNames := make([]string, 0)
		for _, hostDevice := range devicesPerSpace[spaceName] {
			possible, err := possibleBridgeTarget(hostDevice)
			if err != nil {
				return nil, 0, err
			}
			if !possible {
				continue
			}
			hostDeviceNames = append(hostDeviceNames, hostDevice.Name())
			hostDeviceByName[hostDevice.Name()] = hostDevice
			spacesFound.Add(spaceName)
		}
		if len(hostDeviceNames) > 0 {
			if spaceName == "" {
				// When we are bridging unknown space devices, we bridge all
				// of them. Both because this is a fallback, and because we
				// don't know what the exact spaces are going to be.
				for _, deviceName := range hostDeviceNames {
					hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, deviceName)
					if hostDeviceByName[deviceName].Type() == state.BondDevice {
						if reconfigureDelay < b.NetBondReconfigureDelay {
							reconfigureDelay = b.NetBondReconfigureDelay
						}
					}
				}
			} else {
				// This should already be sorted from
				// LinkLayerDevicesForSpaces but sorting to be sure we stably
				// pick the host device
				hostDeviceNames = network.NaturallySortDeviceNames(hostDeviceNames...)
				hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, hostDeviceNames[0])
				if hostDeviceByName[hostDeviceNames[0]].Type() == state.BondDevice {
					if reconfigureDelay < b.NetBondReconfigureDelay {
						reconfigureDelay = b.NetBondReconfigureDelay
					}
				}
			}
		}
	}
	notFound = notFound.Difference(spacesFound)
	if !notFound.IsEmpty() {
		hostSpaces, err := m.AllSpaces()
		if err != nil {
			// log it, but we're returning another error right now
			logger.Warningf("got error looking for spaces for host machine %q: %v",
				m.Id(), err)
		}
		logger.Warningf("container %q wants spaces %s, but host machine %q has %s missing %s",
			containerMachine.Id(), network.QuoteSpaceSet(containerSpaces),
			m.Id(), network.QuoteSpaceSet(hostSpaces), network.QuoteSpaceSet(notFound))
		return nil, 0, errors.Errorf("host machine %q has no available device in space(s) %s",
			m.Id(), network.QuoteSpaceSet(notFound))
	}

	hostToBridge := make([]network.DeviceToBridge, 0, len(hostDeviceNamesToBridge))
	for _, hostName := range network.NaturallySortDeviceNames(hostDeviceNamesToBridge...) {
		hostToBridge = append(hostToBridge, network.DeviceToBridge{
			DeviceName: hostName,
			BridgeName: BridgeNameForDevice(hostName),
			MACAddress: hostDeviceByName[hostName].MACAddress(),
		})
	}
	return hostToBridge, reconfigureDelay, nil
}

// PopulateContainerLinkLayerDevices sets the link-layer devices of the given
// containerMachine, setting each device linked to the corresponding
// BridgeDevice of the host machine. It also records when one of the
// desired spaces is available on the host machine, but not currently
// bridged.
func (p *BridgePolicy) PopulateContainerLinkLayerDevices(m Machine, containerMachine Container) error {
	// TODO(jam): 20017-01-31 This doesn't quite feel right that we would be
	// defining devices that 'will' exist in the container, but don't exist
	// yet. If anything, this feels more like "Provider" level devices, because
	// it is defining the devices from the outside, not the inside.
	containerSpaces, devicesPerSpace, err := p.findSpacesAndDevicesForContainer(m, containerMachine)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("for container %q, found host devices spaces: %s",
		containerMachine.Id(), formatDeviceMap(devicesPerSpace))

	localBridgeForType := map[instance.ContainerType]string{
		instance.LXD: network.DefaultLXDBridge,
		instance.KVM: network.DefaultKVMBridge,
	}
	spacesFound := set.NewStrings()
	devicesByName := make(map[string]LinkLayerDevice)
	bridgeDeviceNames := make([]string, 0)

	for spaceName, hostDevices := range devicesPerSpace {
		for _, hostDevice := range hostDevices {
			isFan := strings.HasPrefix(hostDevice.Name(), "fan-")
			wantThisDevice := isFan == (p.ContainerNetworkingMethod == "fan")
			deviceType, name := hostDevice.Type(), hostDevice.Name()
			if wantThisDevice && deviceType == state.BridgeDevice && !skippedDeviceNames.Contains(name) {
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spacesFound.Add(spaceName)
			}
		}
	}
	missingSpace := containerSpaces.Difference(spacesFound)

	// Check if we are missing "" and can fill it in with a local bridge
	if len(missingSpace) == 1 && missingSpace.Contains("") && p.ContainerNetworkingMethod == "local" {
		localBridgeName := localBridgeForType[containerMachine.ContainerType()]
		for _, hostDevice := range devicesPerSpace[""] {
			name := hostDevice.Name()
			if hostDevice.Type() == state.BridgeDevice && name == localBridgeName {
				missingSpace.Remove("")
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spacesFound.Add("")
			}
		}
	}
	if len(missingSpace) > 0 {
		logger.Warningf("container %q wants spaces %s could not find host %q bridges for %s, found bridges %s",
			containerMachine.Id(), network.QuoteSpaceSet(containerSpaces),
			m.Id(), network.QuoteSpaceSet(missingSpace), bridgeDeviceNames)
		return errors.Errorf("unable to find host bridge for space(s) %s for container %q",
			network.QuoteSpaceSet(missingSpace), containerMachine.Id())
	}

	sortedBridgeDeviceNames := network.NaturallySortDeviceNames(bridgeDeviceNames...)
	logger.Debugf("for container %q using host machine %q bridge devices: %s",
		containerMachine.Id(), m.Id(), network.QuoteSpaces(sortedBridgeDeviceNames))
	containerDevicesArgs := make([]state.LinkLayerDeviceArgs, len(bridgeDeviceNames))

	for i, hostBridgeName := range sortedBridgeDeviceNames {
		hostBridge := devicesByName[hostBridgeName]
		newLLD, err := hostBridge.EthernetDeviceForBridge(fmt.Sprintf("eth%d", i))
		if err != nil {
			return errors.Trace(err)
		}
		containerDevicesArgs[i] = newLLD
	}
	logger.Debugf("prepared container %q network config: %+v", containerMachine.Id(), containerDevicesArgs)

	if err := containerMachine.SetLinkLayerDevices(containerDevicesArgs...); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("container %q network config set", containerMachine.Id())
	return nil
}
