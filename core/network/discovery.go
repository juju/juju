// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"
	"strings"

	"github.com/juju/juju/internal/errors"
)

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
func GetObservedNetworkConfig(source ConfigSource) (InterfaceInfos, error) {
	logger.Tracef(context.TODO(), "discovering observed machine network config...")

	interfaces, err := source.Interfaces()
	if err != nil {
		return nil, errors.Errorf("detecting network interfaces: %w", err)
	}
	if len(interfaces) == 0 {
		logger.Tracef(context.TODO(), "no network interfaces")
		return nil, nil
	}

	knownOVSBridges, err := source.OvsManagedBridges()
	if err != nil {
		// NOTE(achilleasa): we will only get an error here if we do
		// locate the OVS cli tools and get an error executing them.
		return nil, errors.Errorf("querying OVS bridges: %w", err)
	}

	defaultRoute, defaultRouteDevice, err := source.DefaultRoute()
	if err != nil {
		return nil, errors.Errorf("retrieving default route: %w", err)
	}

	var configs InterfaceInfos
	var bridgeNames []string
	var noAddressesNics []string

	for _, nic := range interfaces {
		virtualPortType := NonVirtualPort
		if knownOVSBridges.Contains(nic.Name()) {
			virtualPortType = OvsPort
		}

		nicConfig := createInterfaceInfo(nic, virtualPortType)

		if nicConfig.InterfaceName == defaultRouteDevice {
			nicConfig.IsDefaultGateway = true
			nicConfig.GatewayAddress = NewMachineAddress(defaultRoute.String()).AsProviderAddress()
		}

		// Collect all the bridge device names. We will use these to update all
		// the parent device names for the bridge's port devices at the end.
		if nic.Type() == BridgeDevice {
			bridgeNames = append(bridgeNames, nic.Name())
		}

		nicAddrs, err := nic.Addresses()
		if err != nil {
			return nil, errors.Errorf("detecting addresses for %q: %w", nic.Name(), err)
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
			if nic.Type() != LoopbackDevice {
				nicConfig.ConfigType = ConfigStatic
			}

			nicConfig.Addresses, err = addressesToConfig(nicConfig, nicAddrs)
			if err != nil {
				return nil, errors.Capture(err)
			}
		} else {
			noAddressesNics = append(noAddressesNics, nic.Name())
		}
		configs = append(configs, nicConfig)
	}
	if len(noAddressesNics) > 0 {
		logger.Debugf(context.TODO(), "no addresses observed on interfaces %q", strings.Join(noAddressesNics, ", "))
	}

	updateParentsForBridgePorts(configs, bridgeNames, source)
	return configs, nil
}

func createInterfaceInfo(nic ConfigSourceNIC,
	virtualPortType VirtualPortType,
) InterfaceInfo {
	configType := ConfigManual
	if nic.Type() == LoopbackDevice {
		configType = ConfigLoopback
	}

	isUp := nic.IsUp()

	// TODO (dimitern): Add DNS servers and search domains.
	return InterfaceInfo{
		DeviceIndex:     nic.Index(),
		MACAddress:      nic.HardwareAddr().String(),
		ConfigType:      configType,
		MTU:             nic.MTU(),
		InterfaceName:   nic.Name(),
		InterfaceType:   nic.Type(),
		NoAutoStart:     !isUp,
		Disabled:        !isUp,
		VirtualPortType: virtualPortType,
		Origin:          OriginMachine,
	}
}

func addressesToConfig(nic InterfaceInfo, nicAddrs []ConfigSourceAddr) ([]ProviderAddress, error) {
	var res ProviderAddresses

	for _, nicAddr := range nicAddrs {
		if nicAddr == nil {
			return nil, errors.Errorf("cannot parse nil address on interface %q", nic.InterfaceName)
		}

		ip := nicAddr.IP()

		// TODO (macgreagoir): Skip IPv6 link-local until we decide how to handle them.
		if ip.To4() == nil && ip.IsLinkLocalUnicast() {
			logger.Tracef(context.TODO(), "skipping observed IPv6 link-local address %q on %q", ip, nic.InterfaceName)
			continue
		}

		opts := []func(mutator AddressMutator){
			WithConfigType(nic.ConfigType),
			WithSecondary(nicAddr.IsSecondary()),
		}

		if ipNet := nicAddr.IPNet(); ipNet != nil && ipNet.Mask != nil {
			opts = append(opts, WithCIDR(NetworkCIDRFromIPAndMask(ip, ipNet.Mask)))
		}

		// Constructing a core Address like this first,
		// then converting, populates the scope and type.
		res = append(res, NewMachineAddress(ip.String(), opts...).AsProviderAddress())
	}

	return res, nil
}

func updateParentsForBridgePorts(config []InterfaceInfo, bridgeNames []string, source ConfigSource) {
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
