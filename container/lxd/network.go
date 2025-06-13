// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

const (
	nic            = "nic"
	nicTypeBridged = "bridged"
	nicTypeMACVLAN = "macvlan"
	netTypeBridge  = "bridge"
)

// device is a type alias for profile devices.
type device = map[string]string

// LocalBridgeName returns the name of the local LXD network bridge.
func (s *Server) LocalBridgeName() string {
	return s.localBridgeName
}

// EnableHTTPSListener configures LXD to listen for HTTPS requests, rather than
// only via a Unix socket. Attempts to listen on all protocols, but falls back
// to IPv4 only if IPv6 has been disabled with in kernel.
// Returns an error if updating the server configuration fails.
func (s *Server) EnableHTTPSListener() error {
	// Make sure the LXD service is configured to listen to local https
	// requests, rather than only via the Unix socket.
	// TODO: jam 2016-02-25 This tells LXD to listen on all addresses,
	//      which does expose the LXD to outside requests. It would
	//      probably be better to only tell LXD to listen for requests on
	//      the loopback and LXC bridges that we are using.
	if err := s.UpdateServerConfig(map[string]string{
		"core.https_address": "[::]",
	}); err != nil {
		cause := errors.Cause(err)
		if strings.HasSuffix(cause.Error(), errIPV6NotSupported) {
			// Fall back to IPv4 only.
			return errors.Trace(s.UpdateServerConfig(map[string]string{
				"core.https_address": "0.0.0.0",
			}))
		}
		return errors.Trace(err)
	}
	return nil
}

// EnsureIPv4 retrieves the network for the input name and checks its IPv4
// configuration. If none is detected, it is set to "auto".
// The boolean return indicates if modification was necessary.
func (s *Server) EnsureIPv4(netName string) (bool, error) {
	var modified bool

	net, eTag, err := s.GetNetwork(netName)
	if err != nil {
		return false, errors.Trace(err)
	}

	cfg, ok := net.Config["ipv4.address"]
	if !ok || cfg == "none" {
		if net.Config == nil {
			net.Config = make(device, 2)
		}
		net.Config["ipv4.address"] = "auto"
		net.Config["ipv4.nat"] = "true"

		if err := s.UpdateNetwork(netName, net.Writable(), eTag); err != nil {
			return false, errors.Trace(err)
		}
		modified = true
	}

	return modified, nil
}

// GetNICsFromProfile returns all NIC devices in the profile with the input
// name. All returned devices have a MAC address; generated if required.
func (s *Server) GetNICsFromProfile(profileName string) (map[string]device, error) {
	profile, _, err := s.GetProfile(profileName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	nics := getProfileNICs(profile)
	for name := range nics {
		if nics[name]["hwaddr"] == "" {
			nics[name]["hwaddr"] = corenetwork.GenerateVirtualMACAddress()
		}
	}
	return nics, nil
}

// VerifyNetworkDevice attempts to ensure that there is a network usable by LXD
// and that there is a NIC device with said network as its parent.
// If there are no NIC devices, and this server is *not* in cluster mode,
// an attempt is made to create an new device in the input profile,
// with the default LXD bridge as its parent.
func (s *Server) VerifyNetworkDevice(profile *api.Profile, eTag string) error {
	nics := getProfileNICs(profile)

	if len(nics) == 0 {
		if s.networkAPISupport && !s.clustered {
			return errors.Annotate(s.ensureDefaultNetworking(profile, eTag), "ensuring default bridge config")
		}
		return errors.Errorf("profile %q does not have any devices configured with type %q", profile.Name, nic)
	}

	if s.networkAPISupport {
		return errors.Annotatef(s.verifyNICsWithAPI(nics), "profile %q", profile.Name)
	}

	return errors.Annotatef(s.verifyNICsWithConfigFile(nics, ioutil.ReadFile), "profile %q", profile.Name)
}

// ensureDefaultNetworking ensures that the default LXD bridge exists,
// that it is not configured to use IPv6, and that a NIC device exists in
// the input profile.
// An error is returned if the bridge exists with IPv6 configuration.
// If the bridge does not exist, it is created.
func (s *Server) ensureDefaultNetworking(profile *api.Profile, eTag string) error {
	net, _, err := s.GetNetwork(network.DefaultLXDBridge)
	if err != nil {
		if !IsLXDNotFound(err) {
			return errors.Trace(err)
		}
		req := api.NetworksPost{
			Name: network.DefaultLXDBridge,
			Type: netTypeBridge,
			NetworkPut: api.NetworkPut{Config: map[string]string{
				"ipv4.address": "auto",
				"ipv4.nat":     "true",
				"ipv6.address": "none",
				"ipv6.nat":     "false",
			}},
		}
		err := s.CreateNetwork(req)
		if err != nil {
			return errors.Trace(err)
		}
		net, _, err = s.GetNetwork(network.DefaultLXDBridge)
		if err != nil {
			return errors.Trace(err)
		}
	}

	s.localBridgeName = network.DefaultLXDBridge

	nicName := generateNICDeviceName(profile)
	if nicName == "" {
		return errors.Errorf("failed to generate a unique device name for profile %q", profile.Name)
	}

	// Add the new device with the bridge as its parent.
	nicType := nicTypeMACVLAN
	if net.Type == netTypeBridge {
		nicType = nicTypeBridged
	}
	profile.Devices[nicName] = device{
		"type":    nic,
		"nictype": nicType,
		"parent":  network.DefaultLXDBridge,
	}

	if err := s.UpdateProfile(profile.Name, profile.Writable(), eTag); err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("created new nic device %q in profile %q", nicName, profile.Name)
	return nil
}

// verifyNICsWithAPI uses the LXD network API to check if one of the input NIC
// devices is suitable for LXD to work with Juju.
func (s *Server) verifyNICsWithAPI(nics map[string]device) error {
	checked := make([]string, 0, len(nics))
	for name, nic := range nics {
		checked = append(checked, name)

		// Versions of the LXD profile prior to 3.22 have the network name as
		// "parent" under NIC entries in the "devices" list.
		// Later versions have it under "network".
		netName, ok := nic["network"]
		if !ok {
			netName = nic["parent"]
		}
		if netName == "" {
			continue
		}

		net, _, err := s.GetNetwork(netName)
		if err != nil {
			return errors.Annotatef(err, "retrieving network %q", netName)
		}

		// Versions of the LXD profile prior to 3.22 have a "nictype" member
		// under NIC entries in the "devices" list.
		// Later versions were observed to have this member absent,
		// however this information is available from the actual network.
		if !isValidNetworkType(net) && !isValidNICType(nic) {
			continue
		}

		logger.Tracef("found usable network device %q with parent %q", name, netName)
		s.localBridgeName = netName
		return nil
	}

	// No nics with a nictype of nicTypeBridged, nicTypeMACVLAN was found.
	return errors.Errorf(
		"no network device found with nictype %q or %q"+
			"\n\tthe following devices were checked: %s"+
			"\nReconfigure lxd to use a network of type %q or %q.",
		nicTypeBridged, nicTypeMACVLAN, strings.Join(checked, ", "), nicTypeBridged, nicTypeMACVLAN)
}

// verifyNICsWithConfigFile is recruited for legacy LXD installations.
// It checks the LXD bridge configuration file and ensure that one of the input
// devices is suitable for LXD to work with Juju.
func (s *Server) verifyNICsWithConfigFile(nics map[string]device, reader func(string) ([]byte, error)) error {
	netName, err := checkBridgeConfigFile(reader)
	if err != nil {
		return errors.Trace(err)
	}

	checked := make([]string, 0, len(nics))
	for name, nic := range nics {
		checked = append(checked, name)

		if nic["parent"] != netName {
			continue
		}
		if !isValidNICType(nic) {
			continue
		}

		logger.Tracef("found usable network device %q with parent %q", name, netName)
		s.localBridgeName = netName
		return nil
	}

	return errors.Errorf("no network device found with nictype %q or %q that uses the configured bridge in %s"+
		"\n\tthe following devices were checked: %v", nicTypeBridged, nicTypeMACVLAN, BridgeConfigFile, checked)
}

// generateNICDeviceName attempts to generate a new NIC device name that is not
// already in the input profile. If none can be determined in a reasonable
// search space, an empty name is returned. This should never really happen,
// but the name generation aborts to be safe from (theoretical) integer overflow.
func generateNICDeviceName(profile *api.Profile) string {
	template := "eth%d"
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf(template, i)
		unique := true
		for d := range profile.Devices {
			if d == name {
				unique = false
				break
			}
		}
		if unique {
			return name
		}
	}
	return ""
}

// getProfileNICs iterates over the devices in the input profile and returns
// any that are of type "nic".
func getProfileNICs(profile *api.Profile) map[string]device {
	nics := make(map[string]device, len(profile.Devices))
	for k, v := range profile.Devices {
		if v["type"] == nic {
			nics[k] = v
		}
	}
	return nics
}

func isValidNICType(nic device) bool {
	return nic["nictype"] == nicTypeBridged || nic["nictype"] == nicTypeMACVLAN
}

func isValidNetworkType(net *api.Network) bool {
	return net.Type == netTypeBridge || net.Type == nicTypeMACVLAN
}

const BridgeConfigFile = "/etc/default/lxd-bridge"
const SnapBridgeConfigFile = "/var/snap/lxd/common/lxd-bridge/config"

// checkBridgeConfigFile verifies that the file configuration for the LXD
// bridge has a a bridge name, that it is set to be used by LXD and that
// it has (only) IPv4 configuration.
// TODO (manadart 2018-05-28) The error messages are invalid for LXD
// installations that pre-date the network API support and that were installed
// via Snap. The question of the correct user action was posed on the #lxd IRC
// channel, but has not be answered to-date.
func checkBridgeConfigFile(reader func(string) ([]byte, error)) (string, error) {
	// installed via snap is used to customise the error message, so that if
	// you're running apt install on older series than bionic then it will
	// still show the older messages.
	installedViaSnap := lxdViaSnap()
	fileName := BridgeConfigFile
	if installedViaSnap {
		fileName = SnapBridgeConfigFile
	}
	bridgeConfig, err := reader(fileName)
	if os.IsNotExist(err) {
		return "", bridgeConfigError("no config file found at "+fileName, installedViaSnap)
	} else if err != nil {
		return "", errors.Trace(err)
	}

	foundSubnetConfig := false
	bridgeName := ""
	for _, line := range strings.Split(string(bridgeConfig), "\n") {
		if strings.HasPrefix(line, "USE_LXD_BRIDGE=") {
			b, err := strconv.ParseBool(strings.Trim(line[len("USE_LXD_BRIDGE="):], " \""))
			if err != nil {
				logger.Debugf("unable to parse bool, skipping USE_LXD_BRIDGE check: %s", err)
				continue
			}
			if !b {
				return "", bridgeConfigError(fmt.Sprintf("%s has USE_LXD_BRIDGE set to false", fileName), installedViaSnap)
			}
		} else if strings.HasPrefix(line, "LXD_BRIDGE=") {
			bridgeName = strings.Trim(line[len("LXD_BRIDGE="):], " \"")
			if bridgeName == "" {
				return "", bridgeConfigError(fmt.Sprintf("%s has no LXD_BRIDGE set", fileName), installedViaSnap)
			}
		} else if strings.HasPrefix(line, "LXD_IPV4_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV4_ADDR="):], " \"")
			if len(contents) > 0 {
				foundSubnetConfig = true
			}
		} else if strings.HasPrefix(line, "LXD_IPV6_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV6_ADDR="):], " \"")
			if len(contents) > 0 {
				return "", ipv6BridgeConfigError(fileName, installedViaSnap)
			}
		}
	}

	if !foundSubnetConfig {
		// TODO (hml) 2018-08-09 Question
		// Should the error mention ipv6 is not enabled if juju doesn't support it?
		return "", bridgeConfigError(bridgeName+" has no ipv4 or ipv6 subnet enabled", installedViaSnap)
	}
	return bridgeName, nil
}

func bridgeConfigError(err string, installedViaSnap bool) error {
	errMsg := "%s\nIt looks like your LXD bridge has not yet been configured."
	if !installedViaSnap {
		errMsg += " Configure it via:\n\n" +
			"\tsudo dpkg-reconfigure -p medium lxd\n\n" +
			"and run the command again."
	}
	return errors.Errorf(errMsg, err)
}

func ipv6BridgeConfigError(fileName string, installedViaSnap bool) error {
	errMsg := "%s has IPv6 enabled.\nJuju doesn't currently support IPv6."
	if !installedViaSnap {
		errMsg += "\n" +
			"Disable IPv6 via:\n\n" +
			"\tsudo dpkg-reconfigure -p medium lxd\n\n" +
			"and run the command again."
	}
	return errors.Errorf(errMsg, fileName)
}

// InterfaceInfoFromDevices returns a slice of interface info congruent with the
// input LXD NIC devices.
// The output is used to generate cloud-init user-data congruent with the NICs
// that end up in the container.
func InterfaceInfoFromDevices(nics map[string]device) (corenetwork.InterfaceInfos, error) {
	interfaces := make(corenetwork.InterfaceInfos, len(nics))
	var i int
	for name, device := range nics {
		iInfo := corenetwork.InterfaceInfo{
			InterfaceName:       name,
			ParentInterfaceName: device["parent"],
			MACAddress:          device["hwaddr"],
			ConfigType:          corenetwork.ConfigDHCP,
			Origin:              corenetwork.OriginProvider,
		}
		if device["mtu"] != "" {
			mtu, err := strconv.Atoi(device["mtu"])
			if err != nil {
				return nil, errors.Annotate(err, "parsing device MTU")
			}
			iInfo.MTU = mtu
		}

		interfaces[i] = iInfo
		i++
	}

	sortInterfacesByName(interfaces)
	return interfaces, nil
}

type interfaceInfoSlice corenetwork.InterfaceInfos

func (s interfaceInfoSlice) Len() int      { return len(s) }
func (s interfaceInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s interfaceInfoSlice) Less(i, j int) bool {
	return s[i].InterfaceName < s[j].InterfaceName
}

func sortInterfacesByName(interfaces corenetwork.InterfaceInfos) {
	sort.Sort(interfaceInfoSlice(interfaces))
}

// errIPV6NotSupported is the error returned by glibc for attempts at
// unsupported protocols.
const errIPV6NotSupported = `socket: address family not supported by protocol`

// DevicesFromInterfaceInfo uses the input interface info collection to create
// a map of network device configuration in the LXD format. Names for any
// networks without a known CIDR are returned in a slice.
func DevicesFromInterfaceInfo(interfaces corenetwork.InterfaceInfos) (map[string]device, []string, error) {
	nics := make(map[string]device, len(interfaces))
	var unknown []string
	for _, v := range interfaces {
		if v.InterfaceType == corenetwork.LoopbackDevice {
			continue
		}
		if v.InterfaceType != corenetwork.EthernetDevice {
			return nil, nil, errors.Errorf("interface type %q not supported", v.InterfaceType)
		}
		if v.ParentInterfaceName == "" {
			return nil, nil, errors.Errorf("parent interface name is empty")
		}
		if v.PrimaryAddress().CIDR == "" {
			unknown = append(unknown, v.ParentInterfaceName)
		}
		nics[v.InterfaceName] = newNICDevice(v.InterfaceName, v.ParentInterfaceName, v.MACAddress, v.MTU)
	}

	return nics, unknown, nil
}

// newNICDevice creates and returns a LXD-compatible config for a network
// device from the input arguments.
// TODO (manadart 2018-06-21) We want to support nictype=macvlan too.
// This will involve interrogating the parent device, via the server if it is
// LXD managed, or via the container.NetworkConfig.DeviceType that this is
// being generated from.
func newNICDevice(deviceName, parentDevice, hwAddr string, mtu int) device {
	device := map[string]string{
		"type":    "nic",
		"nictype": nicTypeBridged,
		"name":    deviceName,
		"parent":  parentDevice,
	}
	if hwAddr != "" {
		device["hwaddr"] = hwAddr
	}
	if mtu > 0 {
		device["mtu"] = fmt.Sprintf("%v", mtu)
	}
	return device
}
