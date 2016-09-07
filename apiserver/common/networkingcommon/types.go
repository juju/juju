// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"encoding/json"
	"net"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
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

type byMACThenCIDRThenIndexThenName []params.NetworkConfig

func (c byMACThenCIDRThenIndexThenName) Len() int {
	return len(c)
}

func (c byMACThenCIDRThenIndexThenName) Swap(i, j int) {
	orgI, orgJ := c[i], c[j]
	c[j], c[i] = orgI, orgJ
}

func (c byMACThenCIDRThenIndexThenName) Less(i, j int) bool {
	if c[i].MACAddress == c[j].MACAddress {
		// Same MACAddress means related interfaces.
		if c[i].CIDR == "" || c[j].CIDR == "" {
			// Empty CIDRs go at the bottom, otherwise order by InterfaceName.
			return c[i].CIDR != "" || c[i].InterfaceName < c[j].InterfaceName
		}
		if c[i].DeviceIndex == c[j].DeviceIndex {
			if c[i].InterfaceName == c[j].InterfaceName {
				// Sort addresses of the same interface.
				return c[i].CIDR < c[j].CIDR || c[i].Address < c[j].Address
			}
			// Prefer shorter names (e.g. parents) with equal DeviceIndex.
			return c[i].InterfaceName < c[j].InterfaceName
		}
		// When both CIDR and DeviceIndex are non-empty, order by DeviceIndex
		return c[i].DeviceIndex < c[j].DeviceIndex
	}
	// Group by MACAddress.
	return c[i].MACAddress < c[j].MACAddress
}

// SortNetworkConfigsByParents returns the given input sorted, such that any
// child interfaces appear after their parents.
func SortNetworkConfigsByParents(input []params.NetworkConfig) []params.NetworkConfig {
	sortedInputCopy := CopyNetworkConfigs(input)
	sort.Stable(byMACThenCIDRThenIndexThenName(sortedInputCopy))
	return sortedInputCopy
}

type byInterfaceName []params.NetworkConfig

func (c byInterfaceName) Len() int {
	return len(c)
}

func (c byInterfaceName) Swap(i, j int) {
	orgI, orgJ := c[i], c[j]
	c[j], c[i] = orgI, orgJ
}

func (c byInterfaceName) Less(i, j int) bool {
	return c[i].InterfaceName < c[j].InterfaceName
}

// SortNetworkConfigsByInterfaceName returns the given input sorted by
// InterfaceName.
func SortNetworkConfigsByInterfaceName(input []params.NetworkConfig) []params.NetworkConfig {
	sortedInputCopy := CopyNetworkConfigs(input)
	sort.Stable(byInterfaceName(sortedInputCopy))
	return sortedInputCopy
}

// NetworkConfigsToIndentedJSON returns the given input as an indented JSON
// string.
func NetworkConfigsToIndentedJSON(input []params.NetworkConfig) (string, error) {
	jsonBytes, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// CopyNetworkConfigs returns a copy of the given input
func CopyNetworkConfigs(input []params.NetworkConfig) []params.NetworkConfig {
	return append([]params.NetworkConfig(nil), input...)
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

// MergeProviderAndObservedNetworkConfigs returns the effective, sorted, network
// configs after merging providerConfig with observedConfig.
func MergeProviderAndObservedNetworkConfigs(providerConfigs, observedConfigs []params.NetworkConfig) ([]params.NetworkConfig, error) {
	providerConfigsByNameThenAddress := mapNetworkConfigsByNameThenAddress(providerConfigs, "provider")
	observedConfigsByNameThenAddress := mapNetworkConfigsByNameThenAddress(observedConfigs, "observed")

	var results []params.NetworkConfig
	for name, observedAddressConfigs := range observedConfigsByNameThenAddress {

		providerAddressConfigs, known := providerConfigsByNameThenAddress[name]
		if !known {
			logger.Debugf("interface %q has no provider config (using observed: %+v)", name, observedAddressConfigs)
			for _, observed := range observedAddressConfigs {
				results = append(results, observed)
			}
			continue
		}

		logger.Debugf(
			"merging interface %q observed (%+v) and provider (%+v) configs",
			name, observedAddressConfigs, providerAddressConfigs,
		)

		for observedAddress, observedConfig := range observedAddressConfigs {
			parentName := observedConfig.ParentInterfaceName
			providerConfig, known := providerAddressConfigs[observedAddress]
			if !known && observedAddress == "" && parentName != "" {
				// This is an interface that got bridged and the parent bridge
				// took its address(es).
				logger.Debugf(
					"interface %q has no observed address and parent interface %s",
					name, parentName,
				)
				for parentAddress, _ := range observedConfigsByNameThenAddress[parentName] {
					providerConfig, known = providerAddressConfigs[parentAddress]
					if known {
						logger.Debugf(
							"interface %q's parent %q has observed address %q and matching provider config: %+v",
							name, parentName, parentAddress, providerConfig,
						)
						break
					}

					logger.Debugf(
						"interface %q's parent %q has observed address %q and no matching provider config",
						name, parentName, parentAddress,
					)
				}
			}

			mergedConfig := mergeSingleObservedWithProviderConfig(observedConfig, providerConfig)
			results = append(results, mergedConfig)

			logger.Debugf(
				"interface %q has observed address %q, observed config (%+v), matching provider config (%+v), and merged config: %+v",
				name, observedAddress, observedConfig, providerConfig, mergedConfig,
			)
		}
	}

	return results, nil
}

// mapNetworkConfigsByNameThenAddress translates input to a nested map, first by
// name, then by address.
func mapNetworkConfigsByNameThenAddress(input []params.NetworkConfig, configName string) map[string]map[string]params.NetworkConfig {
	configsByNameThenAddress := make(map[string]map[string]params.NetworkConfig, len(input))
	for _, config := range input {
		name, nicType := config.InterfaceName, config.InterfaceType

		address := ""
		switch netAddress := network.NewAddress(config.Address); netAddress.Type {
		case network.IPv6Address:
			// TODO(dimitern): Handle IPv6 addresses here when we can.
			logger.Debugf(
				"ignoring IPv6 %s address %q on %s interface %q - not yet supported",
				configName, netAddress.Value, nicType, name,
			)
		default:
			address = netAddress.Value
		}

		logger.Debugf("%s interface %q has %s address %q", nicType, name, configName, address)

		nicConfig, nicKnown := configsByNameThenAddress[name]
		if !nicKnown {
			nicConfig = make(map[string]params.NetworkConfig)
		}

		// Already seen this interface, and since we don't expect duplicates
		// in input, log them if they occur to simplify debugging.
		if _, addressKnown := nicConfig[address]; addressKnown {
			logger.Warningf("duplicate %s address %q on %s interface %q", configName, address, nicType, name)
		}

		// Add a new address for already known interface.
		nicConfig[address] = config
		configsByNameThenAddress[name] = nicConfig
	}

	return configsByNameThenAddress
}

func mergeSingleObservedWithProviderConfig(observedConfig, providerConfig params.NetworkConfig) params.NetworkConfig {
	// Prefer observed config values over provider config values, except in
	// a few cases there the latter is a better source.
	mergedConfig := observedConfig

	if observedConfig.InterfaceType == "" {
		mergedConfig.InterfaceType = providerConfig.InterfaceType
	}
	if observedConfig.ParentInterfaceName == "" {
		mergedConfig.ParentInterfaceName = providerConfig.ParentInterfaceName
	}
	if observedConfig.VLANTag == 0 {
		mergedConfig.VLANTag = providerConfig.VLANTag
	}

	// The following values are only known by the provider.
	mergedConfig.ProviderId = providerConfig.ProviderId
	mergedConfig.ProviderSubnetId = providerConfig.ProviderSubnetId
	mergedConfig.ProviderSpaceId = providerConfig.ProviderSpaceId
	mergedConfig.ProviderAddressId = providerConfig.ProviderAddressId
	mergedConfig.ProviderVLANId = providerConfig.ProviderVLANId

	return mergedConfig
}

func oldMergeProviderAndObservedNetworkConfigs(providerConfigs, observedConfigs []params.NetworkConfig) ([]params.NetworkConfig, error) {
	providerConfigsByName := make(map[string][]params.NetworkConfig)
	sortedProviderConfigs := SortNetworkConfigsByParents(providerConfigs)
	for _, config := range sortedProviderConfigs {
		name := config.InterfaceName
		providerConfigsByName[name] = append(providerConfigsByName[name], config)
	}

	jsonProviderConfig, err := NetworkConfigsToIndentedJSON(sortedProviderConfigs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot serialize provider config %#v as JSON", sortedProviderConfigs)
	}
	logger.Debugf("provider network config of machine:\n%s", jsonProviderConfig)

	sortedObservedConfigs := SortNetworkConfigsByParents(observedConfigs)
	jsonObservedConfig, err := NetworkConfigsToIndentedJSON(sortedObservedConfigs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot serialize observed config %#v as JSON", sortedObservedConfigs)
	}
	logger.Debugf("observed network config of machine:\n%s", jsonObservedConfig)

	var mergedConfigs []params.NetworkConfig
	for _, config := range sortedObservedConfigs {
		name := config.InterfaceName
		logger.Tracef("merging observed config for device %q: %+v", name, config)
		if strings.HasPrefix(name, instancecfg.DefaultBridgePrefix) {
			logger.Tracef("found potential juju bridge %q in observed config", name)
			unprefixedName := strings.TrimPrefix(name, instancecfg.DefaultBridgePrefix)
			underlyingConfigs, underlyingKnownByProvider := providerConfigsByName[unprefixedName]
			logger.Tracef("device %q underlying %q has provider config: %+v", name, unprefixedName, underlyingConfigs)
			if underlyingKnownByProvider {
				// This config is for a bridge created by Juju and not known by
				// the provider. The bridge is configured to adopt the address
				// allocated to the underlying interface, which is known by the
				// provider. However, since the same underlying interface can
				// have multiple addresses, we need to match the adopted
				// bridgeConfig to the correct address.

				var underlyingConfig params.NetworkConfig
				for i, underlying := range underlyingConfigs {
					if underlying.Address == config.Address {
						logger.Tracef("replacing undelying config %+v", underlying)
						// Remove what we found before changing it below.
						underlyingConfig = underlying
						underlyingConfigs = append(underlyingConfigs[:i], underlyingConfigs[i+1:]...)
						break
					}
				}
				logger.Tracef("underlying provider config after update: %+v", underlyingConfigs)

				bridgeConfig := config
				bridgeConfig.InterfaceType = string(network.BridgeInterface)
				bridgeConfig.ConfigType = underlyingConfig.ConfigType
				bridgeConfig.VLANTag = underlyingConfig.VLANTag
				bridgeConfig.ProviderId = "" // Juju-created bridges never have a ProviderID
				bridgeConfig.ProviderSpaceId = underlyingConfig.ProviderSpaceId
				bridgeConfig.ProviderVLANId = underlyingConfig.ProviderVLANId
				bridgeConfig.ProviderSubnetId = underlyingConfig.ProviderSubnetId
				bridgeConfig.ProviderAddressId = underlyingConfig.ProviderAddressId
				if underlyingParent := underlyingConfig.ParentInterfaceName; underlyingParent != "" {
					bridgeConfig.ParentInterfaceName = instancecfg.DefaultBridgePrefix + underlyingParent
				}

				underlyingConfig.ConfigType = string(network.ConfigManual)
				underlyingConfig.ParentInterfaceName = name
				underlyingConfig.ProviderAddressId = ""
				underlyingConfig.CIDR = ""
				underlyingConfig.Address = ""

				underlyingConfigs = append(underlyingConfigs, underlyingConfig)
				providerConfigsByName[unprefixedName] = underlyingConfigs
				logger.Tracef("updated provider network config by name: %+v", providerConfigsByName)

				mergedConfigs = append(mergedConfigs, bridgeConfig)
				continue
			}
		}

		knownProviderConfigs, knownByProvider := providerConfigsByName[name]
		if !knownByProvider {
			// Not known by the provider and not a Juju-created bridge, so just
			// use the observed config for it.
			logger.Tracef("device %q not known to provider - adding only observed config: %+v", name, config)
			mergedConfigs = append(mergedConfigs, config)
			continue
		}
		logger.Tracef("device %q has known provider network config: %+v", name, knownProviderConfigs)

		for _, providerConfig := range knownProviderConfigs {
			if providerConfig.Address == config.Address {
				logger.Tracef(
					"device %q has observed address %q, index %d, and MTU %q; overriding index %d and MTU %d from provider config",
					name, config.Address, config.DeviceIndex, config.MTU, providerConfig.DeviceIndex, providerConfig.MTU,
				)
				// Prefer observed device indices and MTU values as more up-to-date.
				providerConfig.DeviceIndex = config.DeviceIndex
				providerConfig.MTU = config.MTU

				mergedConfigs = append(mergedConfigs, providerConfig)
				break
			}
		}
	}

	sortedMergedConfigs := SortNetworkConfigsByParents(mergedConfigs)

	jsonMergedConfig, err := NetworkConfigsToIndentedJSON(sortedMergedConfigs)
	if err != nil {
		errors.Annotatef(err, "cannot serialize merged config %#v as JSON", sortedMergedConfigs)
	}
	logger.Debugf("combined machine network config:\n%s", jsonMergedConfig)

	return mergedConfigs, nil
}
