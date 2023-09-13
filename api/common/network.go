// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/rpc/params"
)

var logger = loggo.GetLogger("juju.api.common")

// GetObservedNetworkConfig uses the given source to find all available network
// interfaces and their assigned addresses, and returns the result as
// []params.NetworkConfig. In addition to what the source returns, a few
// additional transformations are done:
//
//   - On any OS, the state (UP/DOWN) of each interface and the DeviceIndex field,
//     will be correctly populated. Loopback interfaces are also properly detected
//     and will have InterfaceType set as LoopbackInterface.
//   - On Linux only, the InterfaceType field will be reliably detected for a few
//     types: BondInterface, BridgeInterface, VLAN_8021QInterface.
//   - Also on Linux, for interfaces that are discovered to be ports on a bridge,
//     the ParentInterfaceName will be populated with the name of the bridge.
//   - ConfigType fields will be set to ConfigManual when no address is detected,
//     or ConfigStatic when it is.
//   - NICs that correspond to the internal port of an OVS-managed switch will
//     have their type forced to bridge and their virtual port type set to
//     OvsPort.
//   - TODO: IPv6 link-local addresses will be ignored and treated as empty ATM.
func GetObservedNetworkConfig(source network.ConfigSource) ([]params.NetworkConfig, error) {
	logger.Tracef("discovering observed machine network config...")

	interfaces, err := source.Interfaces()
	if err != nil {
		return nil, errors.Annotate(err, "detecting network interfaces")
	}
	if len(interfaces) == 0 {
		logger.Tracef("no network interfaces")
		return nil, nil
	}

	knownOVSBridges, err := source.OvsManagedBridges()
	if err != nil {
		// NOTE(achilleasa): we will only get an error here if we do
		// locate the OVS cli tools and get an error executing them.
		return nil, errors.Annotate(err, "querying OVS bridges")
	}

	defaultRoute, defaultRouteDevice, err := source.DefaultRoute()
	if err != nil {
		return nil, errors.Annotate(err, "retrieving default route")
	}

	var configs []params.NetworkConfig
	var bridgeNames []string
	var noAddressesNics []string

	for _, nic := range interfaces {
		virtualPortType := network.NonVirtualPort
		if knownOVSBridges.Contains(nic.Name()) {
			virtualPortType = network.OvsPort
		}

		nicConfig := interfaceToNetworkConfig(nic, virtualPortType)

		if nicConfig.InterfaceName == defaultRouteDevice {
			nicConfig.IsDefaultGateway = true
			nicConfig.GatewayAddress = defaultRoute.String()
		}

		// Collect all the bridge device names. We will use these to update all
		// the parent device names for the bridge's port devices at the end.
		if nic.Type() == network.BridgeDevice {
			bridgeNames = append(bridgeNames, nic.Name())
		}

		nicAddrs, err := nic.Addresses()
		if err != nil {
			return nil, errors.Annotatef(err, "detecting addresses for %q", nic.Name())
		}

		if len(nicAddrs) > 0 {
			// TODO (manadart 2021-05-07): This preserves prior behaviour,
			// but is incorrect for DHCP configured devices.
			// At present we do not store a config type against the device,
			// only the addresses (which incorrectly default to static too).
			// This could be corrected by interrogating the DHCP leases for
			// the device, should we ever need that detail.
			// At present we do not - we only use it to determine if an address
			// has a configuration method of "loopback".
			if nic.Type() != network.LoopbackDevice {
				nicConfig.ConfigType = string(network.ConfigStatic)
			}

			nicConfig.Addresses, err = addressesToConfig(nicConfig, nicAddrs)
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else {
			noAddressesNics = append(noAddressesNics, nic.Name())
		}
		configs = append(configs, nicConfig)
	}
	if len(noAddressesNics) > 0 {
		logger.Debugf("no addresses observed on interfaces %q", strings.Join(noAddressesNics, ", "))
	}

	updateParentsForBridgePorts(configs, bridgeNames, source)
	return configs, nil
}

func interfaceToNetworkConfig(nic network.ConfigSourceNIC,
	virtualPortType network.VirtualPortType,
) params.NetworkConfig {
	configType := network.ConfigManual
	if nic.Type() == network.LoopbackDevice {
		configType = network.ConfigLoopback
	}

	isUp := nic.IsUp()

	// TODO (dimitern): Add DNS servers and search domains.
	return params.NetworkConfig{
		DeviceIndex:     nic.Index(),
		MACAddress:      nic.HardwareAddr().String(),
		ConfigType:      string(configType),
		MTU:             nic.MTU(),
		InterfaceName:   nic.Name(),
		InterfaceType:   string(nic.Type()),
		NoAutoStart:     !isUp,
		Disabled:        !isUp,
		VirtualPortType: string(virtualPortType),
		NetworkOrigin:   params.NetworkOrigin(network.OriginMachine),
	}
}

func addressesToConfig(nic params.NetworkConfig, nicAddrs []network.ConfigSourceAddr) ([]params.Address, error) {
	var res []params.Address

	for _, nicAddr := range nicAddrs {
		if nicAddr == nil {
			return nil, errors.Errorf("cannot parse nil address on interface %q", nic.InterfaceName)
		}

		ip := nicAddr.IP()

		// TODO (macgreagoir): Skip IPv6 link-local until we decide how to handle them.
		if ip.To4() == nil && ip.IsLinkLocalUnicast() {
			logger.Tracef("skipping observed IPv6 link-local address %q on %q", ip, nic.InterfaceName)
			continue
		}

		opts := []func(mutator network.AddressMutator){
			network.WithConfigType(network.AddressConfigType(nic.ConfigType)),
			network.WithSecondary(nicAddr.IsSecondary()),
		}

		if ipNet := nicAddr.IPNet(); ipNet != nil && ipNet.Mask != nil {
			opts = append(opts, network.WithCIDR(network.NetworkCIDRFromIPAndMask(ip, ipNet.Mask)))
		}

		// Constructing a core network.Address like this first,
		// then converting, populates the scope and type.
		res = append(res, params.FromMachineAddress(network.NewMachineAddress(ip.String(), opts...)))
	}

	return res, nil
}

func updateParentsForBridgePorts(config []params.NetworkConfig, bridgeNames []string, source network.ConfigSource) {
	for _, bridgeName := range bridgeNames {
		for _, portName := range source.GetBridgePorts(bridgeName) {
			for i := range config {
				if config[i].InterfaceName == portName {
					config[i].ParentInterfaceName = bridgeName
					break
				}
			}
		}
	}
}
