// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.api.common")

// GetObservedNetworkConfig uses the given source to find all available network
// interfaces and their assigned addresses, and returns the result as
// []params.NetworkConfig. In addition to what the source returns, a few
// additional transformations are done:
//
// * On any OS, the state (UP/DOWN) of each interface and the DeviceIndex field,
//   will be correctly populated. Loopback interfaces are also properly detected
//   and will have InterfaceType set LoopbackInterface.
// * On Linux only, the InterfaceType field will be reliably detected for a few
//   types: BondInterface, BridgeInterface, VLAN_8021QInterface.
// * Also on Linux, for interfaces that are discovered to be ports on a bridge,
//   the ParentInterfaceName will be populated with the name of the bridge.
// * ConfigType fields will be set to ConfigManual when no address is detected,
//   or ConfigStatic when it is.
// * NICs that correspond to the internal port of an OVS-managed switch will
//   have their type forced to bridge and their virtual port type set to
//   OvsPort.
// * TODO: IPv6 link-local addresses will be ignored and treated as empty ATM.
//
// Result entries will be grouped by InterfaceName, in the same order they are
// returned by the given source.
func GetObservedNetworkConfig(source corenetwork.ConfigSource) ([]params.NetworkConfig, error) {
	logger.Tracef("discovering observed machine network config...")

	interfaces, err := source.Interfaces()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get network interfaces")
	}
	if len(interfaces) == 0 {
		logger.Tracef("no network interfaces")
		return nil, nil
	}

	knownOVSBridges, err := source.OvsManagedBridges()
	if err != nil {
		// NOTE(achilleasa): we will only get an error here if we do
		// locate the OVS cli tools and get an error executing them.
		return nil, errors.Annotate(err, "cannot query list of OVS bridges")
	}

	defaultRoute, defaultRouteDevice, err := source.DefaultRoute()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get default route")
	}
	var namesOrder []string
	nameToConfigs := make(map[string][]params.NetworkConfig)
	sysClassNetPath := source.SysClassNetPath()
	for _, nic := range interfaces {
		virtualPortType := corenetwork.NonVirtualPort
		if knownOVSBridges.Contains(nic.Name()) {
			virtualPortType = corenetwork.OvsPort
		}

		nicType := nic.Type()
		nicConfig := interfaceToNetworkConfig(nic, nicType, virtualPortType, corenetwork.OriginMachine)

		if nicConfig.InterfaceName == defaultRouteDevice {
			nicConfig.IsDefaultGateway = true
			nicConfig.GatewayAddress = defaultRoute.String()
		}

		if nicType == corenetwork.BridgeInterface {
			updateParentForBridgePorts(nic.Name(), sysClassNetPath, nameToConfigs)
		}

		seenSoFar := false
		if existing, ok := nameToConfigs[nic.Name()]; ok {
			nicConfig.ParentInterfaceName = existing[0].ParentInterfaceName
			// If only ParentInterfaceName was set in a previous iteration (e.g.
			// if the bridge appeared before the port), treat the interface as
			// not yet seen.
			seenSoFar = existing[0].InterfaceName != ""
		}

		if !seenSoFar {
			nameToConfigs[nic.Name()] = []params.NetworkConfig(nil)
			namesOrder = append(namesOrder, nic.Name())
		}

		addrs, err := nic.Addresses()
		if err != nil {
			return nil, errors.Annotatef(err, "retrieving interface %q addresses", nic.Name)
		}

		if len(addrs) == 0 {
			logger.Infof("no addresses observed on interface %q", nic.Name)
			nameToConfigs[nic.Name()] = append(nameToConfigs[nic.Name()], nicConfig)
			continue
		}

		for _, addr := range addrs {
			addressConfig, err := interfaceAddressToNetworkConfig(nic.Name(), nicConfig.ConfigType, addr)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Need to copy nicConfig so only the fields relevant for the
			// current address are updated.
			nicConfigCopy := nicConfig
			nicConfigCopy.Address = addressConfig.Address
			nicConfigCopy.CIDR = addressConfig.CIDR
			nicConfigCopy.ConfigType = addressConfig.ConfigType
			nameToConfigs[nic.Name()] = append(nameToConfigs[nic.Name()], nicConfigCopy)
		}
	}

	// Return all interfaces configs in input order.
	var observedConfig []params.NetworkConfig
	for _, name := range namesOrder {
		observedConfig = append(observedConfig, nameToConfigs[name]...)
	}
	logger.Tracef("observed network config: %+v", observedConfig)
	return observedConfig, nil
}

func interfaceToNetworkConfig(nic corenetwork.ConfigSourceNIC,
	nicType corenetwork.InterfaceType,
	virtualPortType corenetwork.VirtualPortType,
	networkOrigin corenetwork.Origin,
) params.NetworkConfig {
	configType := corenetwork.ConfigManual
	if nicType == corenetwork.LoopbackInterface {
		configType = corenetwork.ConfigLoopback
	}

	isUp := nic.IsUp()

	return params.NetworkConfig{
		DeviceIndex:     nic.Index(),
		MACAddress:      nic.HardwareAddr().String(),
		ConfigType:      string(configType),
		MTU:             nic.MTU(),
		InterfaceName:   nic.Name(),
		InterfaceType:   string(nicType),
		NoAutoStart:     !isUp,
		Disabled:        !isUp,
		VirtualPortType: string(virtualPortType),
		NetworkOrigin:   params.NetworkOrigin(networkOrigin),
	}
}

func updateParentForBridgePorts(bridgeName, sysClassNetPath string, nameToConfigs map[string][]params.NetworkConfig) {
	ports := network.GetBridgePorts(sysClassNetPath, bridgeName)
	for _, portName := range ports {
		portConfigs, ok := nameToConfigs[portName]
		if ok {
			portConfigs[0].ParentInterfaceName = bridgeName
		} else {
			portConfigs = []params.NetworkConfig{{ParentInterfaceName: bridgeName}}
		}
		nameToConfigs[portName] = portConfigs
	}
}

func interfaceAddressToNetworkConfig(
	interfaceName, configType string, addr corenetwork.ConfigSourceAddr,
) (params.NetworkConfig, error) {
	config := params.NetworkConfig{
		ConfigType: configType,
	}

	if addr == nil {
		return config, errors.Errorf("cannot parse nil address on interface %q", interfaceName)
	}

	ip := addr.IP()
	if ip.To4() == nil && ip.IsLinkLocalUnicast() {
		// TODO(macgreagoir) IPv6. Skip link-local for now until we decide how to handle them.
		logger.Tracef("skipping observed IPv6 link-local address %q on %q", ip, interfaceName)
		return config, nil
	}

	if ipNet := addr.IPNet(); ipNet != nil && ipNet.Mask != nil {
		config.CIDR = ipNet.String()
	}
	config.Address = ip.String()
	if configType != string(corenetwork.ConfigLoopback) {
		config.ConfigType = string(corenetwork.ConfigStatic)
	}

	// TODO(dimitern): Add DNS servers, search domains, and gateway.

	return config, nil
}
