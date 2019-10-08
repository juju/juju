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

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.network.containerizer")

var skippedDeviceNames = set.NewStrings(
	network.DefaultLXCBridge,
	network.DefaultLXDBridge,
	network.DefaultKVMBridge,
)

// BridgePolicy defines functionality that helps us create and define bridges
// for guests inside of a host machine, along with the creation of network
// devices on those bridges for the containers to use.
type BridgePolicy struct {
	// spaces is a cache of the model's spaces.
	spaces Spaces

	// netBondReconfigureDelay is how much of a delay to inject if we see that
	// one of the devices being bridged is a BondDevice. This exists because of
	// https://bugs.launchpad.net/juju/+bug/1657579
	netBondReconfigureDelay int

	// containerNetworkingMethod defines the way containers are networked.
	// It's one of:
	//  - fan
	//  - provider
	//  - local
	containerNetworkingMethod string
}

// NewBridgePolicy returns a new BridgePolicy for the input environ config
// getter and state indirection.
func NewBridgePolicy(cfgGetter environs.ConfigGetter, st SpaceBacking) (*BridgePolicy, error) {
	cfg := cfgGetter.Config()

	spaces, err := NewSpaces(st)
	if err != nil {
		return nil, errors.Annotate(err, "creating spaces cache")
	}

	return &BridgePolicy{
		spaces:                    spaces,
		netBondReconfigureDelay:   cfg.NetBondReconfigureDelay(),
		containerNetworkingMethod: cfg.ContainerNetworkingMethod(),
	}, nil
}

// FindMissingBridgesForContainer looks at the spaces that the container should
// have access to, and returns any host devices need to be bridged for use as
// the container network.
// This will return an Error if the container requires a space that the host
// machine cannot provide.
func (p *BridgePolicy) FindMissingBridgesForContainer(
	host Machine, guest Container,
) ([]network.DeviceToBridge, int, error) {
	guestSpaces, devicesPerSpace, err := p.findSpacesAndDevicesForContainer(host, guest)
	if err != nil {
		return nil, 0, errors.Trace(err)
	}
	logger.Debugf("FindMissingBridgesForContainer(%q) spaces %s devices %v",
		guest.Id(), guestSpaces.String(), formatDeviceMap(devicesPerSpace))

	spacesFound := set.NewStrings()
	fanSpacesFound := set.NewStrings()
	for spaceName, devices := range devicesPerSpace {
		for _, device := range devices {
			if device.Type() == corenetwork.BridgeDevice {
				if p.containerNetworkingMethod != "local" && skippedDeviceNames.Contains(device.Name()) {
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

	// TODO (manadart 2019-09-27): This is ugly, but once everything is
	// consistently reasoning about spaces in terms of IDs, we should implement
	// this kind of diffing on SpaceInfos.
	// network.QuoteSpaceSet can be removed at that time too.
	guestSpaceSet := set.NewStrings(guestSpaces.Names()...)
	notFound := guestSpaceSet.Difference(spacesFound)
	fanNotFound := guestSpaceSet.Difference(fanSpacesFound)

	if p.containerNetworkingMethod == "fan" {
		if fanNotFound.IsEmpty() {
			// Nothing to do; just return success.
			return nil, 0, nil
		}
		return nil, 0, errors.Errorf("host machine %q has no available FAN devices in space(s) %s",
			host.Id(), network.QuoteSpaceSet(fanNotFound))
	}

	if notFound.IsEmpty() {
		// Nothing to do; just return success.
		return nil, 0, nil
	}

	hostDeviceNamesToBridge := make([]string, 0)
	reconfigureDelay := 0
	hostDeviceByName := make(map[string]LinkLayerDevice, 0)
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
			if spaceName == corenetwork.DefaultSpaceName {
				// When we are bridging unknown space devices, we bridge all
				// of them. Both because this is a fallback, and because we
				// don't know what the exact spaces are going to be.
				for _, deviceName := range hostDeviceNames {
					hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, deviceName)
					if hostDeviceByName[deviceName].Type() == corenetwork.BondDevice {
						if reconfigureDelay < p.netBondReconfigureDelay {
							reconfigureDelay = p.netBondReconfigureDelay
						}
					}
				}
			} else {
				// This should already be sorted from
				// LinkLayerDevicesForSpaces but sorting to be sure we stably
				// pick the host device
				hostDeviceNames = network.NaturallySortDeviceNames(hostDeviceNames...)
				hostDeviceNamesToBridge = append(hostDeviceNamesToBridge, hostDeviceNames[0])
				if hostDeviceByName[hostDeviceNames[0]].Type() == corenetwork.BondDevice {
					if reconfigureDelay < p.netBondReconfigureDelay {
						reconfigureDelay = p.netBondReconfigureDelay
					}
				}
			}
		}
	}
	notFound = notFound.Difference(spacesFound)
	if !notFound.IsEmpty() {
		hostSpaces, err := host.AllSpaces()
		if err != nil {
			// log it, but we're returning another error right now
			logger.Warningf("got error looking for spaces for host machine %q: %v",
				host.Id(), err)
		}
		logger.Warningf("container %q wants spaces %s, but host machine %q has %s missing %s",
			guest.Id(), network.QuoteSpaceSet(guestSpaceSet),
			host.Id(), network.QuoteSpaceSet(hostSpaces), network.QuoteSpaceSet(notFound))
		return nil, 0, errors.Errorf("host machine %q has no available device in space(s) %s",
			host.Id(), network.QuoteSpaceSet(notFound))
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

// findSpacesAndDevicesForContainer looks up what spaces the container wants
// to be in, and what spaces the host machine is already in, and tries to
// find the devices on the host that are useful for the container.
func (p *BridgePolicy) findSpacesAndDevicesForContainer(
	host Machine, guest Container,
) (corenetwork.SpaceInfos, map[string][]LinkLayerDevice, error) {
	containerSpaces, err := p.determineContainerSpaces(host, guest, corenetwork.DefaultSpaceName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	devicesPerSpace, err := host.LinkLayerDevicesForSpaces(containerSpaces)
	if err != nil {
		logger.Errorf("findSpacesAndDevicesForContainer(%q) got error looking for host spaces: %v",
			guest.Id(), err)
		return nil, nil, errors.Trace(err)
	}
	return containerSpaces, devicesPerSpace, nil
}

// determineContainerSpaces tries to use the direct information about a
// container to find what spaces it should be in, and then falls back to what
// we know about the host machine.
func (p *BridgePolicy) determineContainerSpaces(
	host Machine, guest Container, defaultSpaceName string,
) (corenetwork.SpaceInfos, error) {
	spaces := set.NewStrings()

	// Gather any *positive* space constraints for the guest.
	cons, err := guest.Constraints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if cons.Spaces != nil {
		for _, space := range *cons.Spaces {
			if !strings.HasPrefix(space, "^") {
				spaces.Add(space)
			}
		}
	}

	// Gather any space bindings for application endpoints
	// that apply to units the the container will host.
	units, err := guest.Units()
	if err != nil {
		return nil, errors.Trace(err)
	}
	bindings := set.NewStrings()
	for _, unit := range units {
		app, err := unit.Application()
		if err != nil {
			return nil, errors.Trace(err)
		}
		endpointBindings, err := app.EndpointBindings()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, space := range endpointBindings {
			if space != corenetwork.DefaultSpaceName {
				bindings.Add(space)
			}
		}
	}

	logger.Tracef("machine %q found constraints %s and bindings %s",
		guest.Id(), network.QuoteSpaceSet(spaces), network.QuoteSpaceSet(bindings))

	spaces = spaces.Union(bindings)
	logger.Debugf("for container %q, found desired spaces: %s", guest.Id(), network.QuoteSpaceSet(spaces))

	if len(spaces) == 0 {
		// We have determined that the container doesn't have any useful
		// constraints set on it. So lets see if we can come up with
		// something useful.
		spaces, err = p.inferContainerSpaces(host, guest.Id(), defaultSpaceName)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	// TODO (manadart 2019-09-27): The stages above still return space names.
	// This will need to evolve for at least endpoint bindings,
	// which will ultimately be return space IDs.
	spaceInfos := make(corenetwork.SpaceInfos, len(spaces))
	for i, space := range spaces.Values() {
		if spaceInfos[i], err = p.spaces.GetByName(space); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return spaceInfos, nil
}

// inferContainerSpaces tries to find a valid space for the container to be
// on. This should only be used when the container itself doesn't have any
// valid constraints on what spaces it should be in.
// If containerNetworkingMethod is 'local' we fall back to "" and use lxdbr0.
// If this machine is in a single space, then that space is used. Else, if
// the machine has the default space, then that space is used.
// If neither of those conditions is true, then we return an error.
func (p *BridgePolicy) inferContainerSpaces(host Machine, containerId, defaultSpaceName string) (set.Strings, error) {
	if p.containerNetworkingMethod == "local" {
		return set.NewStrings(corenetwork.DefaultSpaceName), nil
	}
	hostSpaces, err := host.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("container %q not qualified to a space, host machine %q is using spaces %s",
		containerId, host.Id(), network.QuoteSpaceSet(hostSpaces))
	if len(hostSpaces) == 1 {
		return hostSpaces, nil
	}
	if defaultSpaceName != corenetwork.DefaultSpaceName && hostSpaces.Contains(defaultSpaceName) {
		return set.NewStrings(defaultSpaceName), nil
	}
	if len(hostSpaces) == 0 {
		logger.Debugf("container has no desired spaces, " +
			"and host has no known spaces, triggering fallback " +
			"to bridge all devices")
		return set.NewStrings(corenetwork.DefaultSpaceName), nil
	}
	return nil, errors.Errorf("no obvious space for container %q, host machine has spaces: %s",
		containerId, network.QuoteSpaceSet(hostSpaces))
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

// The general policy is to:
// 1.  Add br- to device name (to keep current behaviour),
//     if it does not fit in 15 characters then:
// 2.  Add b- to device name, if it doesn't fit in 15 characters then:
// 3a. For devices starting in 'en' remove 'en' and add 'b-'
// 3b. For all other devices
//     'b-' + 6-char hash of name + '-' + last 6 chars of name
// 4.  If using the device name directly always replace '.' with '-'
//     to make sure that bridges from VLANs won't break
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

// PopulateContainerLinkLayerDevices sets the link-layer devices of the input
// guest, setting each device to be a child of the corresponding bridge on the
// host machine.
// It also records when one of the desired spaces is available on the host
// machine, but not currently bridged.
func (p *BridgePolicy) PopulateContainerLinkLayerDevices(host Machine, guest Container) error {
	// TODO(jam): 20017-01-31 This doesn't quite feel right that we would be
	// defining devices that 'will' exist in the container, but don't exist
	// yet. If anything, this feels more like "Provider" level devices, because
	// it is defining the devices from the outside, not the inside.
	guestSpaces, devicesPerSpace, err := p.findSpacesAndDevicesForContainer(host, guest)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("for container %q, found host devices spaces: %s", guest.Id(), formatDeviceMap(devicesPerSpace))
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
			wantThisDevice := isFan == (p.containerNetworkingMethod == "fan")
			deviceType, name := hostDevice.Type(), hostDevice.Name()
			if wantThisDevice && deviceType == corenetwork.BridgeDevice && !skippedDeviceNames.Contains(name) {
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spacesFound.Add(spaceName)
			}
		}
	}

	guestSpaceSet := set.NewStrings(guestSpaces.Names()...)
	missingSpaces := guestSpaceSet.Difference(spacesFound)

	// Check if we are missing "" and can fill it in with a local bridge
	if len(missingSpaces) == 1 &&
		missingSpaces.Contains(corenetwork.DefaultSpaceName) &&
		p.containerNetworkingMethod == "local" {
		localBridgeName := localBridgeForType[guest.ContainerType()]
		for _, hostDevice := range devicesPerSpace[corenetwork.DefaultSpaceName] {
			name := hostDevice.Name()
			if hostDevice.Type() == corenetwork.BridgeDevice && name == localBridgeName {
				missingSpaces.Remove(corenetwork.DefaultSpaceName)
				devicesByName[name] = hostDevice
				bridgeDeviceNames = append(bridgeDeviceNames, name)
				spacesFound.Add(corenetwork.DefaultSpaceName)
			}
		}
	}
	if len(missingSpaces) > 0 {
		logger.Warningf("container %q wants spaces %s could not find host %q bridges for %s, found bridges %s",
			guest.Id(), network.QuoteSpaceSet(guestSpaceSet),
			host.Id(), network.QuoteSpaceSet(missingSpaces), bridgeDeviceNames)
		return errors.Errorf("unable to find host bridge for space(s) %s for container %q",
			network.QuoteSpaceSet(missingSpaces), guest.Id())
	}

	sortedBridgeDeviceNames := network.NaturallySortDeviceNames(bridgeDeviceNames...)
	logger.Debugf("for container %q using host machine %q bridge devices: %s",
		guest.Id(), host.Id(), network.QuoteSpaces(sortedBridgeDeviceNames))
	containerDevicesArgs := make([]state.LinkLayerDeviceArgs, len(bridgeDeviceNames))

	for i, hostBridgeName := range sortedBridgeDeviceNames {
		hostBridge := devicesByName[hostBridgeName]
		newLLD, err := hostBridge.EthernetDeviceForBridge(fmt.Sprintf("eth%d", i))
		if err != nil {
			return errors.Trace(err)
		}
		containerDevicesArgs[i] = newLLD
	}
	logger.Debugf("prepared container %q network config: %+v", guest.Id(), containerDevicesArgs)

	if err := guest.SetLinkLayerDevices(containerDevicesArgs...); err != nil {
		return errors.Trace(err)
	}

	logger.Debugf("container %q network config set", guest.Id())
	return nil
}

func formatDeviceMap(spacesToDevices map[string][]LinkLayerDevice) string {
	spaceNames := make([]string, len(spacesToDevices))
	i := 0
	for spaceName := range spacesToDevices {
		spaceNames[i] = spaceName
		i++
	}
	sort.Strings(spaceNames)
	var out []string
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
