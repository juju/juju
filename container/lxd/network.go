// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"

	"github.com/juju/juju/network"
)

const (
	nicTypeBridged = "bridged"
	nicTypeMACVLAN = "macvlan"
)

// LocalBridgeName returns the name of the local LXD network bridge.
func (s *Server) LocalBridgeName() string {
	return s.localBridgeName
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
			net.Config = make(map[string]string, 2)
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

// VerifyNetworkDevice attempts to ensure that there is a network usable by LXD
// and that there is an eth0 NIC device with said network as its parent.
// If we are not operating as part of a cluster, an attempt will be made to
// create a missing eth0 device with the default LXD bridge as its parent.
// If in clustered mode this attempt is not made - the onus of network
// configuration falls on the operator.
// NOTE (manadart): Cluster behaviour is subject to change.
func (s *Server) VerifyNetworkDevice(profile *api.Profile, eTag string) error {
	eth0, ok := profile.Devices["eth0"]
	if !ok {
		// On LXD >= 2.3 there is no bridge config by default.
		// If not part of a cluster, ensure that the network bridge and eth0
		// device both exist.
		if s.networkAPISupport && !s.clustered {
			return errors.Annotate(s.ensureDefaultBridge(profile, eTag), "ensuring default bridge config")
		}
		return errors.Errorf("profile %q does not have an \"eth0\" device", profile.Name)
	}

	netName := eth0["parent"]

	if s.networkAPISupport {
		net, _, err := s.GetNetwork(netName)
		if err != nil {
			return errors.Annotatef(err, "retrieving network %q", netName)
		}
		if err := verifyNoIPv6(net); err != nil {
			return errors.Trace(err)
		}
	}

	if err := verifyNetworkDeviceType("eth0", eth0); err != nil {
		return errors.Annotatef(err, "profile %q", profile.Name)
	}

	s.localBridgeName = netName
	logger.Infof("LXD %q profile uses network bridge %q", profile.Name, netName)

	// If this LXD version supports the network API, we are done.
	// Otherwise we need to check the legacy lxd-bridge config file.
	if s.networkAPISupport {
		return nil
	}
	return errors.Trace(checkBridgeConfigFile(ioutil.ReadFile))
}

// ensureDefaultBridge ensures that the default LXD bridge exists,
// that it is not configured to use IPv6, and that the eth0 device exists in
// the input profile.
// An error is returned if the bridge exists with IPv6 configuration.
// If the bridge does not exist, it is created.
func (s *Server) ensureDefaultBridge(profile *api.Profile, eTag string) error {
	net, _, err := s.GetNetwork(network.DefaultLXDBridge)
	if err != nil {
		if !IsLXDNotFound(err) {
			return errors.Trace(err)
		}
		req := api.NetworksPost{
			Name:    network.DefaultLXDBridge,
			Type:    "bridge",
			Managed: true,
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
	} else {
		if err := verifyNoIPv6(net); err != nil {
			return errors.Trace(err)
		}
	}

	s.localBridgeName = network.DefaultLXDBridge

	// Add the eth0 device with the bridge as its parent.
	nicType := nicTypeMACVLAN
	if net.Type == "bridge" {
		nicType = nicTypeBridged
	}
	profile.Devices["eth0"] = map[string]string{
		"type":    "nic",
		"nictype": nicType,
		"parent":  network.DefaultLXDBridge,
	}
	return errors.Trace(s.UpdateProfile(profile.Name, profile.Writable(), eTag))
}

// verifyNoIPv6 checks that the input network has no IPv6 configuration.
// An error is returned when it does.
func verifyNoIPv6(net *api.Network) error {
	if !net.Managed {
		return nil
	}
	cfg, ok := net.Config["ipv6.address"]
	if !ok {
		return nil
	}
	if cfg == "none" {
		return nil
	}

	return errors.Errorf("juju does not support IPv6. Disable IPv6 in LXD via:\n"+
		"\tlxc network set %s ipv6.address none\n"+
		"and run the command again", net.Name)
}

// verifyNetworkDeviceType checks that the input device is correctly configured
// as a NIC. An error is returned if not.
func verifyNetworkDeviceType(name string, cfg map[string]string) error {
	if cfg["type"] != "nic" {
		return errors.Errorf("device %q is not configured with type=nic", name)
	}
	if cfg["nictype"] != nicTypeBridged && cfg["nictype"] != nicTypeMACVLAN {
		return errors.Errorf("device %q is not configured with nictype %q or %q", name, nicTypeBridged, nicTypeMACVLAN)
	}
	return nil
}

const BridgeConfigFile = "/etc/default/lxd-bridge"

func checkBridgeConfigFile(reader func(string) ([]byte, error)) error {
	bridgeConfig, err := reader(BridgeConfigFile)
	if os.IsNotExist(err) {
		return bridgeConfigError("lxdbr0 configured but no config file found at " + BridgeConfigFile)
	} else if err != nil {
		return errors.Trace(err)
	}

	foundSubnetConfig := false
	for _, line := range strings.Split(string(bridgeConfig), "\n") {
		if strings.HasPrefix(line, "USE_LXD_BRIDGE=") {
			b, err := strconv.ParseBool(strings.Trim(line[len("USE_LXD_BRIDGE="):], " \""))
			if err != nil {
				logger.Debugf("unable to parse bool, skipping USE_LXD_BRIDGE check: %s", err)
				continue
			}
			if !b {
				return bridgeConfigError("lxdbr0 not enabled but required")
			}
		} else if strings.HasPrefix(line, "LXD_BRIDGE=") {
			name := strings.Trim(line[len("LXD_BRIDGE="):], " \"")
			if name != network.DefaultLXDBridge {
				return bridgeConfigError(fmt.Sprintf("%s has a bridge named %s, not lxdbr0", BridgeConfigFile, name))
			}
		} else if strings.HasPrefix(line, "LXD_IPV4_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV4_ADDR="):], " \"")
			if len(contents) > 0 {
				foundSubnetConfig = true
			}
		} else if strings.HasPrefix(line, "LXD_IPV6_ADDR=") {
			contents := strings.Trim(line[len("LXD_IPV6_ADDR="):], " \"")
			if len(contents) > 0 {
				return ipv6BridgeConfigError(BridgeConfigFile)
			}
		}
	}

	if !foundSubnetConfig {
		return bridgeConfigError("lxdbr0 has no ipv4 or ipv6 subnet enabled")
	}

	return nil
}

func bridgeConfigError(err string) error {
	return errors.Errorf("%s\nIt looks like your lxdbr0 has not yet been configured. Configure it via:\n\n"+
		"\tsudo dpkg-reconfigure -p medium lxd\n\n"+
		"and run the command again.", err)
}

func ipv6BridgeConfigError(fileName string) error {
	return errors.Errorf("%s has IPv6 enabled.\nJuju doesn't currently support IPv6.\n"+
		"Disable IPv6 via:\n\n"+
		"\tsudo dpkg-reconfigure -p medium lxd\n\n"+
		"and run the command again.", fileName)
}
