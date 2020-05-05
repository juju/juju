// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.api.common")

// NetworkConfigSource defines the necessary calls to obtain the network
// configuration of a machine.
type NetworkConfigSource interface {
	// SysClassNetPath returns the Linux kernel userspace SYSFS path used by
	// this source. DefaultNetworkConfigSource() uses network.SysClassNetPath.
	SysClassNetPath() string

	// Interfaces returns information about all network interfaces on the
	// machine as []net.Interface.
	Interfaces() ([]net.Interface, error)

	// InterfaceAddresses returns information about all addresses assigned to
	// the network interface with the given name.
	InterfaceAddresses(name string) ([]net.Addr, error)

	// DefaultRoute returns the gateway IP address and device name of the
	// default route on the machine. If there is no default route (known),
	// then zero values are returned.
	DefaultRoute() (net.IP, string, error)
}

type netPackageConfigSource struct{}

// SysClassNetPath implements NetworkConfigSource.
func (n *netPackageConfigSource) SysClassNetPath() string {
	return network.SysClassNetPath
}

// Interfaces implements NetworkConfigSource.
func (n *netPackageConfigSource) Interfaces() ([]net.Interface, error) {
	return net.Interfaces()
}

// InterfaceAddresses implements NetworkConfigSource.
func (n *netPackageConfigSource) InterfaceAddresses(name string) ([]net.Addr, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return iface.Addrs()
}

// DefaultRoute implements NetworkConfigSource.
func (n *netPackageConfigSource) DefaultRoute() (net.IP, string, error) {
	return network.GetDefaultRoute()
}

// DefaultNetworkConfigSource returns a NetworkConfigSource backed by the net
// package, to be used with GetObservedNetworkConfig().
func DefaultNetworkConfigSource() NetworkConfigSource {
	return &netPackageConfigSource{}
}

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
// * TODO: IPv6 link-local addresses will be ignored and treated as empty ATM.
//
// Result entries will be grouped by InterfaceName, in the same order they are
// returned by the given source.
func GetObservedNetworkConfig(source NetworkConfigSource) ([]params.NetworkConfig, error) {
	logger.Tracef("discovering observed machine network config...")

	interfaces, err := source.Interfaces()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get network interfaces")
	}
	if len(interfaces) == 0 {
		logger.Tracef("no network interfaces")
		return nil, nil
	}

	defaultRoute, defaultRouteDevice, err := source.DefaultRoute()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get default route")
	}
	var namesOrder []string
	nameToConfigs := make(map[string][]params.NetworkConfig)
	sysClassNetPath := source.SysClassNetPath()
	for _, nic := range interfaces {
		nicType := network.ParseInterfaceType(sysClassNetPath, nic.Name)
		nicConfig := interfaceToNetworkConfig(nic, nicType, corenetwork.OriginMachine)
		if nicConfig.InterfaceName == defaultRouteDevice {
			nicConfig.IsDefaultGateway = true
			nicConfig.GatewayAddress = defaultRoute.String()
		}

		if nicType == corenetwork.BridgeInterface {
			updateParentForBridgePorts(nic.Name, sysClassNetPath, nameToConfigs)
		}

		seenSoFar := false
		if existing, ok := nameToConfigs[nic.Name]; ok {
			nicConfig.ParentInterfaceName = existing[0].ParentInterfaceName
			// If only ParentInterfaceName was set in a previous iteration (e.g.
			// if the bridge appeared before the port), treat the interface as
			// not yet seen.
			seenSoFar = existing[0].InterfaceName != ""
		}

		if !seenSoFar {
			nameToConfigs[nic.Name] = []params.NetworkConfig(nil)
			namesOrder = append(namesOrder, nic.Name)
		}

		addrs, err := source.InterfaceAddresses(nic.Name)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get interface %q addresses", nic.Name)
		}

		if len(addrs) == 0 {
			logger.Infof("no addresses observed on interface %q", nic.Name)
			nameToConfigs[nic.Name] = append(nameToConfigs[nic.Name], nicConfig)
			continue
		}

		for _, addr := range addrs {
			addressConfig, err := interfaceAddressToNetworkConfig(nic.Name, nicConfig.ConfigType, addr)
			if err != nil {
				return nil, errors.Trace(err)
			}

			// Need to copy nicConfig so only the fields relevant for the
			// current address are updated.
			nicConfigCopy := nicConfig
			nicConfigCopy.Address = addressConfig.Address
			nicConfigCopy.CIDR = addressConfig.CIDR
			nicConfigCopy.ConfigType = addressConfig.ConfigType
			nameToConfigs[nic.Name] = append(nameToConfigs[nic.Name], nicConfigCopy)
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

func interfaceToNetworkConfig(nic net.Interface,
	nicType corenetwork.InterfaceType,
	networkOrigin corenetwork.Origin,
) params.NetworkConfig {
	configType := corenetwork.ConfigManual // assume manual initially, until we parse the address.
	isUp := nic.Flags&net.FlagUp > 0
	isLoopback := nic.Flags&net.FlagLoopback > 0
	isUnknown := nicType == corenetwork.UnknownInterface

	switch {
	case isUnknown && isLoopback:
		nicType = corenetwork.LoopbackInterface
		configType = corenetwork.ConfigLoopback
	case isUnknown:
		nicType = corenetwork.EthernetInterface
	}

	return params.NetworkConfig{
		DeviceIndex:   nic.Index,
		MACAddress:    nic.HardwareAddr.String(),
		ConfigType:    string(configType),
		MTU:           nic.MTU,
		InterfaceName: nic.Name,
		InterfaceType: string(nicType),
		NoAutoStart:   !isUp,
		Disabled:      !isUp,
		NetworkOrigin: params.NetworkOrigin(networkOrigin),
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

func interfaceAddressToNetworkConfig(interfaceName, configType string, address net.Addr) (params.NetworkConfig, error) {
	config := params.NetworkConfig{
		ConfigType: configType,
	}

	cidrAddress := address.String()
	if cidrAddress == "" {
		return config, nil
	}

	ip, ipNet, err := net.ParseCIDR(cidrAddress)
	if err != nil {
		logger.Tracef("cannot parse %q on interface %q as CIDR, trying as IP address: %v", cidrAddress, interfaceName, err)
		if ip = net.ParseIP(cidrAddress); ip == nil {
			return config, errors.Errorf("cannot parse IP address %q on interface %q", cidrAddress, interfaceName)
		} else {
			ipNet = &net.IPNet{IP: ip}
		}
	}
	if ip.To4() == nil && ip.IsLinkLocalUnicast() {
		// TODO(macgreagoir) IPv6. Skip link-local for now until we decide how to handle them.
		logger.Tracef("skipping observed IPv6 link-local address %q on %q", ip, interfaceName)
		return config, nil
	}

	if ipNet.Mask != nil {
		config.CIDR = ipNet.String()
	}
	config.Address = ip.String()
	if configType != string(corenetwork.ConfigLoopback) {
		config.ConfigType = string(corenetwork.ConfigStatic)
	}

	// TODO(dimitern): Add DNS servers, search domains, and gateway
	// later.

	return config, nil
}
