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

// LocalBridgeName returns the name of the local LXD network bridge.
func (s *Server) LocalBridgeName() string {
	return s.localBridgeName
}

func (s *Server) VerifyDefaultBridge(profile *api.Profile, eTag string) error {
	eth0, ok := profile.Devices["eth0"]
	if !ok {
		// On LXD >= 2.3 there is no bridge config by default.
		// Ensure that the network bridge and eth0 device both exist.
		if s.networkAPISupport {
			return errors.Annotate(s.ensureDefaultBridge(profile, eTag), "ensuring default bridge config")
		}
		return errors.Errorf("unexpected LXD %q profile without eth0 device: %+v", profile.Name, profile)
	}

	netName := eth0["parent"]

	if s.networkAPISupport {
		net, _, err := s.GetNetwork(netName)
		if err != nil {
			return errors.Annotatef(err, "retrieving network %q", netName)
		}
		if err := ensureNoIPv6(net); err != nil {
			return errors.Trace(err)
		}
	}

	if eth0["type"] != "nic" || eth0["nictype"] != "bridged" {
		return errors.Errorf("eth0 is not configured as part of a bridge in %q profile: %v", profile.Name, eth0)
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
		if !isLXDNotFound(err) {
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
		if err := ensureNoIPv6(net); err != nil {
			return errors.Trace(err)
		}
	}

	s.localBridgeName = network.DefaultLXDBridge

	// Add the eth0 device with the bridge as its parent.
	nicType := "macvlan"
	if net.Type == "bridge" {
		nicType = "bridged"
	}
	profile.Devices["eth0"] = map[string]string{
		"type":    "nic",
		"nictype": nicType,
		"parent":  network.DefaultLXDBridge,
	}
	return errors.Trace(s.UpdateProfile(profile.Name, profile.Writable(), eTag))
}

// ensureNoIPv6 checks that the input network has no IPv6 configuration.
// An error is returned when it does.
func ensureNoIPv6(net *api.Network) error {
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
