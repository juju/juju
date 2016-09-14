// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

// BackingSubnet defines the methods supported by a Subnet entity
// stored persistently.
//
// TODO(dimitern): Once the state backing is implemented, remove this
// and just use *state.Subnet.
type BackingSubnet interface {
	CIDR() string
	VLANTag() int
	ProviderId() network.Id
	AvailabilityZones() []string
	Status() string
	SpaceName() string
	Life() params.Life
}

// BackingSubnetInfo describes a single subnet to be added in the
// backing store.
//
// TODO(dimitern): Replace state.SubnetInfo with this and remove
// BackingSubnetInfo, once the rest of state backing methods and the
// following pre-reqs are done:
// * subnetDoc.AvailabilityZone becomes subnetDoc.AvailabilityZones,
//   adding an upgrade step to migrate existing non empty zones on
//   subnet docs. Also change state.Subnet.AvailabilityZone to
// * add subnetDoc.SpaceName - no upgrade step needed, as it will only
//   be used for new space-aware subnets.
// * Subnets need a reference count to calculate Status.
// * ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//   and return all subnets, including the AvailabilityZones (for EC2;
//   empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId network.Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName holds the juju network space this subnet is
	// associated with. Can be empty if not supported.
	SpaceName string

	// Status holds the status of the subnet. Normally this will be
	// calculated from the reference count and Life of a subnet.
	Status string

	// Live holds the life of the subnet
	Life params.Life
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// Name returns the space name.
	Name() string

	// Subnets returns the subnets in the space
	Subnets() ([]BackingSubnet, error)

	// ProviderId returns the network ID of the provider
	ProviderId() network.Id

	// Zones returns a list of availability zone(s) that this
	// space is in. It can be empty if the provider does not support
	// availability zones.
	Zones() []string

	// Life returns the lifecycle state of the space
	Life() params.Life
}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type NetworkBacking interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() ([]providercommon.AvailabilityZone, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones([]providercommon.AvailabilityZone) error

	// AddSpace creates a space
	AddSpace(Name string, ProviderId network.Id, Subnets []string, Public bool) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]BackingSpace, error)

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(BackingSubnetInfo) (BackingSubnet, error)

	// AllSubnets returns all backing subnets.
	AllSubnets() ([]BackingSubnet, error)

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

func BackingSubnetToParamsSubnet(subnet BackingSubnet) params.Subnet {
	cidr := subnet.CIDR()
	vlantag := subnet.VLANTag()
	providerid := subnet.ProviderId()
	zones := subnet.AvailabilityZones()
	status := subnet.Status()
	var spaceTag names.SpaceTag
	if subnet.SpaceName() != "" {
		spaceTag = names.NewSpaceTag(subnet.SpaceName())
	}

	return params.Subnet{
		CIDR:       cidr,
		VLANTag:    vlantag,
		ProviderId: string(providerid),
		Zones:      zones,
		Status:     status,
		SpaceTag:   spaceTag.String(),
		Life:       subnet.Life(),
	}
}

// NetworkConfigFromInterfaceInfo converts a slice of network.InterfaceInfo into
// the equivalent params.NetworkConfig slice.
func NetworkConfigFromInterfaceInfo(interfaceInfos []network.InterfaceInfo) []params.NetworkConfig {
	result := make([]params.NetworkConfig, len(interfaceInfos))
	for i, v := range interfaceInfos {
		var dnsServers []string
		for _, nameserver := range v.DNSServers {
			dnsServers = append(dnsServers, nameserver.Value)
		}
		result[i] = params.NetworkConfig{
			DeviceIndex:         v.DeviceIndex,
			MACAddress:          v.MACAddress,
			CIDR:                v.CIDR,
			MTU:                 v.MTU,
			ProviderId:          string(v.ProviderId),
			ProviderSubnetId:    string(v.ProviderSubnetId),
			ProviderSpaceId:     string(v.ProviderSpaceId),
			ProviderVLANId:      string(v.ProviderVLANId),
			ProviderAddressId:   string(v.ProviderAddressId),
			VLANTag:             v.VLANTag,
			InterfaceName:       v.InterfaceName,
			ParentInterfaceName: v.ParentInterfaceName,
			InterfaceType:       string(v.InterfaceType),
			Disabled:            v.Disabled,
			NoAutoStart:         v.NoAutoStart,
			ConfigType:          string(v.ConfigType),
			Address:             v.Address.Value,
			DNSServers:          dnsServers,
			DNSSearchDomains:    v.DNSSearchDomains,
			GatewayAddress:      v.GatewayAddress.Value,
		}
	}
	return result
}

// NetworkConfigsToStateArgs splits the given networkConfig into a slice of
// state.LinkLayerDeviceArgs and a slice of state.LinkLayerDeviceAddress. The
// input is expected to come from MergeProviderAndObservedNetworkConfigs and to
// be sorted.
func NetworkConfigsToStateArgs(networkConfig []params.NetworkConfig) (
	[]state.LinkLayerDeviceArgs,
	[]state.LinkLayerDeviceAddress,
) {
	var devicesArgs []state.LinkLayerDeviceArgs
	var devicesAddrs []state.LinkLayerDeviceAddress

	logger.Tracef("transforming network config to state args: %+v", networkConfig)
	seenDeviceNames := set.NewStrings()
	for _, netConfig := range networkConfig {
		logger.Tracef("transforming device %q", netConfig.InterfaceName)
		if !seenDeviceNames.Contains(netConfig.InterfaceName) {
			// First time we see this, add it to devicesArgs.
			seenDeviceNames.Add(netConfig.InterfaceName)
			var mtu uint
			if netConfig.MTU >= 0 {
				mtu = uint(netConfig.MTU)
			}
			args := state.LinkLayerDeviceArgs{
				Name:        netConfig.InterfaceName,
				MTU:         mtu,
				ProviderID:  network.Id(netConfig.ProviderId),
				Type:        state.LinkLayerDeviceType(netConfig.InterfaceType),
				MACAddress:  netConfig.MACAddress,
				IsAutoStart: !netConfig.NoAutoStart,
				IsUp:        !netConfig.Disabled,
				ParentName:  netConfig.ParentInterfaceName,
			}
			logger.Tracef("state device args for device: %+v", args)
			devicesArgs = append(devicesArgs, args)
		}

		if netConfig.CIDR == "" || netConfig.Address == "" {
			logger.Tracef(
				"skipping empty CIDR %q and/or Address %q of %q",
				netConfig.CIDR, netConfig.Address, netConfig.InterfaceName,
			)
			continue
		}
		_, ipNet, err := net.ParseCIDR(netConfig.CIDR)
		if err != nil {
			logger.Warningf("FIXME: ignoring unexpected CIDR format %q: %v", netConfig.CIDR, err)
			continue
		}
		ipAddr := net.ParseIP(netConfig.Address)
		if ipAddr == nil {
			logger.Warningf("FIXME: ignoring unexpected Address format %q", netConfig.Address)
			continue
		}
		ipNet.IP = ipAddr
		cidrAddress := ipNet.String()

		var derivedConfigMethod state.AddressConfigMethod
		switch method := state.AddressConfigMethod(netConfig.ConfigType); method {
		case state.StaticAddress, state.DynamicAddress,
			state.LoopbackAddress, state.ManualAddress:
			derivedConfigMethod = method
		case "dhcp": // awkward special case
			derivedConfigMethod = state.DynamicAddress
		default:
			derivedConfigMethod = state.StaticAddress
		}

		addr := state.LinkLayerDeviceAddress{
			DeviceName:       netConfig.InterfaceName,
			ProviderID:       network.Id(netConfig.ProviderAddressId),
			ConfigMethod:     derivedConfigMethod,
			CIDRAddress:      cidrAddress,
			DNSServers:       netConfig.DNSServers,
			DNSSearchDomains: netConfig.DNSSearchDomains,
			GatewayAddress:   netConfig.GatewayAddress,
		}
		logger.Tracef("state address args for device: %+v", addr)
		devicesAddrs = append(devicesAddrs, addr)
	}
	logger.Tracef("seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef("network config transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
	return devicesArgs, devicesAddrs
}

// NetworkingEnvironFromModelConfig constructs and returns
// environs.NetworkingEnviron using the given configGetter. Returns an error
// satisfying errors.IsNotSupported() if the model config does not support
// networking features.
func NetworkingEnvironFromModelConfig(configGetter environs.EnvironConfigGetter) (environs.NetworkingEnviron, error) {
	modelConfig, err := configGetter.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get model config")
	}
	if modelConfig.Type() == "dummy" {
		return nil, errors.NotSupportedf("dummy provider network config")
	}
	env, err := environs.GetEnviron(configGetter, environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "failed to construct a model from config")
	}
	netEnviron, supported := environs.SupportsNetworking(env)
	if !supported {
		// " not supported" will be appended to the message below.
		return nil, errors.NotSupportedf("model %q networking", modelConfig.Name())
	}
	return netEnviron, nil
}

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
// * TODO: any IPv6 addresses found will be ignored and treated as empty ATM.
//
// Result entries will be grouped by InterfaceName, in the same order they are
// returned by the given source.
func GetObservedNetworkConfig(source NetworkConfigSource) ([]params.NetworkConfig, error) {
	logger.Tracef("discovering observed machine network config...")

	interfaces, err := source.Interfaces()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get network interfaces")
	}

	var namesOrder []string
	nameToConfigs := make(map[string][]params.NetworkConfig)
	sysClassNetPath := source.SysClassNetPath()
	for _, nic := range interfaces {
		nicType := network.ParseInterfaceType(sysClassNetPath, nic.Name)
		nicConfig := interfaceToNetworkConfig(nic, nicType)

		if nicType == network.BridgeInterface {
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

func interfaceToNetworkConfig(nic net.Interface, nicType network.InterfaceType) params.NetworkConfig {
	configType := network.ConfigManual // assume manual initially, until we parse the address.
	isUp := nic.Flags&net.FlagUp > 0
	isLoopback := nic.Flags&net.FlagLoopback > 0
	isUnknown := nicType == network.UnknownInterface

	switch {
	case isUnknown && isLoopback:
		nicType = network.LoopbackInterface
		configType = network.ConfigLoopback
	case isUnknown:
		nicType = network.EthernetInterface
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
		logger.Infof("cannot parse %q on interface %q as CIDR, trying as IP address: %v", cidrAddress, interfaceName, err)
		if ip = net.ParseIP(cidrAddress); ip == nil {
			return config, errors.Errorf("cannot parse IP address %q on interface %q", cidrAddress, interfaceName)
		} else {
			ipNet = &net.IPNet{IP: ip}
		}
	}
	if ip.To4() == nil {
		logger.Debugf("skipping observed IPv6 address %q on %q: not fully supported yet", ip, interfaceName)
		// TODO(dimitern): Treat IPv6 addresses as empty until we can handle
		// them reliably.
		return config, nil
	}

	if ipNet.Mask != nil {
		config.CIDR = ipNet.String()
	}
	config.Address = ip.String()
	if configType != string(network.ConfigLoopback) {
		config.ConfigType = string(network.ConfigStatic)
	}

	// TODO(dimitern): Add DNS servers, search domains, and gateway
	// later.

	return config, nil
}

// MergeProviderAndObservedNetworkConfigs returns the effective network configs,
// using observedConfigs as a base and selectively updating it using the
// matching providerConfigs for each interface.
func MergeProviderAndObservedNetworkConfigs(providerConfigs, observedConfigs []params.NetworkConfig) []params.NetworkConfig {

	providerConfigByName := networkConfigsByName(providerConfigs)
	logger.Tracef("known provider config by name: %+v", providerConfigByName)

	providerConfigByAddress := networkConfigsByAddress(providerConfigs)
	logger.Tracef("known provider config by address: %+v", providerConfigByAddress)

	var results []params.NetworkConfig
	for _, observed := range observedConfigs {

		name, ipAddress := observed.InterfaceName, observed.Address
		mergedConfig := observed

		providerConfig, known := providerConfigByName[name]
		if known {
			mergedConfig = mergeObservedAndProviderInterfaceConfig(mergedConfig, providerConfig)
			logger.Debugf("updated observed interface config for %q with: %+v", name, providerConfig)
		}

		providerConfig, known = providerConfigByAddress[ipAddress]
		if known {
			mergedConfig = mergeObservedAndProviderAddressConfig(mergedConfig, providerConfig)
			logger.Debugf("updated observed address config for %q with: %+v", name, providerConfig)
		}

		results = append(results, mergedConfig)
		logger.Debugf("merged config for %q: %+v", name, mergedConfig)
	}

	return results
}

func networkConfigsByName(input []params.NetworkConfig) map[string]params.NetworkConfig {
	configsByName := make(map[string]params.NetworkConfig, len(input))
	for _, config := range input {
		configsByName[config.InterfaceName] = config
	}
	return configsByName
}

func networkConfigsByAddress(input []params.NetworkConfig) map[string]params.NetworkConfig {
	configsByAddress := make(map[string]params.NetworkConfig, len(input))
	for _, config := range input {
		configsByAddress[config.Address] = config
	}
	return configsByAddress
}

func mergeObservedAndProviderInterfaceConfig(observedConfig, providerConfig params.NetworkConfig) params.NetworkConfig {
	mergedConfig := observedConfig

	mergedConfig.ProviderId = providerConfig.ProviderId
	mergedConfig.ProviderVLANId = providerConfig.ProviderVLANId
	mergedConfig.ProviderSubnetId = providerConfig.ProviderSubnetId

	if observedConfig.InterfaceType == "" {
		mergedConfig.InterfaceType = providerConfig.InterfaceType
	}

	if observedConfig.VLANTag == 0 {
		mergedConfig.VLANTag = providerConfig.VLANTag
	}

	if observedConfig.ParentInterfaceName == "" {
		mergedConfig.ParentInterfaceName = providerConfig.ParentInterfaceName
	}

	return mergedConfig
}

func mergeObservedAndProviderAddressConfig(observedConfig, providerConfig params.NetworkConfig) params.NetworkConfig {
	mergedConfig := observedConfig

	mergedConfig.ProviderAddressId = providerConfig.ProviderAddressId
	mergedConfig.ProviderSubnetId = providerConfig.ProviderSubnetId
	mergedConfig.ProviderSpaceId = providerConfig.ProviderSpaceId

	if observedConfig.ProviderVLANId == "" {
		mergedConfig.ProviderVLANId = providerConfig.ProviderVLANId
	}

	if observedConfig.VLANTag == 0 {
		mergedConfig.VLANTag = providerConfig.VLANTag
	}

	if observedConfig.ConfigType == "" {
		mergedConfig.ConfigType = providerConfig.ConfigType
	}

	if observedConfig.CIDR == "" {
		mergedConfig.CIDR = providerConfig.CIDR
	}

	if observedConfig.GatewayAddress == "" {
		mergedConfig.GatewayAddress = providerConfig.GatewayAddress
	}

	if len(observedConfig.DNSServers) == 0 {
		mergedConfig.DNSServers = providerConfig.DNSServers
	}

	if len(observedConfig.DNSSearchDomains) == 0 {
		mergedConfig.DNSSearchDomains = providerConfig.DNSSearchDomains
	}

	return mergedConfig
}
